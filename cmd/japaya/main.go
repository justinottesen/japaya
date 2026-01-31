package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/justinottesen/japaya/internal/core"
	"github.com/justinottesen/japaya/internal/python"
)

func main() {
	var inPath string
	var outPath string
	var pythonCmd string
	var pythonDir string

	flag.StringVar(&inPath, "in", "", "input file path")
	flag.StringVar(&outPath, "out", "", "output file path")
	flag.StringVar(&pythonCmd, "python", "", "python executable (default: python3/python)")
	flag.StringVar(&pythonDir, "python-dir", "", "directory added to Python module search path for snippets (optional)")
	flag.Parse()

	if inPath == "" || outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: japaya -in <input> -out <output> [-python <python>] [-python-dir <dir>]")
		os.Exit(2)
	}

	if pythonDir != "" {
		info, err := os.Stat(pythonDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -python-dir %q: %v\n", pythonDir, err)
			os.Exit(2)
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "invalid -python-dir %q: not a directory\n", pythonDir)
			os.Exit(2)
		}
	}

	ctx := context.Background()

	// Create the python evaluator (long-lived worker).
	py, err := python.NewEvaluator(pythonCmd, pythonDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := py.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to close python worker:", err)
		}
	}()

	if err := core.TranslatePath(ctx, inPath, outPath, py); err != nil {
		var te *core.TranslationError
		if errors.As(err, &te) {
			// print something like: file:line:col: message
			fmt.Fprintf(os.Stderr, "%s:%d:%d: %v\n",
				inPath, te.Region.Start.Line+1, te.Region.Start.Column+1, te.Err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
