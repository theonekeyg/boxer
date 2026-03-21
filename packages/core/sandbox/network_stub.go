//go:build !linux

package sandbox

import "errors"

// errNotLinux is returned by all network functions on non-Linux platforms.
var errNotLinux = errors.New("network namespaces are only supported on Linux")

// NetworkSetup is the non-Linux stub type. On Linux this struct holds live
// kernel resources; here it exists only so the package compiles.
type NetworkSetup struct{}

// NetNSPath always returns an empty string on non-Linux platforms.
func (n *NetworkSetup) NetNSPath() string { return "" }

// ResolvConfPath always returns an empty string on non-Linux platforms.
func (n *NetworkSetup) ResolvConfPath() string { return "" }

// Teardown is a no-op on non-Linux platforms.
func (n *NetworkSetup) Teardown() {}

// SetupNetwork always returns an error on non-Linux platforms.
func SetupNetwork(_ string) (*NetworkSetup, error) {
	return nil, errNotLinux
}
