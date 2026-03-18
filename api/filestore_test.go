package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	return NewFileStore(t.TempDir())
}

func TestFileStore_StoreAndRetrieve(t *testing.T) {
	fs := newTestFileStore(t)
	content := "hello, world"

	if err := fs.Store("workspace/script.py", strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	hostPath, err := fs.HostPath("workspace/script.py")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestFileStore_PathTraversal_Rejected(t *testing.T) {
	fs := newTestFileStore(t)

	cases := []string{
		"../escape",
		"../../etc/passwd",
		"foo/../../escape",
	}
	for _, p := range cases {
		if err := fs.Store(p, strings.NewReader("x")); err == nil {
			t.Errorf("expected error for traversal path %q", p)
		}
		if _, err := fs.HostPath(p); err == nil {
			t.Errorf("HostPath: expected error for traversal path %q", p)
		}
	}
}

func TestFileStore_ReservedOutputPrefix_Rejected(t *testing.T) {
	fs := newTestFileStore(t)

	cases := []string{
		"output/result.csv",
		"output/subdir/file.txt",
		"output",
	}
	for _, p := range cases {
		if err := fs.Store(p, strings.NewReader("x")); err == nil {
			t.Errorf("expected error for reserved path %q", p)
		}
		if _, err := fs.HostPath(p); err == nil {
			t.Errorf("HostPath: expected error for reserved path %q", p)
		}
	}
}

func TestFileStore_Delete(t *testing.T) {
	fs := newTestFileStore(t)

	if err := fs.Store("data/input.csv", strings.NewReader("a,b,c")); err != nil {
		t.Fatal(err)
	}
	hostPath, _ := fs.HostPath("data/input.csv")
	if _, err := os.Stat(hostPath); err != nil {
		t.Fatal("file should exist before delete")
	}

	if err := fs.Delete("data/input.csv"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hostPath); !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestFileStore_CaptureOutput(t *testing.T) {
	fs := newTestFileStore(t)
	srcDir := t.TempDir()

	// Write some files to the source dir (simulating container output).
	if err := os.WriteFile(filepath.Join(srcDir, "result.csv"), []byte("1,2,3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "log.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	execID := "boxer-test-exec-1"
	if err := fs.CaptureOutput(execID, srcDir); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"result.csv", "log.txt"} {
		destPath := filepath.Join(fs.root, "output", execID, name)
		if _, err := os.Stat(destPath); err != nil {
			t.Errorf("expected output file %s to exist: %v", name, err)
		}
	}
}

func TestFileStore_CaptureOutput_EmptyDir(t *testing.T) {
	fs := newTestFileStore(t)
	srcDir := t.TempDir()
	if err := fs.CaptureOutput("boxer-empty", srcDir); err != nil {
		t.Errorf("CaptureOutput on empty dir should not error: %v", err)
	}
}
