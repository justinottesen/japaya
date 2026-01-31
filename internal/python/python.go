package python

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed py/worker.py
var embeddedWorkerPy []byte

// PythonWorker is a long-lived Python worker process that evaluates snippets in an
// isolated namespace per request. This isolation will leak modules if they are
// mutable, however variables and functions used in blocks will not be leaked
type PythonWorker struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	workerDir string // temp path so we can clean up

	mu sync.Mutex

	closeOnce  sync.Once
	closeError error

	closing atomic.Bool
}

type pythonRequest struct {
	Kind string `json:"kind"`
	Code string `json:"code"`
}

type pythonResponse struct {
	OK     bool   `json:"ok"`
	Out    string `json:"out,omitempty"`
	Err    string `json:"err,omitempty"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type PythonError struct {
	Kind   string
	ErrMsg string
	Stdout string
	Stderr string
}

func (e *PythonError) Error() string {
	msg := fmt.Sprintf("python eval failed (%s): %s", e.Kind, e.ErrMsg)
	if e.Stdout != "" {
		msg += fmt.Sprintf(" [stdout=%q]", e.Stdout)
	}
	return msg
}

// Get the python executable command based on the OS
func defaultPythonCmd() string {
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

func StartPythonWorker(pythonCmd string, pythonDir string) (*PythonWorker, error) {
	// Load with defaults if not provided
	if pythonCmd == "" {
		pythonCmd = defaultPythonCmd()
	}

	// Create a temp working directory
	tmpDir, err := os.MkdirTemp("", "japaya-py-*")
	if err != nil {
		return nil, err
	}

	// Create a python file in the dir
	workerPath := filepath.Join(tmpDir, "worker.py")
	if err := os.WriteFile(workerPath, embeddedWorkerPy, 0o600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	// Create and setup the command
	cmd := exec.Command(pythonCmd, "-u", workerPath)

	// Add the python dir
	if pythonDir != "" {
		// Preserve any existing PYTHONPATH and prepend ours.
		env := os.Environ()
		const key = "PYTHONPATH="

		var had bool
		for i := range env {
			if strings.HasPrefix(env[i], key) {
				had = true
				// Prepend our dir so it wins.
				env[i] = key + pythonDir + string(os.PathListSeparator) + strings.TrimPrefix(env[i], key)
				break
			}
		}
		if !had {
			env = append(env, key+pythonDir)
		}
		cmd.Env = env

		// Add an environment variable for the dir as well
		cmd.Env = append(cmd.Env, "JAPAYA_PY_DIR="+pythonDir)
	}

	// Get stdin and stdout pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	// Construct the python object
	p := &PythonWorker{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		workerDir: tmpDir,
	}

	return p, nil
}

// Closes stdin and waits for the python process to exit
func (p *PythonWorker) Close() error {
	p.closeOnce.Do(func() {
		p.closing.Store(true)

		p.mu.Lock()
		defer p.mu.Unlock()

		_ = p.stdin.Close()
		p.closeError = p.cmd.Wait()
		_ = os.RemoveAll(p.workerDir)
	})

	return p.closeError
}

func (p *PythonWorker) IsClosed() bool {
	return p.closing.Load()
}

// Evaluate some python code
func (p *PythonWorker) Eval(ctx context.Context, kind string, code []byte) ([]byte, error) {
	// Check if python evaluator is running
	if p.IsClosed() {
		return nil, fmt.Errorf("python worker is closed")
	}

	// Validate inputs
	if ctx == nil {
		ctx = context.Background()
	}
	if kind != "stmt" && kind != "block" {
		return nil, fmt.Errorf("invalid kind %q (expected stmt|block)", kind)
	}

	// Grab mutex
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check again under the lock if we closed the worker
	if p.IsClosed() {
		return nil, fmt.Errorf("python worker is closed")
	}

	// Check for cancellation
	//
	// TODO: This is not scalable. We only check for context cancellation once we
	// have the mutex, when getting the mutex is likely to be the bottleneck when
	// we have scaled sufficiently to need cancellations
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Actually evaluate
	return p.evalOne(kind, code)
}

// Evaluate a single python snippet
//
// NOTE: This should be run under the mutex
func (p *PythonWorker) evalOne(kind string, code []byte) ([]byte, error) {
	// Create a python request from the provided code
	req := pythonRequest{
		Kind: kind,
		Code: string(code),
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')

	// Send the code to the python process
	if _, err := p.stdin.Write(line); err != nil {
		return nil, fmt.Errorf("failed writing to python worker: %w", err)
	}

	// Read the response
	respLine, err := p.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed reading from python worker: %w", err)
	}
	respLine = bytes.TrimSpace(respLine)

	// Process the response
	var resp pythonResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		s := string(respLine)
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return nil, fmt.Errorf("invalid python response JSON: %w (line=%q)", err, s)
	}

	// Return error info (if applicable)
	if !resp.OK {
		resp.Stdout = strings.ReplaceAll(resp.Stdout, "\r\n", "\n")
		resp.Stderr = strings.ReplaceAll(resp.Stderr, "\r\n", "\n")
		return nil, &PythonError{
			Kind:   kind,
			ErrMsg: resp.Err,
			Stdout: resp.Stdout,
			Stderr: resp.Stderr,
		}
	}

	return []byte(resp.Out), nil
}
