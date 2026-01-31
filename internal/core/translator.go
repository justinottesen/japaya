package core

import (
	"context"
	"fmt"
)

const extraBufferPrediction = 64

func TranslateUnit(ctx context.Context, unit *TranslationUnit, py PythonEvaluator) ([]byte, error) {
	if unit == nil {
		return nil, fmt.Errorf("nil TranslationUnit")
	}
	if py == nil {
		return nil, fmt.Errorf("nil PythonEvaluator")
	}

	out := make([]byte, 0, len(unit.Data)+extraBufferPrediction)

	for _, r := range unit.Regions {
		switch r.Type {
		case RegionTypeJava:
			out = append(out, r.Data...)
		case RegionTypePythonStatement:
			fallthrough
		case RegionTypePythonBlock:
			translated, err := py.Eval(ctx, r.Type, r.Data)
			if err != nil {
				return nil, &TranslationError{Region: r, Err: err}
			}
			out = append(out, translated...)
		default:
			return nil, fmt.Errorf("unknown region type: %v", r.Type)
		}
	}

	return out, nil
}
