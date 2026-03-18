package api

import (
	"context"
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
// @Description Multipart upload; form fields: file (bytes) + path (relative path)
// @Tags        files
// @Accept      multipart/form-data
// @Produce     json
// @Param       file  formData  file    true  "File to upload"
// @Param       path  formData  string  true  "Relative destination path (e.g. workspace/script.py)"
// @Success     200   {object}  map[string]string
// @Failure     400   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /files [post]
func (h *Handler) UploadFile(c *gin.Context) {
	path := c.PostForm("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "form field 'path' is required"})
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "form field 'file' is required: " + err.Error()})
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "open upload: " + err.Error()})
		return
	}
	defer f.Close()

	if err := h.fileStore.Store(path, f); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": path})
}

// DownloadFile godoc
// @Summary     Download a file from the file store
// @Description Download any file from the store by its relative path (including output/<exec_id>/...)
// @Tags        files
// @Produce     application/octet-stream
// @Param       path  query  string  true  "Relative file path"
// @Success     200
// @Failure     400  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
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
// @Description Pulls the image if not cached, then runs the command inside a
// @Description gVisor sandbox. Returns stdout, stderr, exit code, and wall time.
// @Tags        execution
// @Accept      json
// @Produce     json
// @Param       request  body      RunRequest    true  "Execution parameters"
// @Success     200      {object}  RunResponse         "Command completed (any exit code)"
// @Failure     400      {object}  ErrorResponse       "Invalid request body"
// @Failure     408      {object}  ErrorResponse       "Wall-clock timeout exceeded"
// @Failure     500      {object}  ErrorResponse       "Internal error (pull failed, runsc error)"
// @Failure     507      {object}  ErrorResponse       "Output exceeded limit"
// @Router      /run [post]
func (h *Handler) Run(c *gin.Context) {
	var req RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if req.Cwd == "" {
		req.Cwd = "/"
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
			Destination: "/" + filePath,
			Type:        "bind",
			Options:     []string{"rbind", "ro"},
		})
	}

	execID := sandbox.NewExecID()

	// We need the bundle's output dir for the output mount; create the bundle
	// directory structure first so we can get that path, then re-build the spec.
	// Actually we build spec first, then create bundle. To avoid a chicken-and-egg
	// situation with the output dir path, we derive it ourselves here.
	outputHostPath := outputDirPath(h.cfg.StateRoot(), execID)
	if err := os.MkdirAll(outputHostPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create output dir: " + err.Error()})
		return
	}
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
	defer bundle.Cleanup()

	var runCtx context.Context
	var cancel context.CancelFunc
	if limits.WallClockSecs != nil {
		runCtx, cancel = context.WithTimeout(ctx, secondsDuration(*limits.WallClockSecs))
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	result, err := h.executor.Run(runCtx, bundle, limits)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("execution failed")
		status := httpStatus(err)
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}

	// Capture files written to /output inside the container.
	if captureErr := h.fileStore.CaptureOutput(execID, bundle.OutputDir()); captureErr != nil {
		zerolog.Ctx(ctx).Warn().Err(captureErr).Str("exec_id", execID).Msg("failed to capture output files")
	}

	// Clean up input files unless the caller asked to keep them.
	if !req.Persist {
		for _, filePath := range req.Files {
			if delErr := h.fileStore.Delete(filePath); delErr != nil {
				zerolog.Ctx(ctx).Warn().Err(delErr).Str("path", filePath).Msg("failed to delete input file")
			}
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

// outputDirPath returns the path that NewBundleDir will use for the output directory,
// allowing the handler to pre-create it before building the OCI spec.
func outputDirPath(stateRoot, execID string) string {
	return filepath.Join(stateRoot, execID, "output")
}
