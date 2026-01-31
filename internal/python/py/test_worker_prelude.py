import json
import os
import subprocess
import sys
from pathlib import Path


def run_worker(worker_path: Path, env: dict):
    # Use the current interpreter to run worker.py
    return subprocess.Popen(
        [sys.executable, "-u", str(worker_path)],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
    )


def send(proc, kind: str, code: str):
    req = {"kind": kind, "code": code}
    proc.stdin.write(json.dumps(req) + "\n")
    proc.stdin.flush()
    line = proc.stdout.readline()
    assert line, "worker returned EOF"
    return json.loads(line)


def test_prelude_stmt_exposes_public_names(tmp_path: Path):
    py_dir = tmp_path / "pydir"
    py_dir.mkdir()
    (py_dir / "__init__.py").write_text(
        "package_name = 'com.example'\n"
        "_secret = 'nope'\n"
    )

    worker_path = Path(__file__).parent / "worker.py"

    env = os.environ.copy()
    env["JAPAYA_PY_DIR"] = str(py_dir)

    proc = run_worker(worker_path, env)
    try:
        resp = send(proc, "stmt", "package_name")
        assert resp["ok"] is True
        assert resp["out"] == "com.example"

        # Private name should not be exported.
        resp = send(proc, "stmt", "_secret")
        assert resp["ok"] is False
    finally:
        proc.kill()
        proc.wait()


def test_prelude_block_can_use_public_names(tmp_path: Path):
    py_dir = tmp_path / "pydir"
    py_dir.mkdir()
    (py_dir / "__init__.py").write_text("x = 7\n")

    worker_path = Path(__file__).parent / "worker.py"

    env = os.environ.copy()
    env["JAPAYA_PY_DIR"] = str(py_dir)

    proc = run_worker(worker_path, env)
    try:
        resp = send(proc, "block", "print(x + 5)")
        assert resp["ok"] is True
        assert resp["out"] == "12\n"
    finally:
        proc.kill()
        proc.wait()
