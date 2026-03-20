package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStore manages uploaded input files and captured output files under a root directory.
//
// Layout:
//
//	<root>/
//	  workspace/script.py   ← uploaded by user with path "workspace/script.py"
//	  output/<exec_id>/     ← files written by the container to /output/
//	    result.csv
type FileStore struct {
	root string
}

// NewFileStore creates a FileStore rooted at the given directory.
func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

// Store saves the content of r into <root>/<path>. Parent directories are created
// as needed. Returns an error if path is invalid (traversal or reserved prefix).
func (s *FileStore) Store(path string, r io.Reader) error {
	if err := s.validateWritePath(path); err != nil {
		return err
	}
	abs := filepath.Join(s.root, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil { //nolint:gosec // 0o755 required for file store directories
		return fmt.Errorf("create parent dirs: %w", err)
	}
	f, err := os.Create(abs) //nolint:gosec // path validated by validateWritePath above
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close() //nolint:errcheck,gosec // superseded by copy error
		return fmt.Errorf("write file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}
	return nil
}

// HostPath validates path and returns the absolute host path.
// Returns an error on path traversal. The output/ prefix is allowed for reads.
func (s *FileStore) HostPath(path string) (string, error) {
	if err := s.validateReadPath(path); err != nil {
		return "", err
	}
	return filepath.Join(s.root, path), nil
}

// Delete removes the file at <root>/<path>.
func (s *FileStore) Delete(path string) error {
	if err := s.validateWritePath(path); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.root, path)); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}

// CaptureOutput copies all files from srcDir into <root>/output/<execID>/.
func (s *FileStore) CaptureOutput(execID, srcDir string) error {
	destDir := filepath.Join(s.root, "output", execID)
	if err := os.MkdirAll(destDir, 0o755); err != nil { //nolint:gosec // 0o755 required for output directory
		return fmt.Errorf("create output dir: %w", err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read output dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// validateReadPath checks that path does not escape the root.
func (s *FileStore) validateReadPath(path string) error {
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return fmt.Errorf("invalid path: %q", path)
	}
	return nil
}

// validateWritePath checks that path does not escape the root and does not use
// the reserved "output/" prefix (which is managed by CaptureOutput, not uploads).
func (s *FileStore) validateWritePath(path string) error {
	if err := s.validateReadPath(path); err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if clean == "output" || strings.HasPrefix(clean, "output/") || strings.HasPrefix(clean, "output"+string(filepath.Separator)) {
		return fmt.Errorf("path uses reserved prefix 'output/': %q", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // internal path from validated source directory
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close() //nolint:errcheck // source file is read-only
	out, err := os.Create(dst) //nolint:gosec // internal path from validated destination directory
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck,gosec // superseded by copy error
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dest: %w", err)
	}
	return nil
}
