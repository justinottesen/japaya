package core

import (
	"errors"
	"testing"
)

type wantRegion struct {
	typ        RegionType
	startLine  uint
	startCol   uint
	endLine    uint
	endCol     uint
	dataString string
}

func TestParseBytes_Regions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []wantRegion
	}{
		{
			name: "java_only_single_region",
			in:   "class A {}\n",
			want: []wantRegion{
				{
					typ:       RegionTypeJava,
					startLine: 0, startCol: 0,
					endLine: 1, endCol: 0,
					dataString: "class A {}\n",
				},
			},
		},
		{
			name: "python_statement_only",
			in:   "`x`",
			want: []wantRegion{
				{
					typ:       RegionTypePythonStatement,
					startLine: 0, startCol: 1,
					endLine: 0, endCol: 2,
					dataString: "x",
				},
			},
		},
		{
			name: "mixed_java_statement_java",
			in:   "a `x` b",
			want: []wantRegion{
				{
					typ:       RegionTypeJava,
					startLine: 0, startCol: 0,
					endLine: 0, endCol: 2,
					dataString: "a ",
				},
				{
					typ:       RegionTypePythonStatement,
					startLine: 0, startCol: 3,
					endLine: 0, endCol: 4,
					dataString: "x",
				},
				{
					typ:       RegionTypeJava,
					startLine: 0, startCol: 5,
					endLine: 0, endCol: 7,
					dataString: " b",
				},
			},
		},
		{
			name: "python_block_with_newlines",
			in:   "A ```\npy\n``` Z",
			want: []wantRegion{
				{
					typ:       RegionTypeJava,
					startLine: 0, startCol: 0,
					endLine: 0, endCol: 2,
					dataString: "A ",
				},
				// After "A " we see "```" then content starts immediately after those 3 bytes.
				// Input: "A ```\npy\n``` Z"
				// Content is "\npy\n" (from after opening ``` up to before closing ```)
				{
					typ:       RegionTypePythonBlock,
					startLine: 0, startCol: 5, // after "A " (2) + "```" (3) => col 5
					endLine: 2, endCol: 0, // content ends right at the start of the closing ``` on line 2
					dataString: "\npy\n",
				},
				{
					typ:       RegionTypeJava,
					startLine: 2, startCol: 3, // after closing ``` (3 bytes) on line 2
					endLine: 2, endCol: 5,
					dataString: " Z",
				},
			},
		},
		{
			name: "adjacent_python_statements_no_empty_java_regions",
			in:   "`a``b`",
			want: []wantRegion{
				{
					typ:       RegionTypePythonStatement,
					startLine: 0, startCol: 1,
					endLine: 0, endCol: 2,
					dataString: "a",
				},
				{
					typ:       RegionTypePythonStatement,
					startLine: 0, startCol: 4, // "`a`" is 3 bytes, next open backtick is at col 3, so content at col 4
					endLine: 0, endCol: 5,
					dataString: "b",
				},
			},
		},
		{
			name: "empty_python_statement",
			in:   "``",
			want: []wantRegion{
				// content is empty; your parser emits only if end.i > start.i.
				// In current implementation it will emit nothing (no Java either), so expect zero regions.
			},
		},
		{
			name: "empty_python_block",
			in:   "``````", // opening ``` immediately followed by closing ```
			want: []wantRegion{
				// content is empty; current implementation will emit nothing for python region.
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unit, err := ParseBytes([]byte(tc.in))
			if err != nil {
				t.Fatalf("ParseBytes returned error: %v", err)
			}
			if unit == nil {
				t.Fatalf("ParseBytes returned nil unit")
			}

			if got, want := string(unit.Data), tc.in; got != want {
				t.Fatalf("unit.Data mismatch:\n got: %q\nwant: %q", got, want)
			}

			if got, want := len(unit.Regions), len(tc.want); got != want {
				t.Fatalf("region count mismatch: got %d, want %d\nregions: %#v", got, want, unit.Regions)
			}

			for i := range tc.want {
				g := unit.Regions[i]
				w := tc.want[i]

				if g.Type != w.typ {
					t.Fatalf("region %d type mismatch: got %v, want %v", i, g.Type, w.typ)
				}
				if g.Start.Line != w.startLine || g.Start.Column != w.startCol {
					t.Fatalf("region %d start mismatch: got (%d,%d), want (%d,%d)",
						i, g.Start.Line, g.Start.Column, w.startLine, w.startCol)
				}
				if g.End.Line != w.endLine || g.End.Column != w.endCol {
					t.Fatalf("region %d end mismatch: got (%d,%d), want (%d,%d)",
						i, g.End.Line, g.End.Column, w.endLine, w.endCol)
				}
				if got, want := string(g.Data), w.dataString; got != want {
					t.Fatalf("region %d data mismatch:\n got: %q\nwant: %q", i, got, want)
				}
			}
		})
	}
}

func TestParseBytes_UnterminatedStatementError(t *testing.T) {
	t.Parallel()

	_, err := ParseBytes([]byte("x\n`a"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}

	// Opening backtick is at line 1, col 0 in "x\n`a"
	if pe.Pos.Line != 1 || pe.Pos.Column != 0 {
		t.Fatalf("parse error position mismatch: got (%d,%d), want (1,0)", pe.Pos.Line, pe.Pos.Column)
	}
}

func TestParseBytes_UnterminatedBlockError(t *testing.T) {
	t.Parallel()

	_, err := ParseBytes([]byte("```abc"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}

	// Opening ``` is at line 0, col 0
	if pe.Pos.Line != 0 || pe.Pos.Column != 0 {
		t.Fatalf("parse error position mismatch: got (%d,%d), want (0,0)", pe.Pos.Line, pe.Pos.Column)
	}
}
