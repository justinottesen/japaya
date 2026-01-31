package python

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPytestWorker(t *testing.T) {
	if os.Getenv("JAPAYA_SKIP_PYTEST") == "1" {
		t.Skip("JAPAYA_SKIP_PYTEST=1")
	}

	root := mustRepoRoot(t)
	pyDir := filepath.Join(root, "internal", "python", "py")
	venvDir := filepath.Join(pyDir, ".venv")
	reqFile := filepath.Join(pyDir, "requirements-dev.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pyExe, err := ensureVenvPython(ctx, venvDir)
	if err != nil {
		t.Errorf("python not available to create venv: %v", err)
	}

	// Install/verify deps.
	if _, err := os.Stat(reqFile); err != nil {
		t.Fatalf("missing %s: %v", reqFile, err)
	}
	if err := pipInstall(ctx, pyExe, reqFile); err != nil {
		t.Fatalf("pip install failed: %v", err)
	}

	// Run pytest.
	if err := runPytest(ctx, pyExe, pyDir); err != nil {
		t.Fatalf("pytest failed: %v", err)
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	// Walk up until we find go.mod (or .git)
	d := filepath.Dir(thisFile)
	for {
		if d == filepath.Dir(d) {
			t.Fatal("could not find repo root (no go.mod/.git)")
		}
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if st, err := os.Stat(filepath.Join(d, ".git")); err == nil && st.IsDir() {
			return d
		}
		d = filepath.Dir(d)
	}
}

func ensureVenvPython(ctx context.Context, venvDir string) (string, error) {
	// If venv already exists, return its python.
	if exe := venvPythonPath(venvDir); exe != "" {
		return exe, nil
	}

	// Find a system python to create venv.
	sysPy, err := lookPathAny([]string{"python3", "python"})
	if err != nil {
		// Windows users might rely on "py"; include if you care.
		if runtime.GOOS == "windows" {
			sysPy, err = lookPathAny([]string{"python", "py"})
		}
		if err != nil {
			return "", err
		}
	}

	// Create venv.
	if err := os.MkdirAll(venvDir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, sysPy, "-m", "venv", venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", errors.New(string(out))
	}

	exe := venvPythonPath(venvDir)
	if exe == "" {
		return "", errors.New("venv created but python not found inside it")
	}
	return exe, nil
}

func venvPythonPath(venvDir string) string {
	// POSIX
	posix := filepath.Join(venvDir, "bin", "python")
	if st, err := os.Stat(posix); err == nil && !st.IsDir() {
		return posix
	}
	// Windows
	win := filepath.Join(venvDir, "Scripts", "python.exe")
	if st, err := os.Stat(win); err == nil && !st.IsDir() {
		return win
	}
	return ""
}

func pipInstall(ctx context.Context, pyExe, reqFile string) error {
	// Upgrade pip first (optional but helps with fresh envs)
	cmd1 := exec.CommandContext(ctx, pyExe, "-m", "pip", "install", "-q", "--upgrade", "pip")
	if out, err := cmd1.CombinedOutput(); err != nil {
		return errors.New(string(out))
	}

	cmd2 := exec.CommandContext(ctx, pyExe, "-m", "pip", "install", "-q", "-r", reqFile)
	if out, err := cmd2.CombinedOutput(); err != nil {
		return errors.New(string(out))
	}
	return nil
}

func runPytest(ctx context.Context, pyExe, pyDir string) error {
	cmd := exec.CommandContext(ctx, pyExe, "-m", "pytest", pyDir)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return errors.New(out.String())
	}
	return nil
}

func lookPathAny(names []string) (string, error) {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}
