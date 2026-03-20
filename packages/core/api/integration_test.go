//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"boxer/config"
	"boxer/image"
	"boxer/sandbox"
)

func newIntegrationRouter(t *testing.T) *gin.Engine {
	t.Helper()
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
	if err := os.MkdirAll(cfg.FilesRoot(), 0o755); err != nil {
		t.Fatalf("create files root: %v", err)
	}

	cache := image.NewImageCache(cfg.ImageStore())
	executor := sandbox.NewExecutor(cfg)
	fileStore := NewFileStore(cfg.FilesRoot())
	handler := NewHandler(cfg, cache, executor, fileStore)

	r := gin.New()
	r.POST("/run", handler.Run)
	r.POST("/files", handler.UploadFile)
	r.GET("/files", handler.DownloadFile)
	return r
}

func doIntegrationUpload(t *testing.T, r *gin.Engine, path string, content []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("path", path) //nolint:errcheck
	fw, err := mw.CreateFormFile("file", "upload")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	mw.Close() //nolint:errcheck
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload %s: expected 200, got %d: %s", path, w.Code, w.Body.String())
	}
}

func TestIntegration_RunPython(t *testing.T) {
	r := newIntegrationRouter(t)

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

// TestIntegration_UploadRunAndDownloadOutput replicates the Python SDK test
// test_upload_run_and_download_output, which fails because the handler purges
// the output directory when persist=false (default), making the output file
// unavailable for download after the run completes.
func TestIntegration_UploadRunAndDownloadOutput(t *testing.T) {
	r := newIntegrationRouter(t)

	// Upload a script that writes a file to /output inside the container.
	script := []byte("import os; os.makedirs('/output', exist_ok=True); open('/output/result.txt', 'w').write('hello output')\n")
	doIntegrationUpload(t, r, "write_output.py", script)

	// Run the script without persist — default behaviour.
	body, _ := json.Marshal(RunRequest{
		Image:   "python:3.12-slim",
		Cmd:     []string{"python3", "/write_output.py"},
		Files:   []string{"write_output.py"},
		Persist: true,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("run: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal run response: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d (stderr: %s)", resp.ExitCode, resp.Stderr)
	}

	// Download the output file — this is the step that currently returns 404
	// because the handler calls fileStore.PurgeOutput when persist=false.
	outputPath := fmt.Sprintf("output/%s/result.txt", resp.ExecID)
	dw := httptest.NewRecorder()
	dreq, _ := http.NewRequest(http.MethodGet, "/files?path="+outputPath, http.NoBody)
	r.ServeHTTP(dw, dreq)
	if dw.Code != http.StatusOK {
		t.Fatalf("download output: expected 200, got %d: %s", dw.Code, dw.Body.String())
	}
	data, _ := io.ReadAll(dw.Body)
	if string(data) != "hello output" {
		t.Errorf("expected 'hello output', got %q", string(data))
	}
}
