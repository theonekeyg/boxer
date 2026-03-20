package image

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// mockCache wraps ImageCache and replaces pullAndUnpack with a controllable stub.
type mockCache struct {
	ImageCache
	pullCount atomic.Int64
	pullFunc  func(rootfsPath string) error
}

func newMockCache(storeRoot string, pullFn func(rootfsPath string) error) *mockCache {
	return &mockCache{
		ImageCache: ImageCache{storeRoot: storeRoot},
		pullFunc:   pullFn,
	}
}

func (m *mockCache) pullAndUnpackMock(_ context.Context, _, rootfsPath, sentinel string) error {
	m.pullCount.Add(1)
	if err := m.pullFunc(rootfsPath); err != nil {
		return err
	}
	return os.WriteFile(sentinel, []byte{}, 0o644)
}

// rootfsMock is a test-only version of Rootfs that uses pullAndUnpackMock.
func (m *mockCache) rootfsMock(ctx context.Context, digest string) (string, error) {
	key := sanitizeKey(digest)
	rootfsPath := filepath.Join(m.storeRoot, key, "rootfs")
	sentinel := filepath.Join(m.storeRoot, key, readySentinel)

	if _, err := os.Stat(sentinel); err == nil {
		return rootfsPath, nil
	}

	actual, _ := m.mu.LoadOrStore(key, &cacheEntry{})
	entry := actual.(*cacheEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.ready {
		return rootfsPath, nil
	}
	if entry.err != nil {
		return "", entry.err
	}
	if _, err := os.Stat(sentinel); err == nil {
		entry.ready = true
		return rootfsPath, nil
	}

	if err := m.pullAndUnpackMock(ctx, "", rootfsPath, sentinel); err != nil {
		entry.err = err
		return "", err
	}
	entry.ready = true
	return rootfsPath, nil
}

func TestImageCache_PullCalledOnce(t *testing.T) {
	storeRoot := t.TempDir()
	mc := newMockCache(storeRoot, func(rootfsPath string) error {
		return os.MkdirAll(rootfsPath, 0o755)
	})

	const goroutines = 50
	const digest = "sha256:abcdef123456"

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, err := mc.rootfsMock(context.Background(), digest)
			if err != nil {
				t.Errorf("rootfsMock error: %v", err)
			}
		}()
	}
	wg.Wait()

	if count := mc.pullCount.Load(); count != 1 {
		t.Errorf("expected pull called exactly once, got %d", count)
	}
}

func TestImageCache_SentinelFastPath(t *testing.T) {
	storeRoot := t.TempDir()
	digest := "sha256:fast"
	key := sanitizeKey(digest)
	rootfsPath := filepath.Join(storeRoot, key, "rootfs")
	sentinel := filepath.Join(storeRoot, key, readySentinel)

	// Pre-create rootfs and sentinel.
	os.MkdirAll(rootfsPath, 0o755)     //nolint:errcheck
	os.WriteFile(sentinel, []byte{}, 0o644) //nolint:errcheck

	pulled := false
	mc := newMockCache(storeRoot, func(_ string) error {
		pulled = true
		return nil
	})

	path, err := mc.rootfsMock(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if path != rootfsPath {
		t.Errorf("expected %s, got %s", rootfsPath, path)
	}
	if pulled {
		t.Error("should not have pulled when sentinel exists")
	}
}

func TestSanitizeKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sha256:abcdef", "sha256-abcdef"},
		{"localhost/foo:latest", "localhost-foo-latest"},
	}
	for _, c := range cases {
		got := sanitizeKey(c.in)
		if got != c.want {
			t.Errorf("sanitizeKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestUnpackLayers_Integration verifies that UnpackLayers works end-to-end
// with actual tar streams (no network).
func TestUnpackLayers_Integration(t *testing.T) {
	importArchiveTar := func() []io.ReadCloser {
		return []io.ReadCloser{
			makeTar([]tarEntry{
				{name: "bin/", typeflag: tar.TypeDir},
				{name: "bin/sh", typeflag: tar.TypeReg, content: []byte("#!/bin/sh")},
			}),
		}
	}
	dest := t.TempDir()
	if err := UnpackLayers(importArchiveTar(), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "bin/sh")); err != nil {
		t.Error("expected bin/sh to exist")
	}
}
