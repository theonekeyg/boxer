//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRemoveNetNS(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root: createNetNS calls unshare(CLONE_NEWNET)")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "testns")

	require.NoError(t, createNetNS(path))
	_, err := os.Stat(path)
	require.NoError(t, err, "netns file should exist after createNetNS")

	removeNetNS(path)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "netns file should be gone after removeNetNS")
}

func TestCreateNetNS_DuplicatePath_Fails(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "testns")

	require.NoError(t, createNetNS(path))
	defer removeNetNS(path)

	err := createNetNS(path)
	assert.Error(t, err, "expected error when path already exists")
}

// TestSetupNetwork_InvalidExecID verifies that SetupNetwork rejects unsafe
// execID values before performing any privileged operations, so no root is
// required and no netns path is created.
func TestSetupNetwork_InvalidExecID(t *testing.T) {
	invalid := []string{
		"..",
		"../escape",
		"foo/bar",
		"foo\x00bar",
		"",
	}
	for _, id := range invalid {
		ns, err := SetupNetwork(id, []string{"8.8.8.8"})
		assert.Error(t, err, "expected error for execID %q", id)
		assert.Nil(t, ns, "expected nil NetworkSetup for execID %q", id)
	}
}

// TestSetupTeardown exercises the full SetupNetwork → Teardown lifecycle.
// Requires root and CAP_NET_ADMIN (veth creation, bridge attachment).
func TestSetupTeardown(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root: netlink veth/bridge operations need CAP_NET_ADMIN")
	}

	execID := "boxer-net-test-" + t.Name()

	ns, err := SetupNetwork(execID, []string{"8.8.8.8", "8.8.4.4"})
	require.NoError(t, err)

	path := ns.NetNSPath()
	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "netns path should exist after SetupNetwork")

	ns.Teardown()

	_, statErr = os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "netns path should be removed after Teardown")
}
