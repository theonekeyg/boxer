//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"boxer/config"
	"boxer/image"
	"boxer/sandbox"
)

func TestIntegration_RunPython(t *testing.T) {
	cfgPath := os.Getenv("BOXER_CONFIG")
	if cfgPath == "" {
		cfgPath = "../config.dev.json"
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatalf("config file not found at %s; set BOXER_CONFIG to run integration tests", cfgPath)
	}

	t.Setenv("BOXER_CONFIG", cfgPath)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	cache := image.NewImageCache(cfg.ImageStore())
	executor := sandbox.NewExecutor(cfg)
	handler := NewHandler(cfg, cache, executor)

	r := gin.New()
	r.POST("/run", handler.Run)

	body, err := json.Marshal(RunRequest{
		Image: "python:3.12-slim",
		Cmd:   []string{"python3", "-c", "print('hello')"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d (stderr: %s)", resp.ExitCode, resp.Stderr)
	}
	if resp.Stdout != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", resp.Stdout)
	}
}
