package python

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPytestWorker(t *testing.T) {
	// Allow opt-out if needed (CI, quick iteration, etc.)
	if os.Getenv("JAPAYA_SKIP_PYTEST") == "1" {
		t.Skip("JAPAYA_SKIP_PYTEST=1")
	}

	pyDir := mustPyDir(t)

	// Timeout so a hung worker/test doesn't hang `go test`.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd, err := pytestCommand(ctx)
	if err != nil {
		t.Skip(err.Error())
	}

	cmd.Dir = pyDir

	// Inherit environment; if you want to force a venv, you can adjust PATH here.
	cmd.Env = os.Environ()

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("pytest failed: %v\n\nOutput:\n%s", err, out.String())
	}
}

// Locate internal/python/py relative to this file.
func mustPyDir(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	// thisFile == .../internal/python/pytest_test.go
	pythonDir := filepath.Dir(thisFile)
	pyDir := filepath.Join(pythonDir, "py")

	if st, err := os.Stat(pyDir); err != nil || !st.IsDir() {
		t.Fatalf("expected python py dir at %q", pyDir)
	}

	return pyDir
}

// Prefer running pytest as "python -m pytest" so we don't rely on pytest being on PATH.
func pytestCommand(ctx context.Context) (*exec.Cmd, error) {
	// If user wants to force a specific python, allow it:
	// e.g. JAPAYA_PYTHON=internal/python/py/.venv/bin/python
	if forced := os.Getenv("JAPAYA_PYTHON"); forced != "" {
		if _, err := os.Stat(forced); err == nil {
			return exec.CommandContext(ctx, forced, "-m", "pytest"), nil
		}
	}

	// Try common python executables.
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "py"}
	}

	for _, exe := range candidates {
		if _, err := exec.LookPath(exe); err != nil {
			continue
		}

		// Use -m pytest; if pytest isn't installed for that interpreter, this will fail.
		return exec.CommandContext(ctx, exe, "-m", "pytest"), nil
	}

	return nil, exec.ErrNotFound
}
