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
var buildDir string

func TestJapaya_Success_StatementAndBlock(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
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
		t.Error("python not found in PATH")
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
		t.Error("python not found in PATH")
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

		var err error
		buildDir, err = os.MkdirTemp("", "japaya-e2e-*")
		if err != nil {
			buildErr = err
			return
		}

		out := filepath.Join(buildDir, exeName("japaya-test-bin"))

		cmd := exec.Command("go", "build", "-o", out, "./cmd/japaya")
		cmd.Dir = root

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
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

func TestJapaya_TreeMode_TranslatesSubtree_RewritesJapayaToJava(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	inRoot := t.TempDir()
	outRoot := filepath.Join(t.TempDir(), "out")

	// Inputs: one .java, one .japaya in nested dirs, plus ignored files.
	mustWrite(t, filepath.Join(inRoot, "A.java"), "public class A {}\n")
	mustWrite(t, filepath.Join(inRoot, "sub", "B.japaya"), "public class B {}\n")
	mustWrite(t, filepath.Join(inRoot, "sub", "C.java"), "public class C {}\n")
	mustWrite(t, filepath.Join(inRoot, "README.md"), "ignore\n")
	mustWrite(t, filepath.Join(inRoot, "sub", "notes.txt"), "ignore\n")

	res := runJapaya(t, []string{
		"-in", inRoot,
		"-out", outRoot,
		"-python", pythonCmd,
	})

	if res.exitCode != 0 {
		t.Fatalf("expected success (0), got %d\nstderr:\n%s", res.exitCode, res.stderr)
	}

	// .java stays .java
	mustExist(t, filepath.Join(outRoot, "A.java"))
	mustExist(t, filepath.Join(outRoot, "sub", "C.java"))

	// .japaya becomes .java
	mustExist(t, filepath.Join(outRoot, "sub", "B.java"))
	mustNotExist(t, filepath.Join(outRoot, "sub", "B.japaya"))

	// ignored files not emitted
	mustNotExist(t, filepath.Join(outRoot, "README.md"))
	mustNotExist(t, filepath.Join(outRoot, "sub", "notes.txt"))
}

func TestJapaya_TreeMode_SkipsJunkDirs(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	inRoot := t.TempDir()
	outRoot := filepath.Join(t.TempDir(), "out")

	// Put eligible files under skipped dirs.
	mustWrite(t, filepath.Join(inRoot, ".git", "ignored.java"), "public class Ignored {}\n")
	mustWrite(t, filepath.Join(inRoot, "node_modules", "ignored2.java"), "public class Ignored2 {}\n")
	mustWrite(t, filepath.Join(inRoot, "bin", "ignored3.java"), "public class Ignored3 {}\n")
	mustWrite(t, filepath.Join(inRoot, "dist", "ignored4.java"), "public class Ignored4 {}\n")

	// And one normal file.
	mustWrite(t, filepath.Join(inRoot, "ok", "Kept.java"), "public class Kept {}\n")

	res := runJapaya(t, []string{
		"-in", inRoot,
		"-out", outRoot,
		"-python", pythonCmd,
	})

	if res.exitCode != 0 {
		t.Fatalf("expected success (0), got %d\nstderr:\n%s", res.exitCode, res.stderr)
	}

	mustExist(t, filepath.Join(outRoot, "ok", "Kept.java"))

	// None of these should exist.
	for _, p := range []string{
		filepath.Join(outRoot, ".git", "ignored.java"),
		filepath.Join(outRoot, "node_modules", "ignored2.java"),
		filepath.Join(outRoot, "bin", "ignored3.java"),
		filepath.Join(outRoot, "dist", "ignored4.java"),
	} {
		mustNotExist(t, p)
	}
}

func TestJapaya_TreeMode_RejectsOutputInsideInput(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	inRoot := t.TempDir()
	outRoot := filepath.Join(inRoot, "generated") // inside input dir

	// Put at least one file in the input so the tool would otherwise do something.
	mustWrite(t, filepath.Join(inRoot, "A.java"), "public class A {}\n")

	res := runJapaya(t, []string{
		"-in", inRoot,
		"-out", outRoot,
		"-python", pythonCmd,
	})

	if res.exitCode == 0 {
		t.Fatalf("expected failure exit code, got 0")
	}
	// Loose assertion: message should mention output/input relationship.
	if !strings.Contains(strings.ToLower(res.stderr), "output") || !strings.Contains(strings.ToLower(res.stderr), "input") {
		t.Fatalf("expected stderr to mention output/input constraint; got:\n%s", res.stderr)
	}
}

func TestJapaya_DirInput_FileOutput_Errors(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	inRoot := t.TempDir()
	outPath := filepath.Join(t.TempDir(), "out-as-file")

	// Make output a file, but input is a directory.
	mustWrite(t, outPath, "not a dir\n")
	mustWrite(t, filepath.Join(inRoot, "A.java"), "public class A {}\n")

	res := runJapaya(t, []string{
		"-in", inRoot,
		"-out", outPath,
		"-python", pythonCmd,
	})

	if res.exitCode == 0 {
		t.Fatalf("expected failure exit code, got 0")
	}
}

func TestJapaya_FileInput_DirOutput_Errors(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	dir := t.TempDir()
	inPath := filepath.Join(dir, "A.java")
	outDir := filepath.Join(dir, "outdir")

	mustWrite(t, inPath, "public class A {}\n")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outdir: %v", err)
	}

	res := runJapaya(t, []string{
		"-in", inPath,
		"-out", outDir,
		"-python", pythonCmd,
	})

	if res.exitCode == 0 {
		t.Fatalf("expected failure exit code, got 0")
	}
}

// Helpers local to this test file:

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %q to exist: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %q to NOT exist", path)
	}
}

func TestJapaya_Prelude_PyDirInitProvidesNames(t *testing.T) {
	t.Parallel()

	pythonCmd, ok := findPython()
	if !ok {
		t.Error("python not found in PATH")
	}

	dir := t.TempDir()

	// Create python-dir with __init__.py defining package_name
	pyDir := filepath.Join(dir, "pydir")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatalf("mkdir pyDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pyDir, "__init__.py"), []byte("package_name = 'com.example'\n"), 0o644); err != nil {
		t.Fatalf("write __init__.py: %v", err)
	}

	inPath := filepath.Join(dir, "in.japaya")
	outPath := filepath.Join(dir, "out.java")

	// Uses bare package_name (no import), relying on prelude.
	in := "package `package_name`;\n"
	if err := os.WriteFile(inPath, []byte(in), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	res := runJapaya(t, []string{
		"-in", inPath,
		"-out", outPath,
		"-python", pythonCmd,
		"-python-dir", pyDir,
	})

	if res.exitCode != 0 {
		t.Fatalf("expected success (0), got %d\nstderr:\n%s", res.exitCode, res.stderr)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	want := "package com.example;\n"
	if string(got) != want {
		t.Fatalf("unexpected output:\n--- want ---\n%q\n--- got ---\n%q", want, string(got))
	}
}
