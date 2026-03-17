// @title          Boxer API
// @version        1.0
// @description    Sandbox execution service: pull any container image, run arbitrary commands inside gVisor.
// @license.name   MIT
// @host           localhost:8080
// @BasePath       /

package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"boxer/api"
	"boxer/config"
	_ "boxer/docs"
	"boxer/image"
	"boxer/sandbox"
)

func main() {
	flag.String("config", "", "path to config JSON file")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Ensure required directories exist.
	for _, dir := range []string{cfg.Home, cfg.StateRoot(), cfg.ImageStore()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("failed to create directory", "path", dir, "error", err)
			os.Exit(1)
		}
	}

	cache := image.NewImageCache(cfg.ImageStore())
	executor := sandbox.NewExecutor(cfg)
	handler := api.NewHandler(cfg, cache, executor)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	r.GET("/healthz", handler.Health)
	r.POST("/run", handler.Run)
	r.GET("/swagger", func(c *gin.Context) {
		c.Request.URL.Path = "/index.html"
		swaggerFiles.Handler.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	slog.Info("boxer starting", "addr", cfg.ListenAddr, "platform", cfg.Platform)
	if err := r.Run(cfg.ListenAddr); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// requestLogger returns a Gin middleware that logs each request with slog.
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", c.GetInt64("latency_ms"),
		)
	}
}
