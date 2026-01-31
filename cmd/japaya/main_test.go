package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var buildOnce sync.Once
var builtBinPath string
var buildErr error

func TestJapaya_Success_StatementAndBlock(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Skip("python not found in PATH")
	}

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.java")
	outPath := filepath.Join(dir, "out.java")

	// This assumes your parser recognizes:
	// - `...` as a Python statement region
	// - ```...``` as a Python block region
	//
	// And that:
	// - statement "1+2" becomes "3"
	// - block prints "hi" and that stdout is spliced verbatim
	in := strings.Join([]string{
		"class A {",
		"  int x = `1+2`;",
		"  // below is a python block output:",
		"  ```print('hi', end='')```",
		"}",
		"",
	}, "\n")

	if err := os.WriteFile(inPath, []byte(in), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	res := runJapaya(t, []string{
		"-in", inPath,
		"-out", outPath,
		"-python", pythonCmd,
	})

	if res.exitCode != 0 {
		t.Fatalf("expected success (0), got %d\nstderr:\n%s", res.exitCode, res.stderr)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	want := strings.Join([]string{
		"class A {",
		"  int x = 3;",
		"  // below is a python block output:",
		"  hi",
		"}",
		"",
	}, "\n")

	if string(got) != want {
		t.Fatalf("unexpected output:\n--- want ---\n%q\n--- got ---\n%q", want, string(got))
	}
}

func TestJapaya_MissingArgs_ShowsUsage_Exit2(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Skip("python not found in PATH")
	}

	// Missing -out
	res := runJapaya(t, []string{
		"-in", "whatever",
		"-python", pythonCmd,
	})

	if res.exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d\nstderr:\n%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "usage: japaya -in <input> -out <output>") {
		t.Fatalf("expected usage in stderr, got:\n%s", res.stderr)
	}
}

func TestJapaya_PythonError_PrintsFileLineCol(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Skip("python not found in PATH")
	}

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.java")
	outPath := filepath.Join(dir, "out.java")

	// Make Python blow up. This should produce a TranslationError, and main
	// should print "file:line:col: <err>".
	in := "class A { int x = `1/0`; }\n"
	if err := os.WriteFile(inPath, []byte(in), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	res := runJapaya(t, []string{
		"-in", inPath,
		"-out", outPath,
		"-python", pythonCmd,
	})

	if res.exitCode == 0 {
		t.Fatalf("expected failure exit code, got 0")
	}

	// Loose but meaningful assertions (don’t overfit to exact traceback formatting).
	if !strings.Contains(res.stderr, inPath) {
		t.Fatalf("expected stderr to mention input path; got:\n%s", res.stderr)
	}
	// Your python worker may phrase this a bit differently; adjust if needed.
	if !strings.Contains(strings.ToLower(res.stderr), "division") && !strings.Contains(strings.ToLower(res.stderr), "zerodivision") {
		t.Fatalf("expected stderr to mention division-by-zero; got:\n%s", res.stderr)
	}

	// Should look like: /path/to/in.java:<line>:<col>:
	if !strings.Contains(res.stderr, ":") {
		t.Fatalf("expected file:line:col format; got:\n%s", res.stderr)
	}
}

type japayaResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runJapaya(t *testing.T, args []string) japayaResult {
	t.Helper()

	bin := buildJapayaBinary(t)

	cmd := exec.Command(bin, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		// Get exit code portably
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			// Failed to even start
			exitCode = 127
		}
	}

	return japayaResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
}

func buildJapayaBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		root := repoRoot(t)
		out := filepath.Join(t.TempDir(), exeName("japaya-test-bin"))

		cmd := exec.Command("go", "build", "-o", out, "./cmd/japaya")
		cmd.Dir = root

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			buildErr = err
			builtBinPath = ""
			// Include stderr in the error path for debugging.
			buildErr = &buildFailure{err: err, stderr: stderr.String()}
			return
		}

		builtBinPath = out
	})

	if buildErr != nil {
		t.Fatalf("failed to build japaya: %v", buildErr)
	}
	return builtBinPath
}

type buildFailure struct {
	err    error
	stderr string
}

func (b *buildFailure) Error() string {
	return b.err.Error() + "\n" + b.stderr
}

func repoRoot(t *testing.T) string {
	t.Helper()

	// Find the directory of this test file, then walk up to repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// this file: <repo>/cmd/japaya/main_test.go
	cmdDir := filepath.Dir(thisFile)                       // <repo>/cmd/japaya
	root := filepath.Clean(filepath.Join(cmdDir, "../..")) // <repo>
	return root
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func findPython() (string, bool) {
	// Prefer python3, then python (common on Windows).
	if p, err := exec.LookPath("python3"); err == nil {
		return p, true
	}
	if p, err := exec.LookPath("python"); err == nil {
		return p, true
	}
	return "", false
}
