package image

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
)

const readySentinel = ".ready"

// ImageCache manages a flat merged rootfs per image digest.
// Pull-and-unpack happens exactly once per digest; concurrent requests for the
// same image block until the first request finishes.
//
//nolint:revive // ImageCache is intentional for clarity when used as image.ImageCache
type ImageCache struct {
	storeRoot string
	mu        sync.Map // key: digest string → *cacheEntry
	digestMu  sync.Map // key: imageRef string → *digestEntry
}

type cacheEntry struct {
	mu    sync.Mutex
	ready bool
	err   error
}

type digestEntry struct {
	mu     sync.Mutex
	digest string
	err    error
}

// NewImageCache returns a cache rooted at storeRoot.
func NewImageCache(storeRoot string) *ImageCache {
	return &ImageCache{storeRoot: storeRoot}
}

// Rootfs returns the path to the merged rootfs for imageRef, pulling and
// unpacking the image if necessary.
func (c *ImageCache) Rootfs(ctx context.Context, imageRef string) (string, error) {
	digest, err := c.cachedResolveDigest(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("resolve digest for %s: %w", imageRef, err)
	}

	key := sanitizeKey(digest)
	rootfsPath := filepath.Join(c.storeRoot, key, "rootfs")
	sentinel := filepath.Join(c.storeRoot, key, readySentinel)

	// Fast path: sentinel already written.
	if _, err := os.Stat(sentinel); err == nil {
		return rootfsPath, nil
	}

	// Load or create the per-digest entry.
	actual, _ := c.mu.LoadOrStore(key, &cacheEntry{})
	entry := actual.(*cacheEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Double-check after acquiring the lock.
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

	// Pull and unpack.
	if err := c.pullAndUnpack(ctx, imageRef, rootfsPath, sentinel); err != nil {
		entry.err = err
		return "", err
	}

	entry.ready = true
	return rootfsPath, nil
}

func (c *ImageCache) pullAndUnpack(ctx context.Context, imageRef, rootfsPath, sentinel string) error {
	if err := os.MkdirAll(rootfsPath, 0o755); err != nil { //nolint:gosec // 0o755 required for rootfs directory
		return fmt.Errorf("create rootfs dir: %w", err)
	}

	layers, err := c.fetchLayers(ctx, imageRef)
	if err != nil {
		return err
	}
	defer func() {
		for _, l := range layers {
			l.Close() //nolint:errcheck,gosec // layer readers are in-memory; close errors irrelevant
		}
	}()

	if err := UnpackLayers(layers, rootfsPath); err != nil {
		return fmt.Errorf("unpack layers: %w", err)
	}

	if err := os.WriteFile(sentinel, []byte{}, 0o600); err != nil {
		return fmt.Errorf("write sentinel: %w", err)
	}
	return nil
}

// fetchLayers tries the registry first, then falls back to the local Docker daemon.
func (c *ImageCache) fetchLayers(ctx context.Context, imageRef string) ([]io.ReadCloser, error) {
	ref, parseErr := name.ParseReference(imageRef)
	isDaemonRef := parseErr == nil && (
		strings.HasPrefix(ref.Context().RegistryStr(), "localhost") ||
		strings.HasPrefix(ref.Context().RegistryStr(), "127.0.0.1"))

	if !isDaemonRef {
		layers, err := pullFromRegistry(ctx, imageRef)
		if err == nil {
			return layers, nil
		}
	}

	layers, err := pullFromDaemon(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("pull %s: registry and daemon both failed: %w", imageRef, err)
	}
	return layers, nil
}

// cachedResolveDigest resolves the digest for imageRef, caching the result so
// that only one registry request is made per unique image reference.
func (c *ImageCache) cachedResolveDigest(ctx context.Context, imageRef string) (string, error) {
	actual, _ := c.digestMu.LoadOrStore(imageRef, &digestEntry{})
	entry := actual.(*digestEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.digest != "" {
		return entry.digest, nil
	}

	digest, err := c.resolveDigest(ctx, imageRef)
	if err != nil {
		// Evict the entry so callers that arrive after this failure create a
		// fresh digestEntry rather than serialising on this one indefinitely.
		// Callers already queued on entry.mu will still retry when they acquire
		// the lock (they see digest == "" and call resolveDigest again).
		c.digestMu.Delete(imageRef)
		return "", err
	}
	entry.digest = digest
	return digest, nil
}

// resolveDigest returns a stable, filesystem-safe key for the image content.
func (c *ImageCache) resolveDigest(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parse ref: %w", err)
	}

	isDaemonRef := strings.HasPrefix(ref.Context().RegistryStr(), "localhost") ||
		strings.HasPrefix(ref.Context().RegistryStr(), "127.0.0.1")

	if !isDaemonRef {
		if d, err := resolveRegistryDigest(ctx, imageRef); err == nil {
			return d, nil
		}
	}

	return daemonDigest(ctx, imageRef)
}

// resolveRegistryDigest fetches the manifest digest from the registry.
func resolveRegistryDigest(ctx context.Context, imageRef string) (string, error) {
	img, err := pullImageRef(ctx, imageRef)
	if err != nil {
		return "", err
	}
	d, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("image digest: %w", err)
	}
	return d.String(), nil
}

// sanitizeKey converts a digest/ID into a filesystem-safe directory name.
func sanitizeKey(digest string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(digest)
}
