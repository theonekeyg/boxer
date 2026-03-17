package api

//go:generate go tool mockery --all --outpkg=mocks --output=mocks

import (
	"context"

	"boxer/config"
	"boxer/sandbox"
)

// ImageCacher resolves a container image reference to a local rootfs path.
type ImageCacher interface {
	Rootfs(ctx context.Context, imageRef string) (string, error)
}

// SandboxExecutor runs an OCI bundle inside a gVisor sandbox.
type SandboxExecutor interface {
	Run(ctx context.Context, bundle *sandbox.BundleDir, limits config.ResourceLimits) (*sandbox.Result, error)
}
