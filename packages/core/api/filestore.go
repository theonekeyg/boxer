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

// CaptureOutput copies all files from srcDir (recursively) into <root>/output/<execID>/,
// preserving the directory structure. The destination directory is created lazily —
// if the container writes no files to /output/, no directory is created in the file store.
func (s *FileStore) CaptureOutput(execID, srcDir string) error {
	destDir := filepath.Join(s.root, "output", execID)
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk output dir: %w", err)
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // skip symlinks — they may point outside the output directory on the host
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}
		dst := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // 0o755 required for output directory
			return fmt.Errorf("create output dir: %w", err)
		}
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
		return nil
	})
}

// PurgeOutput removes the output directory for the given execution ID.
// It is a no-op if no output was captured (directory does not exist).
func (s *FileStore) PurgeOutput(execID string) error {
	destDir := filepath.Join(s.root, "output", execID)
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("purge output %s: %w", execID, err)
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
