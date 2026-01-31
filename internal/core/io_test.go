package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile_WritesAndReplaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.java")

	// Create an existing file to ensure we replace it.
	if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("setup write old file: %v", err)
	}

	newData := []byte("new content\n")
	if err := atomicWriteFile(outPath, newData, 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != string(newData) {
		t.Fatalf("unexpected file contents:\nwant: %q\ngot:  %q", string(newData), string(got))
	}
}

func TestTranslateFile_JavaOnly_DoesNotCallEvaluator_WritesOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.java")

	// Pure Java input. Parser should treat this as Java-only.
	inData := []byte("public class A { }\n")
	if err := os.WriteFile(inPath, inData, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	// Output goes into a nested directory to verify MkdirAll behavior.
	outDir := filepath.Join(dir, "nested", "out")
	outPath := filepath.Join(outDir, "A.java")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called for Java-only input (got type %v, code %q)", t, string(code))
			return nil, nil
		},
	}

	if err := TranslateFile(context.Background(), inPath, outPath, py); err != nil {
		t.Fatalf("TranslateFile: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != string(inData) {
		t.Fatalf("unexpected output:\nwant: %q\ngot:  %q", string(inData), string(got))
	}
}

func TestTranslateFile_ArgumentValidation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.java")
	outPath := filepath.Join(dir, "out.java")

	_ = os.WriteFile(inPath, []byte("public class A {}\n"), 0o644)

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, t RegionType, code []byte) ([]byte, error) {
			return nil, nil
		},
	}

	// nil evaluator
	if err := TranslateFile(context.Background(), inPath, outPath, nil); err == nil {
		t.Fatalf("expected error for nil PythonEvaluator")
	}

	// empty input path
	if err := TranslateFile(context.Background(), "", outPath, py); err == nil {
		t.Fatalf("expected error for empty input path")
	}

	// empty output path
	if err := TranslateFile(context.Background(), inPath, "", py); err == nil {
		t.Fatalf("expected error for empty output path")
	}
}

func TestTranslateFile_MissingInputFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "does_not_exist.java")
	outPath := filepath.Join(dir, "out.java")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called if input can't be opened")
			return nil, nil
		},
	}

	err := TranslateFile(context.Background(), inPath, outPath, py)
	if err == nil {
		t.Fatalf("expected error for missing input file")
	}
}

func TestTranslateReader_ArgumentValidation(t *testing.T) {
	t.Parallel()

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, t RegionType, code []byte) ([]byte, error) {
			return nil, nil
		},
	}

	// nil reader
	_, err := TranslateReader(context.Background(), nil, py)
	if err == nil {
		t.Fatalf("expected error for nil reader")
	}

	// nil evaluator
	_, err = TranslateReader(context.Background(), &neverReadReader{}, nil)
	if err == nil {
		t.Fatalf("expected error for nil PythonEvaluator")
	}
}

// neverReadReader is just to satisfy io.Reader without providing data.
type neverReadReader struct{}

func (r *neverReadReader) Read(p []byte) (int, error) {
	return 0, errors.New("should not be read in this test")
}
