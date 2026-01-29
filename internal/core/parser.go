package core

import (
	"bytes"
	"fmt"
	"io"
	"log"
)

// Pulls the bytes out of a reader, then parses using that
func ParseReader(reader io.Reader) (*TranslationUnit, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		log.Println("Encountered error reading data:", err)
		return nil, err
	}

	return ParseBytes(data)
}

// ParseBytes splits a Japaya source file into regions:
// - Java: everything not inside backticks
// - PythonStatement: ` ... ` (single backticks, must close)
// - PythonBlock: ``` ... ``` (triple backticks, must close)
//
// Positions are half-open: [Start, End).
func ParseBytes(data []byte) (*TranslationUnit, error) {
	unit := &TranslationUnit{Data: data}

	type pos struct {
		i    int  // byte offset
		line uint // 0-based
		col  uint // 0-based (bytes, not runes)
	}

	// Advance p by one byte, updating line/col.
	// (Column counts bytes. Good enough for now; revisit if you need UTF-16 columns.)
	advance1 := func(p *pos) {
		if p.i >= len(data) {
			return
		}
		if data[p.i] == '\n' {
			p.line++
			p.col = 0
		} else {
			p.col++
		}
		p.i++
	}

	// Copy a slice so Region owns its bytes.
	own := func(b []byte) []byte {
		return append([]byte(nil), b...)
	}

	emit := func(t RegionType, start pos, end pos) {
		if end.i <= start.i {
			return
		}
		unit.Regions = append(unit.Regions, Region{
			Type:  t,
			Start: Position{Line: start.line, Column: start.col},
			End:   Position{Line: end.line, Column: end.col},
			Data:  own(data[start.i:end.i]),
		})
	}

	// Find next occurrence of delim starting at offset i. Returns byte index or -1.
	indexFrom := func(hay []byte, i int, delim []byte) int {
		j := bytes.Index(hay[i:], delim)
		if j < 0 {
			return -1
		}
		return i + j
	}

	// Convert a byte offset "to" from a known position "from" by scanning bytes.
	// Used to compute end Position for regions without tracking every byte in main loop.
	// (Still linear overall because each byte is scanned a small number of times.)
	advanceTo := func(from pos, to int) pos {
		p := from
		for p.i < to {
			advance1(&p)
		}
		return p
	}

	p := pos{i: 0, line: 0, col: 0}
	javaStart := p

	for p.i < len(data) {
		// Look for a backtick. Anything else is Java.
		if data[p.i] != '`' {
			advance1(&p)
			continue
		}

		// Determine whether this is ``` or `
		isTriple := false
		if p.i+2 < len(data) && data[p.i] == '`' && data[p.i+1] == '`' && data[p.i+2] == '`' {
			isTriple = true
		}

		// Emit Java region before this delimiter
		emit(RegionTypeJava, javaStart, p)

		if isTriple {
			// Consume opening ```
			openPos := p
			advance1(&p)
			advance1(&p)
			advance1(&p)
			contentStart := p

			// Find closing ```
			closeIdx := indexFrom(data, p.i, []byte("```"))
			if closeIdx < 0 {
				return nil, &ParseError{
					Pos: Position{Line: openPos.line, Column: openPos.col},
					Msg: "unterminated python block (missing closing ```)",
				}
			}

			contentEnd := advanceTo(contentStart, closeIdx)
			emit(RegionTypePythonBlock, contentStart, contentEnd)

			// Move p past closing ```
			p = advanceTo(contentEnd, closeIdx+3)
			javaStart = p
			continue
		}

		// Single backtick statement: consume opening `
		openPos := p
		advance1(&p)
		contentStart := p

		// Find closing `
		closeIdx := indexFrom(data, p.i, []byte("`"))
		if closeIdx < 0 {
			return nil, &ParseError{
				Pos: Position{Line: openPos.line, Column: openPos.col},
				Msg: "unterminated python statement (missing closing `)",
			}
		}

		contentEnd := advanceTo(contentStart, closeIdx)
		emit(RegionTypePythonStatement, contentStart, contentEnd)

		// Move p past closing `
		p = advanceTo(contentEnd, closeIdx+1)
		javaStart = p
	}

	// Trailing Java
	emit(RegionTypeJava, javaStart, p)

	return unit, nil
}

// Optional richer error.
type ParseError struct {
	Pos Position
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, col %d: %s", e.Pos.Line, e.Pos.Column, e.Msg)
}
