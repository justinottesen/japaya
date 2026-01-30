import json
import pytest

import worker


def test_run_stmt_simple_string():
    assert worker.run_stmt('"int x = 3;"') == "int x = 3;"


def test_run_stmt_fstring():
    assert worker.run_stmt('f"int x = {2+1};"') == "int x = 3;"


def test_run_stmt_none_becomes_empty_string():
    # Expression evaluates to None => ""
    assert worker.run_stmt("None") == ""


def test_run_stmt_print_is_captured_by_handle_request_not_returned():
    # run_stmt itself doesn't capture stdout; handle_request does.
    resp = worker.handle_request({"kind": "stmt", "code": '(print("hi"), "ok")[1]'})
    assert resp["ok"] is True
    assert resp["out"] == "ok"
    assert resp["stdout"] == "hi\n"
    assert resp["stderr"] == ""


def test_run_block_captures_stdout():
    out, err = worker.run_block("print('a')\nprint('b')\n")
    assert out == "a\nb\n"
    assert err == ""


def test_run_block_captures_stderr():
    out, err = worker.run_block("import sys\nprint('x')\nprint('y', file=sys.stderr)\n")
    assert out == "x\n"
    assert err == "y\n"


def test_handle_request_stmt_returns_eval_result_and_captured_output():
    resp = worker.handle_request({"kind": "stmt", "code": '"hello"'})
    assert resp == {"ok": True, "out": "hello", "stdout": "", "stderr": ""}


def test_handle_request_block_returns_stdout_as_out_and_stderr_separately():
    resp = worker.handle_request({"kind": "block", "code": "print('line1')\nprint('line2')\n"})
    assert resp["ok"] is True
    assert resp["out"] == "line1\nline2\n"
    assert resp["stdout"] == ""   # by design
    assert resp["stderr"] == ""


def test_handle_request_block_stderr_goes_to_stderr_field():
    resp = worker.handle_request(
        {"kind": "block", "code": "import sys\nprint('ok')\nprint('bad', file=sys.stderr)\n"}
    )
    assert resp["ok"] is True
    assert resp["out"] == "ok\n"
    assert resp["stderr"] == "bad\n"


@pytest.mark.parametrize(
    "req, expected_err_substr",
    [
        ({"kind": "nope", "code": "1"}, "kind must be 'stmt' or 'block'"),
        ({"kind": "stmt", "code": 123}, "code must be a string"),
        ({"kind": "block", "code": None}, "code must be a string"),
        ({}, "kind must be 'stmt' or 'block'"),
    ],
)
def test_handle_request_validation_errors(req, expected_err_substr):
    resp = worker.handle_request(req)
    assert resp["ok"] is False
    assert expected_err_substr in resp["err"]
    assert "Traceback" in resp["stderr"]


def test_handle_request_stmt_exception_includes_traceback():
    resp = worker.handle_request({"kind": "stmt", "code": "1/0"})
    assert resp["ok"] is False
    assert "division by zero" in resp["err"]
    assert "ZeroDivisionError" in resp["stderr"]
    assert "Traceback" in resp["stderr"]


def test_handle_request_block_exception_includes_traceback():
    resp = worker.handle_request({"kind": "block", "code": "raise RuntimeError('boom')"})
    assert resp["ok"] is False
    assert "boom" in resp["err"]
    assert "RuntimeError" in resp["stderr"]
    assert "Traceback" in resp["stderr"]


def test_isolation_no_variable_leak_between_requests_block():
    # First request defines a variable, second should not see it.
    r1 = worker.handle_request({"kind": "block", "code": "x = 123\nprint('ok')\n"})
    assert r1["ok"] is True
    assert r1["out"] == "ok\n"

    r2 = worker.handle_request({"kind": "block", "code": "print(x)\n"})
    assert r2["ok"] is False
    assert "NameError" in r2["stderr"]


def test_isolation_no_variable_leak_between_requests_stmt():
    r1 = worker.handle_request({"kind": "stmt", "code": "x = 5"})  # invalid: eval can't assign
    assert r1["ok"] is False
    assert "SyntaxError" in r1["stderr"]

    r2 = worker.handle_request({"kind": "stmt", "code": "globals().get('x')"})
    assert r2["ok"] is True
    assert r2["out"] == ""   # None -> ""

def test_newlines_preserved_in_stmt_output():
    resp = worker.handle_request({"kind": "stmt", "code": "'a\\nb\\nc'"})
    assert resp["ok"] is True
    assert resp["out"] == "a\nb\nc"


def test_newlines_preserved_in_block_output():
    resp = worker.handle_request({"kind": "block", "code": "print('a')\nprint('b')\n"})
    assert resp["ok"] is True
    assert resp["out"] == "a\nb\n"


def test_handle_request_ignores_extra_fields():
    # Ensure protocol is robust to extra keys.
    resp = worker.handle_request({"kind": "stmt", "code": "41+1", "extra": "ignored"})
    assert resp["ok"] is True
    assert resp["out"] == "42"


def test_main_json_roundtrip_smoke():
    """
    This doesn't spawn a subprocess; it just verifies that responses are JSON-serializable
    and contain expected fields. Full stdin/stdout framing should be tested from Go.
    """
    resp = worker.handle_request({"kind": "block", "code": "print('x')"})
    encoded = json.dumps(resp)
    decoded = json.loads(encoded)
    assert decoded["ok"] is True
    assert decoded["out"] == "x\n"
