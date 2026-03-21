package sandbox

import (
	"context"
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

// TestSetupTeardown exercises the full SetupNetwork → Teardown lifecycle.
// It requires root AND the CNI plugin binaries (bridge, host-local) to be
// installed in one of the default search directories.
func TestSetupTeardown(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root: CNI setup calls unshare(CLONE_NEWNET) and creates veth pairs")
	}

	ctx := context.Background()
	execID := "boxer-cni-test-" + t.Name()

	ns, err := SetupNetwork(ctx, execID, nil, t.TempDir())
	if err != nil {
		t.Skipf("CNI plugin binaries not available (skipping): %v", err)
	}

	path := ns.NetNSPath()
	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "netns path should exist after SetupNetwork")

	ns.Teardown(ctx)

	_, statErr = os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "netns path should be removed after Teardown")
}
