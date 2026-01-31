package core

import (
	"context"
	"errors"
	"testing"
)

func TestTranslateUnit_NilUnit(t *testing.T) {
	t.Parallel()

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called")
			return nil, nil
		},
	}

	_, err := TranslateUnit(context.Background(), nil, py)
	if err == nil {
		t.Fatalf("expected error for nil TranslationUnit")
	}
}

func TestTranslateUnit_NilPythonEvaluator(t *testing.T) {
	t.Parallel()

	unit := &TranslationUnit{
		Data:    []byte("doesnt matter"),
		Regions: []Region{{Type: RegionTypeJava, Data: []byte("x")}},
	}

	_, err := TranslateUnit(context.Background(), unit, nil)
	if err == nil {
		t.Fatalf("expected error for nil PythonEvaluator")
	}
}

func TestTranslateUnit_JavaOnly_Passthrough(t *testing.T) {
	t.Parallel()

	unit := &TranslationUnit{
		Data: []byte("ignored for output other than capacity"),
		Regions: []Region{
			{Type: RegionTypeJava, Data: []byte("class ")},
			{Type: RegionTypeJava, Data: []byte("A ")},
			{Type: RegionTypeJava, Data: []byte("{ }\n")},
		},
	}

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called for RegionTypeJava")
			return nil, nil
		},
	}

	out, err := TranslateUnit(context.Background(), unit, py)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "class A { }\n"
	if string(out) != want {
		t.Fatalf("unexpected output:\nwant: %q\ngot:  %q", want, string(out))
	}
}

func TestTranslateUnit_PythonRegions_AreEvaluatedAndSpliced(t *testing.T) {
	t.Parallel()

	unit := &TranslationUnit{
		Data: []byte("ignored"),
		Regions: []Region{
			{Type: RegionTypeJava, Data: []byte("int x = ")},
			{Type: RegionTypePythonStatement, Data: []byte("1+2")},
			{Type: RegionTypeJava, Data: []byte("; // ")},
			{Type: RegionTypePythonBlock, Data: []byte("print('hi')")},
			{Type: RegionTypeJava, Data: []byte("\n")},
		},
	}

	calls := []struct {
		t    RegionType
		code string
	}{}

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, t RegionType, code []byte) ([]byte, error) {
			calls = append(calls, struct {
				t    RegionType
				code string
			}{t: t, code: string(code)})

			switch t {
			case RegionTypePythonStatement:
				// pretend eval result
				return []byte("3"), nil
			case RegionTypePythonBlock:
				// pretend captured stdout
				return []byte("hi\n"), nil
			default:
				tFatalf(t, "unexpected region type to Eval: %v", t)
				return nil, nil
			}
		},
	}

	out, err := TranslateUnit(context.Background(), unit, py)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// output should be spliced in-order
	want := "int x = 3; // hi\n\n"
	if string(out) != want {
		t.Fatalf("unexpected output:\nwant: %q\ngot:  %q", want, string(out))
	}

	// verify calls happened in order with correct inputs
	if len(calls) != 2 {
		t.Fatalf("expected 2 Eval calls, got %d", len(calls))
	}
	if calls[0].t != RegionTypePythonStatement || calls[0].code != "1+2" {
		t.Fatalf("unexpected first call: %#v", calls[0])
	}
	if calls[1].t != RegionTypePythonBlock || calls[1].code != "print('hi')" {
		t.Fatalf("unexpected second call: %#v", calls[1])
	}
}

// helper so we can fatal inside the switch without shadowing testing.T
func tFatalf(t RegionType, format string, args ...any) {
	// This should never be used in passing tests; it's just a guard.
	panic("tFatalf called: " + format)
}

func TestTranslateUnit_EvalError_IsWrappedWithTranslationErrorAndRegion(t *testing.T) {
	t.Parallel()

	badRegion := Region{
		Type:  RegionTypePythonStatement,
		Start: Position{Line: 10, Column: 2},
		End:   Position{Line: 10, Column: 5},
		Data:  []byte("oops"),
	}

	unit := &TranslationUnit{
		Data: []byte("ignored"),
		Regions: []Region{
			{Type: RegionTypeJava, Data: []byte("before ")},
			badRegion,
			{Type: RegionTypeJava, Data: []byte(" after")},
		},
	}

	sentinel := errors.New("python blew up")

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			if regionType != RegionTypePythonStatement {
				t.Fatalf("expected statement eval, got %v", t)
			}
			if string(code) != "oops" {
				t.Fatalf("unexpected code: %q", string(code))
			}
			return nil, sentinel
		},
	}

	_, err := TranslateUnit(context.Background(), unit, py)
	if err == nil {
		t.Fatalf("expected error")
	}

	var te *TranslationError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TranslationError, got %T: %v", err, err)
	}

	// Region should be preserved exactly.
	if te.Region.Type != badRegion.Type ||
		te.Region.Start != badRegion.Start ||
		te.Region.End != badRegion.End ||
		string(te.Region.Data) != string(badRegion.Data) {
		t.Fatalf("wrapped region mismatch:\nwant: %#v\ngot:  %#v", badRegion, te.Region)
	}

	// If TranslationError has Unwrap, this will pass. If not, you can delete this block.
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected errors.Is(err, sentinel) to be true; implement Unwrap() on TranslationError")
	}
}

func TestTranslateUnit_UnknownRegionType(t *testing.T) {
	t.Parallel()

	unit := &TranslationUnit{
		Data: []byte("ignored"),
		Regions: []Region{
			{Type: RegionTypeJava, Data: []byte("ok")},
			{Type: RegionType(999), Data: []byte("???")},
		},
	}

	py := fakePythonEvaluator{
		eval: func(ctx context.Context, regionType RegionType, code []byte) ([]byte, error) {
			t.Fatalf("Eval should not be called for unknown region type")
			return nil, nil
		},
	}

	_, err := TranslateUnit(context.Background(), unit, py)
	if err == nil {
		t.Fatalf("expected error for unknown region type")
	}
}
