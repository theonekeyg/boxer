package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"boxer/api/mocks"
	"boxer/config"
	"boxer/sandbox"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestHandler(t *testing.T, cache ImageCacher, exec SandboxExecutor) *gin.Engine {
	t.Helper()
	cfg := &config.BoxerConfig{
		Home: t.TempDir(),
	}
	h := NewHandler(cfg, cache, exec)
	r := gin.New()
	r.GET("/healthz", h.Health)
	r.POST("/run", h.Run)
	return r
}

func TestHealth(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !body["ok"] {
		t.Errorf("expected {ok:true}, got %v", body)
	}
}

func TestRun_InvalidJSON(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/run", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRun_MissingImage(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	body, _ := json.Marshal(map[string]any{"cmd": []string{"sh"}})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRun_ImagePullFailed(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("", fmt.Errorf("pull error"))

	r := newTestHandler(t, cache, nil)
	w := doRunRequest(t, r, runBody{Image: "missing:img", Cmd: []string{"sh"}})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRun_Timeout(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w after 5000ms", sandbox.ErrTimeout))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusRequestTimeout {
		t.Errorf("expected 408, got %d", w.Code)
	}
}

func TestRun_OutputLimit(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: limit=1024 bytes", sandbox.ErrOutputLimit))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusInsufficientStorage {
		t.Errorf("expected 507, got %d", w.Code)
	}
}

func TestRun_ExecutorError(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("runsc exploded"))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRun_Success(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything).
		Return(&sandbox.Result{
			ExitCode: 0,
			Stdout:   []byte("hello\n"),
			Stderr:   []byte(""),
			WallMs:   42,
		}, nil)

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d", resp.ExitCode)
	}
	if resp.Stdout != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", resp.Stdout)
	}
	if resp.WallMs != 42 {
		t.Errorf("expected wall_ms 42, got %d", resp.WallMs)
	}
}

func TestRun_NonZeroExitCode(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, mock.Anything).Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything).
		Return(&sandbox.Result{
			ExitCode: 1,
			Stdout:   []byte(""),
			Stderr:   []byte("error\n"),
			WallMs:   10,
		}, nil)

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	// Non-zero exit code is NOT an HTTP error — the process ran, it just failed.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for non-zero exit, got %d", w.Code)
	}
	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ExitCode != 1 {
		t.Errorf("expected exit_code 1, got %d", resp.ExitCode)
	}
}

// --- helpers ---

type runBody struct {
	Image string   `json:"image"`
	Cmd   []string `json:"cmd"`
}

func doRunRequest(t *testing.T, r *gin.Engine, body runBody) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}
