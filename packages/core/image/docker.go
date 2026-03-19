package image

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// pullFromDaemon exports an image from the local Docker daemon and returns
// the ordered layer tars (bottom layer first).
func pullFromDaemon(ctx context.Context, imageRef string) ([]io.ReadCloser, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close()

	rc, err := cli.ImageSave(ctx, []string{imageRef})
	if err != nil {
		return nil, fmt.Errorf("docker image save: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read image save: %w", err)
	}
	return extractDockerLayers(data)
}

// daemonDigest returns the image ID from the local Docker daemon,
// or an error if the image is not present.
func daemonDigest(ctx context.Context, imageRef string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}
	defer cli.Close()

	inspect, err := cli.ImageInspect(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("docker inspect %s: %w", imageRef, err)
	}
	return inspect.ID, nil
}

// dockerManifest is an entry in manifest.json inside a docker-save archive.
type dockerManifest struct {
	Layers []string `json:"Layers"`
}

// extractDockerLayers parses a Docker-save tar archive and returns layer readers.
func extractDockerLayers(archiveData []byte) ([]io.ReadCloser, error) {
	// Read manifest.json.
	var manifests []dockerManifest
	if err := readFileFromTar(archiveData, "manifest.json", func(r io.Reader) error {
		return json.NewDecoder(r).Decode(&manifests)
	}); err != nil {
		return nil, fmt.Errorf("read manifest.json: %w", err)
	}
	if len(manifests) == 0 {
		return nil, fmt.Errorf("empty manifest.json")
	}

	var layers []io.ReadCloser
	for _, layerPath := range manifests[0].Layers {
		var buf []byte
		if err := readFileFromTar(archiveData, layerPath, func(r io.Reader) error {
			var err error
			buf, err = io.ReadAll(r)
			return err
		}); err != nil {
			return nil, fmt.Errorf("read layer %s: %w", layerPath, err)
		}
		layers = append(layers, io.NopCloser(bytes.NewReader(buf)))
	}
	return layers, nil
}

// readFileFromTar calls fn with the reader for the first entry named `name`.
func readFileFromTar(data []byte, name string, fn func(io.Reader) error) error {
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("file %q not found in tar", name)
		}
		if err != nil {
			return err
		}
		if hdr.Name == name {
			return fn(tr)
		}
	}
}

// Ensure docker image types package is used.
var _ dockerimage.ListOptions
