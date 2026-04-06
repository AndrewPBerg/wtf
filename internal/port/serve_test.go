package port

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartDevServer_NoFramework(t *testing.T) {
	dir := t.TempDir()
	result, err := StartDevServer(dir, 8000)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestStartDevServer_StartsProcess(t *testing.T) {
	dir := t.TempDir()
	// Create a vite.config.js so the framework is detected
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte(""), 0o644))

	result, err := StartDevServer(dir, 5173)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 5173, result.Port)
	assert.Contains(t, result.Command, "5173")
	assert.Greater(t, result.PID, 0)
	assert.FileExists(t, result.LogFile)
	assert.FileExists(t, filepath.Join(dir, ".wtf-server.pid"))

	// Cleanup: stop the server
	_ = StopDevServer(dir)
	// Give it a moment to die
	time.Sleep(50 * time.Millisecond)
}

func TestStopDevServer_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	err := StopDevServer(dir)
	assert.NoError(t, err)
}

func TestStopDevServer_InvalidPIDFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wtf-server.pid"), []byte("not-a-number"), 0o644))

	err := StopDevServer(dir)
	assert.NoError(t, err)
	// PID file should be cleaned up
	assert.NoFileExists(t, filepath.Join(dir, ".wtf-server.pid"))
}

func TestStartDevServer_PortExpanded(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "next.config.js"), []byte(""), 0o644))

	result, err := StartDevServer(dir, 3042)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Command, "3042")
	assert.Equal(t, 3042, result.Port)

	_ = StopDevServer(dir)
	time.Sleep(50 * time.Millisecond)
}
