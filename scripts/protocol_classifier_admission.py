#!/usr/bin/env python3
"""Validate complete evidence for every Gateway protocol classifier."""

from __future__ import annotations

import ast
import json
import re
import sys
from pathlib import Path
from typing import Any


REQUIRED_DIMENSIONS = (
    "positive",
    "collision",
    "malformed_local_failure",
    "minimal_policy_projection",
    "privacy_exclusion",
    "no_ordinary_http_fallback",
    "zero_downstream",
    "bounded_adversarial_corpus",
)
ALLOWED_IMPORTS = {
    "__future__",
    "dataclasses",
    "graphql",
    "graphql.language",
    "graphql.language.ast",
    "json",
    "re",
    "typing",
    "urllib.parse",
}
FORBIDDEN_CALLS = {"__import__", "compile", "eval", "exec", "open"}
ASSERTIONS = {
    "positive": {"assertEqual", "assertIsNotNone"},
    "collision": {"assertFalse", "assertIsNone", "assertRaises"},
    "malformed_local_failure": {"assertRaises"},
    "minimal_policy_projection": {"assertEqual"},
    "privacy_exclusion": {"assertNotIn"},
    "zero_downstream": {"assert_not_called"},
}


class AdmissionError(ValueError):
    pass


def _load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise AdmissionError(f"admission manifest is unreadable: {error}") from error


def _python_tests(root: Path) -> dict[str, ast.FunctionDef]:
    tests: dict[str, ast.FunctionDef] = {}
    for path in sorted((root / "gateway").glob("test_*.py")):
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        for node in tree.body:
            if not isinstance(node, ast.ClassDef):
                continue
            for child in node.body:
                if isinstance(child, ast.FunctionDef) and child.name.startswith("test_"):
                    identity = f"gateway/{path.name}::{node.name}::{child.name}"
                    tests[identity] = child
    return tests


def _rego_tests(root: Path) -> set[str]:
    tests: set[str] = set()
    policy_root = root / "internal/infra/runtimeassets/assets/opa/policy"
    for path in sorted(policy_root.glob("*_test.rego")):
        relative = path.relative_to(root).as_posix()
        for name in re.findall(r"(?m)^(test_[A-Za-z0-9_]+)\s+if\s*\{", path.read_text(encoding="utf-8")):
            tests.add(f"{relative}::{name}")
    return tests


def _call_names(node: ast.AST) -> set[str]:
    result: set[str] = set()
    for child in ast.walk(node):
        if not isinstance(child, ast.Call):
            continue
        if isinstance(child.func, ast.Name):
            result.add(child.func.id)
        elif isinstance(child.func, ast.Attribute):
            result.add(child.func.attr)
    return result


def _has_bounded_corpus(node: ast.FunctionDef) -> bool:
    has_loop = any(isinstance(child, (ast.For, ast.AsyncFor)) for child in ast.walk(node))
    largest_literal = max(
        (
            len(child.elts)
            if isinstance(child, (ast.List, ast.Tuple, ast.Set))
            else len(child.keys)
            for child in ast.walk(node)
            if isinstance(child, (ast.List, ast.Tuple, ast.Set, ast.Dict))
        ),
        default=0,
    )
    return has_loop and largest_literal >= 3


