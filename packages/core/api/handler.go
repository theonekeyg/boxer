package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"boxer/config"
	"boxer/oci"
	"boxer/sandbox"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Handler holds the dependencies injected into HTTP handlers.
type Handler struct {
	cfg       *config.BoxerConfig
	cache     ImageCacher
	executor  SandboxExecutor
	fileStore *FileStore
}

// NewHandler constructs a Handler with all dependencies.
func NewHandler(cfg *config.BoxerConfig, cache ImageCacher, executor SandboxExecutor, fileStore *FileStore) *Handler {
	return &Handler{cfg: cfg, cache: cache, executor: executor, fileStore: fileStore}
}

// Health godoc
// @Summary     Health check
// @Description Returns {"ok": true} when the service is running
// @Tags        system
// @Produce     json
// @Success     200  {object}  map[string]bool
// @Router      /healthz [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UploadFile godoc
// @Summary     Upload a file to the file store
// @Description Multipart upload. The stored file can be referenced by its path in POST /run — it is bind-mounted read-only at /<path> inside the container.
// @Tags        files
// @Accept      multipart/form-data
// @Produce     json
// @Param       file  formData  file    true  "File content"
// @Param       path  formData  string  true  "Relative destination path (e.g. workspace/script.py)"
// @Success     200   {object}  UploadResponse     "Stored path"
// @Failure     400   {object}  ErrorResponse      "Missing or invalid form fields"
// @Failure     413   {object}  ErrorResponse      "Upload exceeds configured limit"
// @Failure     500   {object}  ErrorResponse      "Internal error"
// @Router      /files [post]
func (h *Handler) UploadFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(h.cfg.UploadLimitBytes))

	// FormFile triggers ParseMultipartForm, which surfaces MaxBytesError.
	// Must come before PostForm: Gin's PostForm swallows ParseMultipartForm errors.
	fh, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: "upload exceeds limit"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "form field 'file' is required: " + err.Error()})
		return
	}

	path := c.PostForm("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "form field 'path' is required"})
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "open upload: " + err.Error()})
		return
	}
	defer f.Close() //nolint:errcheck // multipart file handle is read-only

	if err := h.fileStore.Store(path, f); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, UploadResponse{Path: path})
}

// DownloadFile godoc
// @Summary     Download a file from the file store
// @Description Download any file by its relative path. To retrieve output files written by a container to /output/, use the path pattern output/<exec_id>/<filename>.
// @Tags        files
// @Produce     application/octet-stream
// @Param       path  query  string  true  "Relative file path (e.g. output/boxer-abc123/result.json)"
// @Success     200   {file}  file  "File contents"
// @Failure     400   {object}  ErrorResponse  "Missing or invalid path"
// @Failure     404   {object}  ErrorResponse  "File not found"
// @Router      /files [get]
func (h *Handler) DownloadFile(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query parameter 'path' is required"})
		return
	}

	hostPath, err := h.fileStore.HostPath(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if _, err := os.Stat(hostPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "file not found"})
		return
	}

	c.File(hostPath)
}

