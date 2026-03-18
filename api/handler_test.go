package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	h := NewHandler(cfg, cache, exec, NewFileStore(t.TempDir()))
	r := gin.New()
	r.GET("/healthz", h.Health)
	r.POST("/run", h.Run)
	r.POST("/files", h.UploadFile)
	r.GET("/files", h.DownloadFile)
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
	cache.EXPECT().Rootfs(mock.Anything, "missing:img").Return("", fmt.Errorf("pull error"))

	r := newTestHandler(t, cache, nil)
	w := doRunRequest(t, r, runBody{Image: "missing:img", Cmd: []string{"sh"}})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRun_Timeout(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool { return b != nil && b.BundlePath() != "" }),
		config.ResourceLimits{}).
		Return(nil, fmt.Errorf("%w after 5000ms", sandbox.ErrTimeout))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusRequestTimeout {
		t.Errorf("expected 408, got %d", w.Code)
	}
}

func TestRun_OutputLimit(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool { return b != nil && b.BundlePath() != "" }),
		config.ResourceLimits{}).
		Return(nil, fmt.Errorf("%w: limit=1024 bytes", sandbox.ErrOutputLimit))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusInsufficientStorage {
		t.Errorf("expected 507, got %d", w.Code)
	}
}

func TestRun_ExecutorError(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool { return b != nil && b.BundlePath() != "" }),
		config.ResourceLimits{}).
		Return(nil, fmt.Errorf("runsc exploded"))

	r := newTestHandler(t, cache, exec)
	w := doRunRequest(t, r, runBody{Image: "python:3.12-slim", Cmd: []string{"python3"}})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRun_Success(t *testing.T) {
	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool { return b != nil && b.BundlePath() != "" }),
		config.ResourceLimits{}).
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
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool { return b != nil && b.BundlePath() != "" }),
		config.ResourceLimits{}).
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

func TestUploadFile_Success(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := doUploadRequest(t, r, "workspace/script.py", "print('hello')")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["path"] != "workspace/script.py" {
		t.Errorf("expected path=workspace/script.py, got %q", resp["path"])
	}
}

func TestUploadFile_MissingPath(t *testing.T) {
	r := newTestHandler(t, nil, nil)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "script.py")
	fw.Write([]byte("data"))
	mw.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUploadFile_TraversalPath(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := doUploadRequest(t, r, "../escape.py", "bad")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal path, got %d", w.Code)
	}
}

func TestDownloadFile_Success(t *testing.T) {
	cfg := &config.BoxerConfig{Home: t.TempDir()}
	store := NewFileStore(t.TempDir())
	store.Store("data/hello.txt", strings.NewReader("hello"))

	h := NewHandler(cfg, nil, nil, store)
	r := gin.New()
	r.GET("/files", h.DownloadFile)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/files?path=data/hello.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "hello" {
		t.Errorf("expected body 'hello', got %q", w.Body.String())
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/files?path=nonexistent.txt", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDownloadFile_MissingPath(t *testing.T) {
	r := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/files", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRun_WithFiles_MountsInjected(t *testing.T) {
	cfg := &config.BoxerConfig{Home: t.TempDir()}
	store := NewFileStore(t.TempDir())
	if err := store.Store("workspace/script.py", strings.NewReader("print(1)")); err != nil {
		t.Fatal(err)
	}

	cache := mocks.NewImageCacher(t)
	cache.EXPECT().Rootfs(mock.Anything, "python:3.12-slim").Return("/fake/rootfs", nil)

	exec := mocks.NewSandboxExecutor(t)
	exec.EXPECT().Run(
		mock.Anything,
		mock.MatchedBy(func(b *sandbox.BundleDir) bool {
			if b == nil {
				return false
			}
			spec, err := readBundleSpec(b)
			if err != nil {
				return false
			}
			hasInput, hasOutput := false, false
			for _, m := range spec.Mounts {
				if m.Destination == "/workspace/script.py" {
					hasInput = true
				}
				if m.Destination == "/output" {
					hasOutput = true
				}
			}
			return hasInput && hasOutput
		}),
		config.ResourceLimits{},
	).Return(&sandbox.Result{ExitCode: 0, Stdout: []byte("1\n")}, nil)

	h := NewHandler(cfg, cache, exec, store)
	r := gin.New()
	r.POST("/run", h.Run)

	body, _ := json.Marshal(map[string]any{
		"image":   "python:3.12-slim",
		"cmd":     []string{"python3", "/workspace/script.py"},
		"files":   []string{"workspace/script.py"},
		"persist": true,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ExecID == "" {
		t.Error("expected non-empty exec_id in response")
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

func doUploadRequest(t *testing.T, r *gin.Engine, path, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("path", path)
	fw, _ := mw.CreateFormFile("file", "upload")
	fw.Write([]byte(content))
	mw.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)
	return w
}

// readBundleSpec parses the config.json from a bundle for test assertions.
func readBundleSpec(b *sandbox.BundleDir) (*bundleSpecStub, error) {
	configPath := filepath.Join(b.BundlePath(), "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var spec bundleSpecStub
	return &spec, json.Unmarshal(data, &spec)
}

type bundleSpecStub struct {
	Mounts []struct {
		Destination string `json:"destination"`
	} `json:"mounts"`
}
