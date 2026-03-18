// @title          Boxer API
// @version        1.0
// @description    Sandbox execution service: pull any container image, run arbitrary commands inside gVisor.
// @license.name   MIT
// @host           localhost:8080
// @BasePath       /

package main

import (
	"flag"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"boxer/api"
	"boxer/config"
	_ "boxer/docs"
	"boxer/image"
	"boxer/sandbox"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	flag.String("config", "", "path to config JSON file")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Ensure required directories exist.
	for _, dir := range []string{cfg.Home, cfg.StateRoot(), cfg.ImageStore(), cfg.FilesRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal().Err(err).Str("path", dir).Msg("failed to create directory")
		}
	}

	cache := image.NewImageCache(cfg.ImageStore())
	executor := sandbox.NewExecutor(cfg)
	fileStore := api.NewFileStore(cfg.FilesRoot())
	handler := api.NewHandler(cfg, cache, executor, fileStore)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	r.MaxMultipartMemory = int64(cfg.UploadLimitBytes)

	r.GET("/healthz", handler.Health)
	r.POST("/run", handler.Run)
	r.POST("/files", handler.UploadFile)
	r.GET("/files", handler.DownloadFile)
	swaggerHandler := ginSwagger.WrapHandler(swaggerFiles.Handler)
	serveSwaggerUI := func(c *gin.Context) {
		c.Request.URL.Path = "/index.html"
		swaggerFiles.Handler.ServeHTTP(c.Writer, c.Request)
	}
	r.GET("/swagger", serveSwaggerUI)
	r.GET("/swagger/*any", func(c *gin.Context) {
		if c.Param("any") == "/" {
			serveSwaggerUI(c)
			return
		}
		swaggerHandler(c)
	})

	log.Info().Str("addr", cfg.ListenAddr).Str("platform", cfg.Platform).Msg("boxer starting")
	if err := r.Run(cfg.ListenAddr); err != nil {
		log.Fatal().Err(err).Msg("server error")
	}
}

// requestLogger returns a Gin middleware that logs each request with zerolog.
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := log.Logger.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Int64("latency_ms", c.GetInt64("latency_ms")).
			Msg("request")
	}
}
