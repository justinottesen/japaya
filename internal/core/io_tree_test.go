package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Creates a regular file and parents.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestTranslateTree_ProcessesJavaAndJapaya_RewritesJapayaToJava(t *testing.T) {
	t.Parallel()

	inRoot := t.TempDir()
	outRoot := filepath.Join(t.TempDir(), "out")

	// Files to process
	writeFile(t, filepath.Join(inRoot, "A.java"), "public class A {}\n")
	writeFile(t, filepath.Join(inRoot, "sub", "B.japaya"), "public class B {}\n")
	writeFile(t, filepath.Join(inRoot, "sub", "C.java"), "public class C {}\n")

	// Files to ignore
	writeFile(t, filepath.Join(inRoot, "README.md"), "hi\n")
	writeFile(t, filepath.Join(inRoot, "sub", "notes.txt"), "nope\n")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called for these inputs (no python regions expected). got %v %q", t, string(code))
			return nil, nil
		},
	}

	if err := TranslateTree(context.Background(), inRoot, outRoot, py); err != nil {
		t.Fatalf("TranslateTree: %v", err)
	}

	// A.java should exist
	if _, err := os.Stat(filepath.Join(outRoot, "A.java")); err != nil {
		t.Fatalf("expected output A.java: %v", err)
	}

	// B.japaya should become B.java
	if _, err := os.Stat(filepath.Join(outRoot, "sub", "B.java")); err != nil {
		t.Fatalf("expected output sub/B.java (rewritten from .japaya): %v", err)
	}
	// and it should NOT create B.japaya
	if _, err := os.Stat(filepath.Join(outRoot, "sub", "B.japaya")); err == nil {
		t.Fatalf("did not expect output sub/B.japaya")
	}

	// C.java should exist
	if _, err := os.Stat(filepath.Join(outRoot, "sub", "C.java")); err != nil {
		t.Fatalf("expected output sub/C.java: %v", err)
	}

	// Ignored files should not exist
	if _, err := os.Stat(filepath.Join(outRoot, "README.md")); err == nil {
		t.Fatalf("did not expect output README.md")
	}
	if _, err := os.Stat(filepath.Join(outRoot, "sub", "notes.txt")); err == nil {
		t.Fatalf("did not expect output sub/notes.txt")
	}
}

func TestTranslateTree_SkipsJunkDirs(t *testing.T) {
	t.Parallel()

	inRoot := t.TempDir()
	outRoot := filepath.Join(t.TempDir(), "out")

	// Put a .java file under a skipped directory and one under normal.
	writeFile(t, filepath.Join(inRoot, ".git", "ignored.java"), "public class Ignored {}\n")
	writeFile(t, filepath.Join(inRoot, "node_modules", "ignored2.java"), "public class Ignored2 {}\n")
	writeFile(t, filepath.Join(inRoot, "bin", "ignored3.java"), "public class Ignored3 {}\n")
	writeFile(t, filepath.Join(inRoot, "dist", "ignored4.java"), "public class Ignored4 {}\n")
	writeFile(t, filepath.Join(inRoot, "ok", "Kept.java"), "public class Kept {}\n")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called for these inputs. got %v %q", t, string(code))
			return nil, nil
		},
	}

	if err := TranslateTree(context.Background(), inRoot, outRoot, py); err != nil {
		t.Fatalf("TranslateTree: %v", err)
	}

	// Kept.java should exist
	if _, err := os.Stat(filepath.Join(outRoot, "ok", "Kept.java")); err != nil {
		t.Fatalf("expected output ok/Kept.java: %v", err)
	}

	// ignored outputs should not exist
	for _, p := range []string{
		filepath.Join(outRoot, ".git", "ignored.java"),
		filepath.Join(outRoot, "node_modules", "ignored2.java"),
		filepath.Join(outRoot, "bin", "ignored3.java"),
		filepath.Join(outRoot, "dist", "ignored4.java"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("did not expect output in skipped dir: %q", p)
		}
	}
}

func TestTranslateTree_RejectsOutputInsideInput(t *testing.T) {
	t.Parallel()

	inRoot := t.TempDir()
	outRoot := filepath.Join(inRoot, "generated") // inside input

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called")
			return nil, nil
		},
	}

	err := TranslateTree(context.Background(), inRoot, outRoot, py)
	if err == nil {
		t.Fatalf("expected error for output dir inside input dir")
	}
}

func TestTranslatePath_Directory_CreatesOutDirAndTranslatesTree(t *testing.T) {
	t.Parallel()

	inRoot := t.TempDir()
	outRoot := filepath.Join(t.TempDir(), "out")

	writeFile(t, filepath.Join(inRoot, "A.japaya"), "public class A {}\n")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called")
			return nil, nil
		},
	}

	if err := TranslatePath(context.Background(), inRoot, outRoot, py); err != nil {
		t.Fatalf("TranslatePath(dir): %v", err)
	}

	// Should have created outRoot and produced A.java
	if _, err := os.Stat(filepath.Join(outRoot, "A.java")); err != nil {
		t.Fatalf("expected output A.java: %v", err)
	}
}

func TestTranslatePath_DirectoryButOutputIsFile_Errors(t *testing.T) {
	t.Parallel()

	inRoot := t.TempDir()
	outPath := filepath.Join(t.TempDir(), "out-as-file")

	// Make output a file.
	writeFile(t, outPath, "not a dir\n")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called")
			return nil, nil
		},
	}

	err := TranslatePath(context.Background(), inRoot, outPath, py)
	if err == nil {
		t.Fatalf("expected error when input is dir but output is file")
	}
}
