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
	"net/url"
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
	if err := mw.WriteField("path", path); err != nil {
		t.Fatalf("write path field: %v", err)
	}
	fw, err := mw.CreateFormFile("file", "upload")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "/files", &buf)
	if err != nil {
		t.Fatalf("new upload request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
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

// TestIntegration_UploadRunAndDownloadOutput uploads a script that writes a file
// to /output inside the container, runs it with persist=true, then downloads the
// captured output file and verifies its contents.
func TestIntegration_UploadRunAndDownloadOutput(t *testing.T) {
	r := newIntegrationRouter(t)

	// Upload a script that writes a file to /output inside the container.
	script := []byte("import os; os.makedirs('/output', exist_ok=True); open('/output/result.txt', 'w').write('hello output')\n")
	doIntegrationUpload(t, r, "write_output.py", script)

	// Run the script with persist=true so the output directory is not purged.
	body, err := json.Marshal(RunRequest{
		Image:   "python:3.12-slim",
		Cmd:     []string{"python3", "/write_output.py"},
		Files:   []string{"write_output.py"},
		Persist: true,
	})
	if err != nil {
		t.Fatalf("marshal run request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new run request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
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

	// Download the captured output file. This succeeds because Persist: true
	// prevents the handler from calling fileStore.PurgeOutput after the run.
	params := url.Values{"path": []string{fmt.Sprintf("output/%s/result.txt", resp.ExecID)}}
	dreq, err := http.NewRequest(http.MethodGet, "/files?"+params.Encode(), http.NoBody)
	if err != nil {
		t.Fatalf("new download request: %v", err)
	}
	dw := httptest.NewRecorder()
	r.ServeHTTP(dw, dreq)
	if dw.Code != http.StatusOK {
		t.Fatalf("download output: expected 200, got %d: %s", dw.Code, dw.Body.String())
	}
	data, err := io.ReadAll(dw.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	if string(data) != "hello output" {
		t.Errorf("expected 'hello output', got %q", string(data))
	}
}
