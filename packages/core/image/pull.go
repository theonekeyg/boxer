package image

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// pullFromRegistry fetches an image from a remote registry and returns
// an ordered slice of layer tar streams (bottom layer first).
// The caller is responsible for closing each reader.
func pullFromRegistry(ctx context.Context, imageRef string) ([]io.ReadCloser, error) {
	img, err := pullImageRef(ctx, imageRef)
	if err != nil {
		return nil, err
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	readers := make([]io.ReadCloser, 0, len(layers))
	for _, layer := range layers {
		rc, err := layer.Uncompressed()
		if err != nil {
			for _, r := range readers {
				r.Close() //nolint:errcheck,gosec // cleanup on error path
			}
			return nil, fmt.Errorf("get uncompressed layer: %w", err)
		}
		readers = append(readers, rc)
	}
	return readers, nil
}

// pullImageRef fetches the remote image object (manifest + config, no layers).
func pullImageRef(ctx context.Context, imageRef string) (v1.Image, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse image ref: %w", err)
	}
	img, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(defaultKeychain()),
	)
	if err != nil {
		return nil, fmt.Errorf("pull image: %w", err)
	}
	return img, nil
}
