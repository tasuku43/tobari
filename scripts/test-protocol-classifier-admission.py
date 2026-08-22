#!/usr/bin/env python3
"""Negative contract tests for protocol-classifier admission validation."""

from __future__ import annotations

import json
import shutil
import tempfile
from contextlib import contextmanager
from pathlib import Path

from protocol_classifier_admission import AdmissionError, validate


ROOT = Path(__file__).resolve().parents[1]


@contextmanager
def fixture():
    with tempfile.TemporaryDirectory(prefix="tobari-classifier-admission-") as temporary:
        target = Path(temporary)
        (target / ".harness").mkdir()
        shutil.copy2(
            ROOT / ".harness/protocol_classifier_admission.json",
            target / ".harness/protocol_classifier_admission.json",
        )
        shutil.copytree(ROOT / "gateway", target / "gateway")
        policy_target = target / "internal/infra/runtimeassets/assets/opa/policy"
        policy_target.mkdir(parents=True)
        shutil.copy2(
            ROOT / "internal/infra/runtimeassets/assets/opa/policy/tobari_test.rego",
            policy_target / "tobari_test.rego",
        )
        yield target


def manifest(root: Path):
    path = root / ".harness/protocol_classifier_admission.json"
    document = json.loads(path.read_text(encoding="utf-8"))
    return path, document


def reject(label: str, mutate, expected: str) -> None:
    with fixture() as root:
        mutate(root)
        try:
            validate(root)
        except AdmissionError as error:
            if expected not in str(error):
                raise AssertionError(f"{label}: wrong failure: {error}") from error
        else:
            raise AssertionError(f"{label}: invalid fixture passed")


def main() -> int:
    validate(ROOT)

    def remove_row(root: Path) -> None:
        path, document = manifest(root)
        del document["classifiers"]["aws"]
        path.write_text(json.dumps(document), encoding="utf-8")

    def remove_dimension(root: Path) -> None:
        path, document = manifest(root)
        del document["classifiers"]["git"]["evidence"]["collision"]
        path.write_text(json.dumps(document), encoding="utf-8")

    def break_reference(root: Path) -> None:
        path, document = manifest(root)
        document["classifiers"]["oci"]["evidence"]["positive"] = [
            "python:gateway/test_oci_request.py::OCIRequestTests::test_missing"
        ]
        path.write_text(json.dumps(document), encoding="utf-8")

    def remove_corpus(root: Path) -> None:
        path, document = manifest(root)
        document["classifiers"]["mcp"]["evidence"]["bounded_adversarial_corpus"] = [
            "python:gateway/test_mcp_request.py::MCPRequestTest::test_tools_call_retains_only_exact_tool_name"
        ]
        path.write_text(json.dumps(document), encoding="utf-8")

    def add_io(root: Path) -> None:
        path = root / "gateway/addon/graphql_request.py"
        path.write_text(path.read_text(encoding="utf-8") + "\nimport socket\n", encoding="utf-8")

    reject("missing row", remove_row, "classifier inventory differs")
    reject("missing dimension", remove_dimension, "evidence dimensions differ")
    reject("missing evidence", break_reference, "unresolved Python evidence")
    reject("unbounded corpus", remove_corpus, "not a finite corpus loop")
    reject("parser I/O", add_io, "imports outside the local-only allowlist")
    print("protocol classifier admission tests: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
