package python

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func pickPythonCmd(t *testing.T) string {
	t.Helper()

	// Respect your default choice first.
	candidates := []string{defaultPythonCmd()}

	// Add fallbacks (common across platforms).
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "python", "py")
	} else {
		candidates = append(candidates, "python3", "python")
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}

	t.Error("python not found on PATH (tried common python executables)")
	return ""
}

func mustStart(t *testing.T) *PythonWorker {
	t.Helper()

	cmd := pickPythonCmd(t)
	p, err := StartPythonWorker(cmd)
	if err != nil {
		t.Fatalf("StartPython(%q) error: %v", cmd, err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestStartEvalClose_BasicStmt(t *testing.T) {
	p := mustStart(t)

	out, err := p.Eval(context.Background(), "stmt", []byte(`"int x = 3;"`))
	if err != nil {
		t.Fatalf("Eval stmt error: %v", err)
	}
	if string(out) != "int x = 3;" {
		t.Fatalf("unexpected out: %q", string(out))
	}

	if p.IsClosed() {
		t.Fatalf("expected open worker")
	}
}

func TestEval_BlockReturnsStdout(t *testing.T) {
	p := mustStart(t)

	out, err := p.Eval(context.Background(), "block", []byte("print('line1')\nprint('line2')\n"))
	if err != nil {
		t.Fatalf("Eval block error: %v", err)
	}
	if string(out) != "line1\nline2\n" {
		t.Fatalf("unexpected out: %q", string(out))
	}
}

func TestEval_StmtCanPrintWithoutBreakingProtocol(t *testing.T) {
	p := mustStart(t)

	// This prints "hi\n" to snippet stdout but returns "ok" as expression result.
	out, err := p.Eval(context.Background(), "stmt", []byte(`(print("hi"), "ok")[1]`))
	if err != nil {
		t.Fatalf("Eval stmt error: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("unexpected out: %q", string(out))
	}
}

func TestEval_NewlinesInOutput_ArePreserved(t *testing.T) {
	p := mustStart(t)

	out, err := p.Eval(context.Background(), "stmt", []byte(`"a\nb\nc"`))
	if err != nil {
		t.Fatalf("Eval stmt error: %v", err)
	}
	if string(out) != "a\nb\nc" {
		t.Fatalf("unexpected out: %q", string(out))
	}
}

func TestEval_InvalidKind(t *testing.T) {
	p := mustStart(t)

	_, err := p.Eval(context.Background(), "nope", []byte("1"))
	if err == nil {
		t.Fatalf("expected error for invalid kind")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEval_NilContextDefaults(t *testing.T) {
	p := mustStart(t)

	out, err := p.Eval(context.TODO(), "stmt", []byte(`"ok"`))
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("unexpected out: %q", string(out))
	}
}

func TestEval_ContextCancelledBeforeLock(t *testing.T) {
	p := mustStart(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Eval(ctx, "stmt", []byte(`"x"`))
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	// Could be context.Canceled or wrapped; check via errors.Is.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestEval_ErrorFromWorker_Stmt(t *testing.T) {
	p := mustStart(t)

	_, err := p.Eval(context.Background(), "stmt", []byte("1/0"))
	if err == nil {
		t.Fatalf("expected error")
	}

	var pe *PythonError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
	if pe.Kind != "stmt" {
		t.Fatalf("expected kind stmt, got %q", pe.Kind)
	}
	if !strings.Contains(pe.ErrMsg, "division by zero") {
		t.Fatalf("expected division by zero, got %q", pe.ErrMsg)
	}
	if !strings.Contains(pe.Stderr, "ZeroDivisionError") {
		t.Fatalf("expected ZeroDivisionError in stderr, got %q", pe.Stderr)
	}
}

func TestEval_ErrorFromWorker_Block(t *testing.T) {
	p := mustStart(t)

	_, err := p.Eval(context.Background(), "block", []byte(`raise RuntimeError("boom")`))
	if err == nil {
		t.Fatalf("expected error")
	}

	var pe *PythonError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
	if pe.Kind != "block" {
		t.Fatalf("expected kind block, got %q", pe.Kind)
	}
	if !strings.Contains(pe.ErrMsg, "boom") {
		t.Fatalf("expected boom, got %q", pe.ErrMsg)
	}
	if !strings.Contains(pe.Stderr, "RuntimeError") {
		t.Fatalf("expected RuntimeError in stderr, got %q", pe.Stderr)
	}
}

func TestIsolation_NoVariableLeakAcrossCalls(t *testing.T) {
	p := mustStart(t)

	// Define x in one block.
	out, err := p.Eval(context.Background(), "block", []byte("x = 123\nprint('ok')\n"))
	if err != nil {
		t.Fatalf("first block error: %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("unexpected out: %q", string(out))
	}

	// Next block should not see x (fresh namespace per request).
	_, err = p.Eval(context.Background(), "block", []byte("print(x)\n"))
	if err == nil {
		t.Fatalf("expected NameError (no leak), got nil")
	}
	var pe *PythonError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
	if !strings.Contains(pe.Stderr, "NameError") {
		t.Fatalf("expected NameError in stderr, got %q", pe.Stderr)
	}
}

func TestClose_IsIdempotent_AndPreventsEval(t *testing.T) {
	p := mustStart(t)

	if err := p.Close(); err != nil {
		// Python can exit cleanly with nil; errors here are worth surfacing.
		t.Fatalf("Close error: %v", err)
	}
	// Second close should not panic and should return the same stored error (likely nil).
	_ = p.Close()

	if !p.IsClosed() {
		t.Fatalf("expected closed")
	}

	_, err := p.Eval(context.Background(), "stmt", []byte(`"x"`))
	if err == nil {
		t.Fatalf("expected error after close")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConcurrentEval_SerializesAndWorks(t *testing.T) {
	p := mustStart(t)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	errCh := make(chan error, n)

	for i := range n {
		go func() {
			defer wg.Done()

			// Give each request a small timeout so deadlocks show up fast.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			code := []byte(`f"val=` + strconv.Itoa(i) + `"`)
			out, err := p.Eval(ctx, "stmt", code)
			if err != nil {
				errCh <- err
				return
			}
			if string(out) != "val="+strconv.Itoa(i) {
				errCh <- fmt.Errorf("wrong out: got %q want %q", string(out), "val="+strconv.Itoa(i))
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent eval error: %v", err)
	}
}
