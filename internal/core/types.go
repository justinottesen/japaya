package core

import (
	"context"
	"fmt"
)

// Represents a location in a file (zero-based)
type Position struct {
	Line   uint // The line in the file (zero-based)
	Column uint // The column in the line (zero-based)
}

// Represents the type of a region
type RegionType int

const (
	RegionTypeJava            RegionType = iota // Type for a java region
	RegionTypePythonStatement                   // Type for a python statement region
	RegionTypePythonBlock                       // Type for a python code block region
)

// Represents a half-open [Start, end) region of a file
type Region struct {
	Type  RegionType // The type of data in this region
	Start Position   // The starting position of this region
	End   Position   // The ending position of this region
	Data  []byte     // The data in the region
}

// Represents a single translation unit (file)
type TranslationUnit struct {
	Data    []byte   // The data contained in the file
	Regions []Region // The mapped regions that comprise a file
}

type TranslationError struct {
	Region Region
	Err    error
}

func (e *TranslationError) Error() string {
	return fmt.Sprintf("%s at %d:%d-%d:%d: %v",
		regionTypeString(e.Region.Type),
		e.Region.Start.Line, e.Region.Start.Column,
		e.Region.End.Line, e.Region.End.Column,
		e.Err)
}

func (e *TranslationError) Unwrap() error { return e.Err }

func regionTypeString(t RegionType) string {
	switch t {
	case RegionTypeJava:
		return "java"
	case RegionTypePythonStatement:
		return "python statement"
	case RegionTypePythonBlock:
		return "python block"
	default:
		return "unknown"
	}
}

type PythonError struct {
	Message   string
	Line      *uint // line within the python snippet (0-based), if known
	Column    *uint // col within the python snippet (0-based), if known
	Traceback string
}

func (e *PythonError) Error() string { return e.Message }

// Implemented by internal/python, mocked in core tests.
type PythonEvaluator interface {
	Eval(ctx context.Context, mode RegionType, code []byte) ([]byte, error)
}
