package image

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// UnpackLayers unpacks an ordered slice of tar streams (bottom layer first)
// into destDir, applying Docker whiteout semantics.
func UnpackLayers(layers []io.ReadCloser, destDir string) error {
	for _, rc := range layers {
		if err := unpackLayer(rc, destDir); err != nil {
			return err
		}
	}
	return nil
}

// unpackLayer applies a single layer tar to destDir.
func unpackLayer(rc io.Reader, destDir string) error { //nolint:gocyclo,funlen // unpackLayer handles all OCI tar entry types and whiteout semantics
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Strip leading slash to make paths relative.
		name := strings.TrimPrefix(hdr.Name, "./")
		name = strings.TrimPrefix(name, "/")

		dir := filepath.Dir(name)
		base := filepath.Base(name)

		// Path traversal guard.
		if err := safeJoin(destDir, name); err != nil {
			return err
		}

		// Opaque whiteout: delete all existing contents of the directory.
		if base == ".wh..wh..opq" {
			parent := filepath.Join(destDir, dir)
			entries, readErr := os.ReadDir(parent)
			if readErr == nil {
				for _, e := range entries {
					os.RemoveAll(filepath.Join(parent, e.Name())) //nolint:errcheck,gosec // whiteout removal is best-effort
				}
			}
			continue
		}

		// File whiteout: delete the named file/dir.
		if suffix, ok := strings.CutPrefix(base, ".wh."); ok {
			target := filepath.Join(destDir, dir, suffix)
			os.RemoveAll(target) //nolint:errcheck,gosec // whiteout removal is best-effort
			continue
		}

		dest := filepath.Join(destDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, hdr.FileInfo().Mode()|0o111); err != nil {
				return fmt.Errorf("mkdir %s: %w", dest, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:gosec // 0o755 required for rootfs parent directories
				return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(dest), err)
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode()) //nolint:gosec // path validated by safeJoin above
			if err != nil {
				return fmt.Errorf("create file %s: %w", dest, err)
			}
			_, copyErr := io.Copy(f, tr) //nolint:gosec // decompression size bounded by available disk; container images can be large
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("write file %s: %w", dest, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close file %s: %w", dest, closeErr)
			}

		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:gosec // 0o755 required for rootfs parent directories
				return fmt.Errorf("mkdir symlink parent %s: %w", filepath.Dir(dest), err)
			}
			os.Remove(dest) //nolint:errcheck,gosec // pre-existing file removal is best-effort
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return fmt.Errorf("symlink %s → %s: %w", dest, hdr.Linkname, err)
			}

		case tar.TypeLink:
			linkTarget := filepath.Join(destDir, strings.TrimPrefix(hdr.Linkname, "/"))
			if err := safeJoin(destDir, hdr.Linkname); err != nil {
				return err
			}
			os.Remove(dest) //nolint:errcheck,gosec // pre-existing file removal is best-effort
			if err := os.Link(linkTarget, dest); err != nil {
				return fmt.Errorf("hardlink %s → %s: %w", dest, linkTarget, err)
			}

		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			// Skip device nodes — gVisor provides a virtual /dev via tmpfs.
			continue

		default:
			// Ignore unknown entry types.
			continue
		}
	}
	return nil
}

// safeJoin checks that joining destDir with relPath does not escape destDir.
func safeJoin(destDir, relPath string) error {
	// filepath.Join cleans the path (resolves "..").
	full := filepath.Join(destDir, relPath)
	if !strings.HasPrefix(full, filepath.Clean(destDir)+string(os.PathSeparator)) &&
		full != filepath.Clean(destDir) {
		return fmt.Errorf("path traversal detected: %q", relPath)
	}
	return nil
}
