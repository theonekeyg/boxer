package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"boxer/config"
	"boxer/image"
	"boxer/oci"
	"boxer/sandbox"
)

// Handler holds the dependencies injected into HTTP handlers.
type Handler struct {
	cfg      *config.BoxerConfig
	cache    *image.ImageCache
	executor *sandbox.Executor
}

// NewHandler constructs a Handler with all dependencies.
func NewHandler(cfg *config.BoxerConfig, cache *image.ImageCache, executor *sandbox.Executor) *Handler {
	return &Handler{cfg: cfg, cache: cache, executor: executor}
}

// Health handles GET /healthz.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Run handles POST /run.
func (h *Handler) Run(c *gin.Context) {
	var req RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Cwd == "" {
		req.Cwd = "/"
	}

	limits := h.cfg.ResolveLimits(req.Limits)

	ctx := c.Request.Context()

	rootfs, err := h.cache.Rootfs(ctx, req.Image)
	if err != nil {
		slog.ErrorContext(ctx, "image pull failed", "image", req.Image, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "image pull failed: " + err.Error()})
		return
	}

	execID := sandbox.NewExecID()
	spec, err := oci.NewSpecBuilder(rootfs, execID).
		WithCmd(req.Cmd).
		WithEnv(req.Env).
		WithCwd(req.Cwd).
		WithLimits(limits).
		Build()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "spec build failed: " + err.Error()})
		return
	}

	bundle, err := sandbox.NewBundleDir(h.cfg.StateRoot, execID, spec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "bundle setup failed: " + err.Error()})
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
		status := httpStatus(err)
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	slog.InfoContext(ctx, "execution complete",
		"exec_id", execID,
		"image", req.Image,
		"exit_code", result.ExitCode,
		"wall_ms", result.WallMs,
	)

	c.JSON(http.StatusOK, RunResponse{
		ExitCode: result.ExitCode,
		Stdout:   string(result.Stdout),
		Stderr:   string(result.Stderr),
		WallMs:   result.WallMs,
	})
}
