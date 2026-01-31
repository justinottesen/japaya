package core

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TranslatePath translates either a single file or a whole directory tree.
//
// If inPath is a file: outPath must be a file path.
// If inPath is a dir : outPath must be a dir path (will be created).
func TranslatePath(ctx context.Context, inPath, outPath string, py PythonEvaluator) error {
	if py == nil {
		return fmt.Errorf("nil PythonEvaluator")
	}
	inInfo, err := os.Stat(inPath)
	if err != nil {
		return fmt.Errorf("stat input %q: %w", inPath, err)
	}

	if inInfo.IsDir() {
		// Ensure outPath is a directory (create if needed).
		if err := os.MkdirAll(outPath, 0o755); err != nil {
			return fmt.Errorf("mkdir output dir %q: %w", outPath, err)
		}
		outInfo, err := os.Stat(outPath)
		if err != nil {
			return fmt.Errorf("stat output %q: %w", outPath, err)
		}
		if !outInfo.IsDir() {
			return fmt.Errorf("input is a directory, but output %q is not a directory", outPath)
		}
		return TranslateTree(ctx, inPath, outPath, py)
	}

	// Input is a file; output must be a file (or a non-existing path).
	// If output exists and is a directory, that's an error.
	if outInfo, err := os.Stat(outPath); err == nil && outInfo.IsDir() {
		return fmt.Errorf("input is a file, but output %q is a directory", outPath)
	}

	return TranslateFile(ctx, inPath, outPath, py)
}

// TranslateTree walks inRoot recursively and writes translated output into outRoot
// preserving relative paths.
func TranslateTree(ctx context.Context, inRoot, outRoot string, py PythonEvaluator) error {
	inRoot = filepath.Clean(inRoot)
	outRoot = filepath.Clean(outRoot)

	// Prevent the classic footgun: output dir inside input dir causes infinite recursion
	// if the output is within the input tree.
	rel, err := filepath.Rel(inRoot, outRoot)
	if err == nil {
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return fmt.Errorf("output directory %q must not be inside input directory %q", outRoot, inRoot)
		}
	}

	return filepath.WalkDir(inRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip common junk dirs; adjust as you like.
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "bin" || name == "dist" {
				return fs.SkipDir
			}
			return nil
		}

		// Only process regular files.
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		// Filter which files get translated.
		if !shouldTranslatePath(path) {
			return nil
		}

		relPath, err := filepath.Rel(inRoot, path)
		if err != nil {
			return err
		}
		relPath = outputRelPath(relPath)

		outPath := filepath.Join(outRoot, relPath)

		// Ensure parent dirs exist (TranslateFile will do this if you used atomicWriteFile with MkdirAll,
		// but it doesn't hurt to keep this invariant here if you don't).
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}

		if err := TranslateFile(ctx, path, outPath, py); err != nil {
			return err
		}
		return nil
	})
}

func shouldTranslatePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".java", ".japaya": // add/remove as needed
		return true
	default:
		return false
	}
}

func outputRelPath(relPath string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext == ".japaya" {
		return strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".java"
	}
	return relPath
}

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
