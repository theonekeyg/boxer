package image

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func makeTar(entries []tarEntry) io.ReadCloser {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Size:     int64(len(e.content)),
			Mode:     0o644,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeDir {
			hdr.Mode = 0o755
		}
		tw.WriteHeader(hdr) //nolint:errcheck
		if len(e.content) > 0 {
			tw.Write(e.content) //nolint:errcheck
		}
	}
	tw.Close() //nolint:errcheck
	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

type tarEntry struct {
	name     string
	typeflag byte
	content  []byte
	linkname string
}

func TestUnpack_SimpleFile(t *testing.T) {
	dest := t.TempDir()
	layer := makeTar([]tarEntry{
		{name: "hello.txt", typeflag: tar.TypeReg, content: []byte("world")},
	})
	if err := UnpackLayers([]io.ReadCloser{layer}, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Errorf("expected 'world', got %q", got)
	}
}

func TestUnpack_WhiteoutDelete(t *testing.T) {
	dest := t.TempDir()

	// Layer 1: create a file.
	layer1 := makeTar([]tarEntry{
		{name: "toremove.txt", typeflag: tar.TypeReg, content: []byte("bye")},
	})
	// Layer 2: whiteout for that file.
	layer2 := makeTar([]tarEntry{
		{name: ".wh.toremove.txt", typeflag: tar.TypeReg},
	})

	if err := UnpackLayers([]io.ReadCloser{layer1, layer2}, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "toremove.txt")); !os.IsNotExist(err) {
		t.Error("expected toremove.txt to be deleted by whiteout")
	}
}

func TestUnpack_OpaqueWhiteout(t *testing.T) {
	dest := t.TempDir()

	layer1 := makeTar([]tarEntry{
		{name: "dir/", typeflag: tar.TypeDir},
		{name: "dir/a.txt", typeflag: tar.TypeReg, content: []byte("a")},
		{name: "dir/b.txt", typeflag: tar.TypeReg, content: []byte("b")},
	})
	layer2 := makeTar([]tarEntry{
		{name: "dir/.wh..wh..opq", typeflag: tar.TypeReg},
		{name: "dir/c.txt", typeflag: tar.TypeReg, content: []byte("c")},
	})

	if err := UnpackLayers([]io.ReadCloser{layer1, layer2}, dest); err != nil {
		t.Fatal(err)
	}
	// a.txt and b.txt should be gone.
	for _, f := range []string{"dir/a.txt", "dir/b.txt"} {
		if _, err := os.Stat(filepath.Join(dest, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s deleted by opaque whiteout", f)
		}
	}
	// c.txt added in layer2 should exist.
	if _, err := os.Stat(filepath.Join(dest, "dir/c.txt")); err != nil {
		t.Errorf("expected dir/c.txt to exist: %v", err)
	}
}

func TestUnpack_PathTraversalRejected(t *testing.T) {
	dest := t.TempDir()
	layer := makeTar([]tarEntry{
		{name: "../../etc/passwd", typeflag: tar.TypeReg, content: []byte("evil")},
	})
	err := UnpackLayers([]io.ReadCloser{layer}, dest)
	if err == nil {
		t.Error("expected path traversal to be rejected")
	}
}

func TestUnpack_Symlink(t *testing.T) {
	dest := t.TempDir()
	layer := makeTar([]tarEntry{
		{name: "target.txt", typeflag: tar.TypeReg, content: []byte("content")},
		{name: "link.txt", typeflag: tar.TypeSymlink, linkname: "target.txt"},
	})
	if err := UnpackLayers([]io.ReadCloser{layer}, dest); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(dest, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "target.txt" {
		t.Errorf("expected symlink target 'target.txt', got %q", link)
	}
}

func TestUnpack_LayerOverwrite(t *testing.T) {
	dest := t.TempDir()
	layer1 := makeTar([]tarEntry{
		{name: "file.txt", typeflag: tar.TypeReg, content: []byte("original")},
	})
	layer2 := makeTar([]tarEntry{
		{name: "file.txt", typeflag: tar.TypeReg, content: []byte("updated")},
	})
	if err := UnpackLayers([]io.ReadCloser{layer1, layer2}, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "updated" {
		t.Errorf("expected 'updated', got %q", got)
	}
}
