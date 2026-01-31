package core

import (
	"context"
	"errors"
)

type fakePythonEvaluator struct {
	eval func(ctx context.Context, t RegionType, code []byte) ([]byte, error)
}

func (f fakePythonEvaluator) Eval(ctx context.Context, t RegionType, code []byte) ([]byte, error) {
	if f.eval == nil {
		return nil, errors.New("fakePythonEvaluator.eval is nil")
	}
	return f.eval(ctx, t, code)
}