// Run godoc
// @Summary     Execute a command in a sandboxed container
// @Description Pulls the image if not cached, constructs a hardened OCI bundle, and runs the command inside a gVisor sandbox.
// @Description Files listed in `files` must be uploaded first via POST /files; each is bind-mounted read-only at /<path> inside the container.
// @Description Output files written to /output/ inside the container are captured and retrievable via GET /files?path=output/<exec_id>/<filename> only when persist=true is set; they are deleted by default.
// @Tags        execution
// @Accept      json
// @Produce     json
// @Param       request  body      RunRequest    true   "Execution parameters"
// @Success     200      {object}  RunResponse          "Command completed (any exit code is valid — check exit_code)"
// @Failure     400      {object}  ErrorResponse        "Invalid request body or file not found"
// @Failure     408      {object}  ErrorResponse        "Wall-clock timeout exceeded"
// @Failure     500      {object}  ErrorResponse        "Internal error (image pull failed, runsc error)"
// @Failure     507      {object}  ErrorResponse        "stdout or stderr exceeded the configured output limit"
// @Router      /run [post]
func (h *Handler) Run(c *gin.Context) { //nolint:gocyclo,funlen // Run covers all execution lifecycle stages in sequence
	var req RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if req.Cwd == "" {
		req.Cwd = "/"
	}
	switch req.Network {
	case "", "none", "sandbox", "host":
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid network: must be none, sandbox, or host"})
		return
	}

	limits := h.cfg.ResolveLimits(req.Limits)

	ctx := c.Request.Context()

	rootfs, err := h.cache.Rootfs(ctx, req.Image)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("image", req.Image).Msg("image pull failed")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "image pull failed: " + err.Error()})
		return
	}

	// Resolve and validate all requested input files before building the spec.
	var extraMounts []specs.Mount
	for _, filePath := range req.Files {
		hostPath, err := h.fileStore.HostPath(filePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid file path: " + err.Error()})
			return
		}
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "file not found: " + filePath})
			return
		}
		extraMounts = append(extraMounts, specs.Mount{
			Source:      hostPath,
			Destination: filepath.Clean("/" + filePath),
			Type:        "bind",
			Options:     []string{"rbind", "ro"},
		})
	}

	execID := sandbox.NewExecID()

	// For sandbox/host network modes, configure a network namespace before
	// building the OCI spec so that gVisor can join a prepared netns with a
	// veth pair, IP address, and routes already in place.
	var netnsPath string
	if req.Network == "sandbox" || req.Network == "host" {
		if os.Getuid() != 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "network=" + req.Network + " requires boxer to run as root (CAP_NET_ADMIN)",
			})
			return
		}
		netSetup, nsErr := sandbox.SetupNetwork(execID, h.cfg.ResolveDNSServers())
		if nsErr != nil {
			zerolog.Ctx(ctx).Error().Err(nsErr).Msg("network setup failed")
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "network setup failed: " + nsErr.Error()})
			return
		}
		defer netSetup.Teardown()
		netnsPath = netSetup.NetNSPath()
		extraMounts = append(extraMounts, specs.Mount{
			Source:      netSetup.ResolvConfPath(),
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Options:     []string{"rbind", "ro"},
		})
	}

	// We need the bundle's output dir for the output mount; create the bundle
	// directory structure first so we can get that path, then re-build the spec.
	// Actually we build spec first, then create bundle. To avoid a chicken-and-egg
	// situation with the output dir path, we derive it ourselves here.
	outputHostPath := sandbox.OutputPath(h.cfg.StateRoot(), execID)
	if err := os.MkdirAll(outputHostPath, 0o755); err != nil { //nolint:gosec // 0o755 required for output directory
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create output dir: " + err.Error()})
		return
	}
	// Guard against the execRoot being orphaned if Build or NewBundleDir fails.
	// Disarmed once bundle.Cleanup() takes ownership of the directory tree.
	execRoot := filepath.Dir(outputHostPath)
	bundleReady := false
	defer func() {
		if !bundleReady {
			os.RemoveAll(execRoot) //nolint:errcheck,gosec // best-effort cleanup in defer
		}
	}()

	extraMounts = append(extraMounts, specs.Mount{
		Source:      outputHostPath,
		Destination: "/output",
		Type:        "bind",
		Options:     []string{"rbind", "rw"},
	})

	spec, err := oci.NewSpecBuilder(rootfs, execID).
		WithCmd(req.Cmd).
		WithEnv(req.Env).
		WithCwd(req.Cwd).
		WithLimits(limits).
		WithNetwork(req.Network).
		WithNetworkNamespacePath(netnsPath).
		WithMounts(extraMounts).
		Build()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "spec build failed: " + err.Error()})
		return
	}

	bundle, err := sandbox.NewBundleDir(h.cfg.StateRoot(), execID, spec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "bundle setup failed: " + err.Error()})
		return
	}
	bundleReady = true
	defer bundle.Cleanup()

	var runCtx context.Context
	var cancel context.CancelFunc
	if limits.WallClockSecs != nil {
		runCtx, cancel = context.WithTimeout(ctx, secondsDuration(*limits.WallClockSecs))
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	result, err := h.executor.Run(runCtx, bundle, limits, req.Network)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("execution failed")
		status := httpStatus(err)
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}

	// Capture files written to /output inside the container.
	if captureErr := h.fileStore.CaptureOutput(execID, bundle.OutputDir()); captureErr != nil {
		zerolog.Ctx(ctx).Error().Err(captureErr).Str("exec_id", execID).Msg("failed to capture output files")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to capture output files: " + captureErr.Error()})
		return
	}

	// Clean up files unless the caller asked to keep them.
	if !req.Persist {
		for _, filePath := range req.Files {
			if delErr := h.fileStore.Delete(filePath); delErr != nil {
				zerolog.Ctx(ctx).Warn().Err(delErr).Str("path", filePath).Msg("failed to delete input file")
			}
		}
		if purgeErr := h.fileStore.PurgeOutput(execID); purgeErr != nil {
			zerolog.Ctx(ctx).Warn().Err(purgeErr).Str("exec_id", execID).Msg("failed to purge output dir")
		}
	}

	zerolog.Ctx(ctx).Info().
		Str("exec_id", execID).
		Str("image", req.Image).
		Int("exit_code", result.ExitCode).
		Int64("wall_ms", result.WallMs).
		Msg("execution complete")

	c.JSON(http.StatusOK, RunResponse{
		ExecID:   execID,
		ExitCode: result.ExitCode,
		Stdout:   string(result.Stdout),
		Stderr:   string(result.Stderr),
		WallMs:   result.WallMs,
	})
}