def _validate_parser_source(
    root: Path,
    protocol: str,
    row: dict[str, Any],
    gateway_imports: set[str],
    gateway_calls: set[str],
) -> None:
    expected_source = f"gateway/addon/{protocol}_request.py"
    if row.get("source") != expected_source:
        raise AdmissionError(f"{protocol}: source must be {expected_source}")
    path = root / expected_source
    tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    imports: set[str] = set()
    definitions: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            imports.update(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            imports.add(node.module)
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            definitions.add(node.name)
        elif isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id in FORBIDDEN_CALLS:
            raise AdmissionError(f"{protocol}: parser performs forbidden local call {node.func.id}")
    unsupported = sorted(imports - ALLOWED_IMPORTS)
    if unsupported:
        raise AdmissionError(
            f"{protocol}: parser imports outside the local-only allowlist: {', '.join(unsupported)}"
        )
    parser_functions = row.get("parser_functions")
    if not isinstance(parser_functions, list) or not parser_functions or any(
        not isinstance(name, str) or not name for name in parser_functions
    ):
        raise AdmissionError(f"{protocol}: parser_functions must be a non-empty string list")
    for name in parser_functions:
        if name not in definitions:
            raise AdmissionError(f"{protocol}: parser function {name} is not defined")
        if protocol + "_request" not in gateway_imports or name not in gateway_calls:
            raise AdmissionError(f"{protocol}: parser function {name} is not imported and called by the Gateway")


def _validate_evidence(
    protocol: str,
    evidence: dict[str, Any],
    python_tests: dict[str, ast.FunctionDef],
    rego_tests: set[str],
) -> None:
    if set(evidence) != set(REQUIRED_DIMENSIONS):
        missing = sorted(set(REQUIRED_DIMENSIONS) - set(evidence))
        extra = sorted(set(evidence) - set(REQUIRED_DIMENSIONS))
        raise AdmissionError(f"{protocol}: evidence dimensions differ; missing={missing}, extra={extra}")
    for dimension in REQUIRED_DIMENSIONS:
        references = evidence[dimension]
        if not isinstance(references, list) or not references or any(
            not isinstance(reference, str) or not reference for reference in references
        ):
            raise AdmissionError(f"{protocol}/{dimension}: evidence must be a non-empty string list")
        for reference in references:
            if reference.startswith("python:"):
                identity = reference.removeprefix("python:")
                node = python_tests.get(identity)
                if node is None:
                    raise AdmissionError(f"{protocol}/{dimension}: unresolved Python evidence {identity}")
                if dimension == "no_ordinary_http_fallback":
                    raise AdmissionError(f"{protocol}/{dimension}: fallback evidence must be an OPA test")
                required = ASSERTIONS.get(dimension)
                if required and not (_call_names(node) & required):
                    raise AdmissionError(
                        f"{protocol}/{dimension}: {identity} lacks one of {sorted(required)}"
                    )
                if dimension == "bounded_adversarial_corpus" and not _has_bounded_corpus(node):
                    raise AdmissionError(f"{protocol}/{dimension}: {identity} is not a finite corpus loop of at least three cases")
            elif reference.startswith("rego:"):
                identity = reference.removeprefix("rego:")
                if identity not in rego_tests:
                    raise AdmissionError(f"{protocol}/{dimension}: unresolved Rego evidence {identity}")
                if dimension != "no_ordinary_http_fallback":
                    raise AdmissionError(f"{protocol}/{dimension}: only fallback evidence may use an OPA test")
            else:
                raise AdmissionError(f"{protocol}/{dimension}: unsupported evidence reference {reference}")


def validate(root: Path) -> None:
    manifest = _load_json(root / ".harness/protocol_classifier_admission.json")
    if not isinstance(manifest, dict) or set(manifest) != {
        "schema_version", "required_dimensions", "classifiers"
    }:
        raise AdmissionError("admission manifest shape is invalid")
    if manifest["schema_version"] != 1:
        raise AdmissionError("admission manifest version is invalid")
    if manifest["required_dimensions"] != list(REQUIRED_DIMENSIONS):
        raise AdmissionError("admission manifest dimensions are not the canonical ordered set")
    classifiers = manifest["classifiers"]
    if not isinstance(classifiers, dict):
        raise AdmissionError("admission classifiers must be an object")

    discovered = {
        path.name.removesuffix("_request.py")
        for path in (root / "gateway/addon").glob("*_request.py")
    }
    declared = set(classifiers)
    if discovered != declared:
        raise AdmissionError(
            f"classifier inventory differs; unregistered={sorted(discovered - declared)}, stale={sorted(declared - discovered)}"
        )

    gateway_path = root / "gateway/addon/tobari_gateway.py"
    gateway_tree = ast.parse(gateway_path.read_text(encoding="utf-8"), filename=str(gateway_path))
    gateway_imports = {
        node.module
        for node in ast.walk(gateway_tree)
        if isinstance(node, ast.ImportFrom) and node.module is not None
    }
    gateway_calls = _call_names(gateway_tree)
    python_tests = _python_tests(root)
    rego_tests = _rego_tests(root)

    for protocol in sorted(classifiers):
        row = classifiers[protocol]
        if not isinstance(row, dict) or set(row) != {"source", "parser_functions", "evidence"}:
            raise AdmissionError(f"{protocol}: classifier row shape is invalid")
        _validate_parser_source(root, protocol, row, gateway_imports, gateway_calls)
        evidence = row["evidence"]
        if not isinstance(evidence, dict):
            raise AdmissionError(f"{protocol}: evidence must be an object")
        _validate_evidence(protocol, evidence, python_tests, rego_tests)


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    try:
        validate(root)
    except AdmissionError as error:
        print(f"protocol classifier admission: {error}", file=sys.stderr)
        return 1
    print("protocol classifier admission: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
