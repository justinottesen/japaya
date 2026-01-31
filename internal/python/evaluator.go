package python

import (
	"context"
	"fmt"

	"github.com/justinottesen/japaya/internal/core"
)

// Evaluator implements core.PythonEvaluator using the existing python worker wrapper.
type Evaluator struct {
	// Put your existing wrapper type here.
	// For example, if python.go exposes `type Worker struct {...}`,
	// store `w *Worker`.
	w *PythonWorker
}

func NewEvaluator(pythonCmd string, pythonDir string) (*Evaluator, error) {
	// Construct your existing wrapper here.
	w, err := StartPythonWorker(pythonCmd, pythonDir)
	if err != nil {
		return nil, err
	}
	return &Evaluator{w: w}, nil
}

func (e *Evaluator) Close() error {
	// Close/kill the worker subprocess cleanly.
	return e.w.Close() // <-- rename to your real close
}

// Eval satisfies core.PythonEvaluator.
// It maps RegionTypePythonStatement -> stmt mode, RegionTypePythonBlock -> block mode.
func (e *Evaluator) Eval(ctx context.Context, t core.RegionType, code []byte) ([]byte, error) {
	switch t {
	case core.RegionTypePythonStatement:
		return e.w.Eval(ctx, "stmt", code)
	case core.RegionTypePythonBlock:
		return e.w.Eval(ctx, "block", code)
	default:
		return nil, fmt.Errorf("python evaluator received non-python region type: %v", t)
	}
}
