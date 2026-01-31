#!/usr/bin/env python3
import sys
import json
import io
import os
import traceback
import importlib.util
from contextlib import redirect_stdout, redirect_stderr
from typing import Tuple

PRELUDE = {}

def load_prelude(py_dir: str) -> dict:
    init_path = os.path.join(py_dir, "__init__.py")
    if not os.path.isfile(init_path):
        return {}

    # Execute __init__.py as a module-like object.
    spec = importlib.util.spec_from_file_location("japaya_prelude", init_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"failed to load prelude from {init_path}")

    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)

    # Export public names into PRELUDE
    out = {}
    for name, val in vars(mod).items():
        if name.startswith("_"):
            continue
        out[name] = val
    return out

def run_stmt(code: str) -> str:
    """
    Evaluate `code` as a Python expression and return its string form.

    Examples:
      code: '"int x = 3;"'        -> out: 'int x = 3;'
      code: 'f"int x = {2+1};"'   -> out: 'int x = 3;'
    """
    globs = {"__builtins__": __builtins__}
    globs.update(PRELUDE)
    locs = {}
    result = eval(code, globs, locs)
    return "" if result is None else str(result)


def run_block(code: str) -> Tuple[str, str]:
    """
    Execute `code` as Python statements and return captured stdout.
    Users generate output via print().

    Example:
      print("int x = 3;")
      print("int y = 4;")
    """
    globs = {"__builtins__": __builtins__}
    globs.update(PRELUDE)
    locs = {}
    buf_out = io.StringIO()
    buf_err = io.StringIO()

    # Capture stdout/stderr from the snippet itself so protocol isn't corrupted.
    with redirect_stdout(buf_out), redirect_stderr(buf_err):
        exec(code, globs, locs)

    # NOTE: We return stdout only as OUT for block. Stderr is returned separately.
    return buf_out.getvalue(), buf_err.getvalue()


def handle_request(req: dict) -> dict:
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()

    try:
        kind = req.get("kind")
        code = req.get("code")

        if kind not in ("stmt", "block"):
            raise ValueError("kind must be 'stmt' or 'block'")
        if not isinstance(code, str):
            raise TypeError("code must be a string")

        if kind == "stmt":
            # For stmt: capture any incidental output, but 'out' is the eval result.
            with redirect_stdout(stdout_buf), redirect_stderr(stderr_buf):
                out = run_stmt(code)
            return {
                "ok": True,
                "out": out,
                "stdout": stdout_buf.getvalue(),
                "stderr": stderr_buf.getvalue(),
            }

        # kind == "block"
        # For block: run exec and return stdout as out.
        # We still separately expose stderr for debugging.
        out, snippet_stderr = run_block(code)
        return {
            "ok": True,
            "out": out,
            "stdout": "",                 # optional; keep empty since 'out' is stdout
            "stderr": snippet_stderr,     # stderr from snippet execution
        }

    except Exception as e:
        # Include traceback in stderr to help debugging.
        stderr_buf.write(traceback.format_exc())
        return {
            "ok": False,
            "err": str(e),
            "stdout": stdout_buf.getvalue(),
            "stderr": stderr_buf.getvalue(),
        }


def main() -> None:
    global PRELUDE

    # Load in the init file, if present
    py_dir = os.environ.get("JAPAYA_PY_DIR", "").strip()
    if py_dir:
        try:
            PRELUDE = load_prelude(py_dir)
        except Exception:
            sys.stderr.write(traceback.format_exc())
            sys.stderr.flush()
            raise

    # JSON-lines protocol: one request per line, one response per line.
    for raw in sys.stdin:
        line = raw.strip()
        if not line:
            continue

        try:
            req = json.loads(line)
        except Exception as e:
            resp = {
                "ok": False,
                "err": f"invalid JSON request: {e}",
                "stdout": "",
                "stderr": traceback.format_exc(),
            }
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()
            continue

        resp = handle_request(req)
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
