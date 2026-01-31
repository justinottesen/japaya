package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TranslateFile reads inPath, parses + translates it, and atomically writes to outPath.
func TranslateFile(ctx context.Context, inPath string, outPath string, py PythonEvaluator) error {
	if py == nil {
		return fmt.Errorf("nil PythonEvaluator")
	}
	if inPath == "" {
		return fmt.Errorf("empty input path")
	}
	if outPath == "" {
		return fmt.Errorf("empty output path")
	}

	in, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("open input %q: %w", inPath, err)
	}
	defer in.Close()

	outBytes, err := TranslateReader(ctx, in, py) // see below
	if err != nil {
		return fmt.Errorf("translate %q: %w", inPath, err)
	}

	if err := atomicWriteFile(outPath, outBytes, 0o644); err != nil {
		return fmt.Errorf("write output %q: %w", outPath, err)
	}
	return nil
}

// TranslateReader is the “pipeline” entry point: parse + TranslateUnit.
func TranslateReader(ctx context.Context, r io.Reader, py PythonEvaluator) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}
	if py == nil {
		return nil, fmt.Errorf("nil PythonEvaluator")
	}

	unit, err := ParseReader(r)
	if err != nil {
		return nil, err
	}
	return TranslateUnit(ctx, unit, py)
}

// atomicWriteFile writes data to a temp file in the destination directory, then renames it.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".japaya-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Best-effort cleanup on failure.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}
