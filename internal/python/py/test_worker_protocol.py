import json
import subprocess
import sys
from pathlib import Path


def _worker_path() -> Path:
    return Path(__file__).parent / "worker.py"


def _start_worker() -> subprocess.Popen:
    # Use the current interpreter so the active venv is used.
    return subprocess.Popen(
        [sys.executable, "-u", str(_worker_path())],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,  # line-buffered in text mode
    )


def _send(p: subprocess.Popen, msg: dict) -> dict:
    assert p.stdin is not None
    assert p.stdout is not None

    p.stdin.write(json.dumps(msg) + "\n")
    p.stdin.flush()

    line = p.stdout.readline()
    assert line != "", "worker terminated or produced no output"
    return json.loads(line)


def _stop(p: subprocess.Popen) -> None:
    try:
        p.terminate()
        p.wait(timeout=2)
    except Exception:
        try:
            p.kill()
        finally:
            p.wait(timeout=2)


def test_subprocess_protocol_stmt_roundtrip():
    p = _start_worker()
    try:
        resp = _send(p, {"kind": "stmt", "code": '"hi"'})
        assert resp["ok"] is True
        assert resp["out"] == "hi"
        assert "stdout" in resp
        assert "stderr" in resp
    finally:
        _stop(p)


def test_subprocess_protocol_block_roundtrip():
    p = _start_worker()
    try:
        resp = _send(p, {"kind": "block", "code": "print('line1')\nprint('line2')\n"})
        assert resp["ok"] is True
        assert resp["out"] == "line1\nline2\n"
        # By design, stdout field is empty for block; out carries stdout.
        assert resp.get("stdout", "") == ""
        assert "stderr" in resp
    finally:
        _stop(p)


def test_subprocess_block_prints_json_like_text_is_safe():
    # Ensure user prints don't corrupt the worker's stdout framing.
    p = _start_worker()
    try:
        code = "print('{\"fake\": 1}')\nprint('done')\n"
        resp = _send(p, {"kind": "block", "code": code})
        assert resp["ok"] is True
        assert resp["out"] == '{"fake": 1}\ndone\n'
    finally:
        _stop(p)


def test_subprocess_invalid_json_request_returns_error():
    p = _start_worker()
    try:
        assert p.stdin is not None
        assert p.stdout is not None

        p.stdin.write("{not json}\n")
        p.stdin.flush()

        line = p.stdout.readline()
        assert line != "", "worker terminated or produced no output"
        resp = json.loads(line)
        assert resp["ok"] is False
        assert "invalid JSON request" in resp["err"]
        assert "stderr" in resp
    finally:
        _stop(p)


def test_block_no_output_returns_empty_string():
    # Unit-level quick check on behavior: exec with no prints => empty out.
    # (This isn't a subprocess test; keeps it fast.)
    import worker

    resp = worker.handle_request({"kind": "block", "code": "x = 1\n"})
    assert resp["ok"] is True
    assert resp["out"] == ""


def test_unicode_stmt_and_block_roundtrip():
    # Mix unit- and subprocess-level verification.
    import worker

    r1 = worker.handle_request({"kind": "stmt", "code": "'π → λ'"})
    assert r1["ok"] is True
    assert r1["out"] == "π → λ"

    p = _start_worker()
    try:
        resp = _send(p, {"kind": "block", "code": "print('你好')\n"})
        assert resp["ok"] is True
        assert resp["out"] == "你好\n"
    finally:
        _stop(p)
