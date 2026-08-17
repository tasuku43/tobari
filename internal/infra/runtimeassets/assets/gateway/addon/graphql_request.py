"""Bounded extraction of GraphQL policy identity from one HTTP POST request.

This module is deliberately independent from mitmproxy and Tobari's Gateway
hooks.  It validates the transport envelope, parses one bounded GraphQL
document, and returns only the operation type, canonical root field names, and
the original bytes that a later integration must forward unchanged.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from typing import Iterable

from graphql import GraphQLSyntaxError, parse
from graphql.language import OperationType
from graphql.language.ast import (
    FieldNode,
    FragmentDefinitionNode,
    FragmentSpreadNode,
    InlineFragmentNode,
    OperationDefinitionNode,
    SelectionSetNode,
)

MAX_BODY_BYTES = 1024 * 1024
MAX_GRAPHQL_TOKENS = 10_000
MAX_ROOT_FIELD_OCCURRENCES = 256
MAX_FRAGMENTS = 256
MAX_SELECTION_DEPTH = 64
MAX_FRAGMENT_DEPTH = 64
MAX_TRAVERSAL_NODES = 20_000

_POSITIVE_DECIMAL = re.compile(r"^[1-9][0-9]*$")


class GraphQLRequestError(ValueError):
    """The request cannot safely produce a GraphQL policy identity."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


class _DuplicateJSONKey(ValueError):
    pass


@dataclass(frozen=True)
class GraphQLParseLimits:
    """Finite parser and traversal limits for one GraphQL request."""

    body_bytes: int = MAX_BODY_BYTES
    tokens: int = MAX_GRAPHQL_TOKENS
    root_field_occurrences: int = MAX_ROOT_FIELD_OCCURRENCES
    fragments: int = MAX_FRAGMENTS
    selection_depth: int = MAX_SELECTION_DEPTH
    fragment_depth: int = MAX_FRAGMENT_DEPTH
    traversal_nodes: int = MAX_TRAVERSAL_NODES

    def validate(self) -> None:
        for name, value in vars(self).items():
            if not isinstance(value, int) or isinstance(value, bool) or value < 1:
                raise ValueError(f"GraphQL parser limit {name} must be positive")


@dataclass(frozen=True)
class ParsedGraphQLRequest:
    """The only request facts intended for policy plus unchanged body bytes."""

    operation_type: str
    root_fields: tuple[str, ...]
    original_body: bytes = field(repr=False, compare=False)


def _reject(code: str, message: str) -> None:
    raise GraphQLRequestError(code, message)


def _header_values(
    headers: Iterable[tuple[str, str]], requested_name: str
) -> list[str]:
    values: list[str] = []
    try:
        pairs = list(headers)
    except (TypeError, ValueError):
        _reject("invalid_headers", "GraphQL request headers are invalid")
    for pair in pairs:
        if not isinstance(pair, tuple) or len(pair) != 2:
            _reject("invalid_headers", "GraphQL request headers are invalid")
        name, value = pair
        if not isinstance(name, str) or not isinstance(value, str):
            _reject("invalid_headers", "GraphQL request headers are invalid")
        if name.lower() == requested_name:
            values.append(value)
    return values


def validate_graphql_post_headers(
    method: str,
    headers: Iterable[tuple[str, str]],
    limits: GraphQLParseLimits,
) -> int | None:
    """Reject an unsafe GraphQL transport envelope before buffering its body."""

    limits.validate()
    if method != "POST":
        _reject("unsupported_method", "GraphQL policy supports POST only")

    try:
        pairs = list(headers)
    except (TypeError, ValueError):
        _reject("invalid_headers", "GraphQL request headers are invalid")
    content_types = _header_values(pairs, "content-type")
    if len(content_types) != 1:
        _reject("invalid_content_type", "GraphQL request requires one Content-Type")
    content_type_parts = [part.strip().lower() for part in content_types[0].split(";")]
    if not (
        content_type_parts == ["application/json"]
        or content_type_parts == ["application/json", "charset=utf-8"]
    ):
        _reject(
            "invalid_content_type",
            "GraphQL request Content-Type must be application/json UTF-8",
        )

    if _header_values(pairs, "transfer-encoding"):
        _reject(
            "unsupported_transfer_encoding",
            "GraphQL request transfer encoding is unsupported",
        )
    if _header_values(pairs, "content-encoding"):
        _reject(
            "unsupported_content_encoding",
            "GraphQL request content encoding is unsupported",
        )

    content_lengths = _header_values(pairs, "content-length")
    if not content_lengths:
        return None
    if len(content_lengths) != 1 or not _POSITIVE_DECIMAL.fullmatch(content_lengths[0].strip()):
        _reject(
            "invalid_content_length",
            "GraphQL request Content-Length must be one positive decimal value",
        )
    raw_length = content_lengths[0].strip()
    if len(raw_length) > 10:
        _reject(
            "invalid_content_length",
            "GraphQL request Content-Length is invalid",
        )
    declared_length = int(raw_length)
    if declared_length > limits.body_bytes:
        _reject("body_too_large", "GraphQL request body exceeds its size limit")
    return declared_length


def _validate_transport(
    method: str,
    headers: Iterable[tuple[str, str]],
    body: bytes,
    limits: GraphQLParseLimits,
) -> None:
    if not isinstance(body, bytes):
        _reject("invalid_body", "GraphQL request body must be bytes")
    declared_length = validate_graphql_post_headers(method, headers, limits)
    if len(body) > limits.body_bytes:
        _reject("body_too_large", "GraphQL request body exceeds its size limit")
    if declared_length is not None and declared_length != len(body):
        _reject(
            "content_length_mismatch",
            "GraphQL request Content-Length does not match its body",
        )


def _strict_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise _DuplicateJSONKey(key)
        result[key] = value
    return result


def _reject_json_constant(_value: str) -> None:
    raise ValueError("non-finite JSON number")


def _decode_envelope(body: bytes) -> tuple[str, str | None]:
    try:
        text = body.decode("utf-8", errors="strict")
        envelope = json.loads(
            text,
            object_pairs_hook=_strict_object,
            parse_constant=_reject_json_constant,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, _DuplicateJSONKey, ValueError, RecursionError):
        _reject("invalid_json", "GraphQL request body is not strict UTF-8 JSON")

    if not isinstance(envelope, dict):
        _reject("invalid_envelope", "GraphQL request body must be one JSON object")
    allowed_keys = {"query", "operationName", "variables", "extensions"}
    if set(envelope) - allowed_keys:
        _reject("invalid_envelope", "GraphQL request contains an unknown envelope field")

    query = envelope.get("query")
    if not isinstance(query, str):
        _reject("invalid_envelope", "GraphQL request requires a query string")
    operation_name = envelope.get("operationName")
    if operation_name is not None and not isinstance(operation_name, str):
        _reject("invalid_envelope", "GraphQL operationName must be a string or null")
    variables = envelope.get("variables")
    if variables is not None and not isinstance(variables, dict):
        _reject("invalid_envelope", "GraphQL variables must be an object or null")
    extensions = envelope.get("extensions")
    if extensions is not None and extensions != {}:
        _reject("invalid_envelope", "GraphQL extensions must be absent, null, or empty")
    return query, operation_name


def _selection_nodes(
    selection_set: SelectionSetNode, limits: GraphQLParseLimits
) -> Iterable[FieldNode | FragmentSpreadNode | InlineFragmentNode]:
    stack: list[tuple[SelectionSetNode, int]] = [(selection_set, 1)]
    visited = 0
    while stack:
        current, depth = stack.pop()
        if depth > limits.selection_depth:
            _reject("document_too_deep", "GraphQL selection depth exceeds its limit")
        for selection in current.selections:
            visited += 1
            if visited > limits.traversal_nodes:
                _reject("document_too_complex", "GraphQL document traversal exceeds its limit")
            yield selection
            nested = getattr(selection, "selection_set", None)
            if nested is not None:
                stack.append((nested, depth + 1))


def _document_parts(query: str, limits: GraphQLParseLimits):
    try:
        document = parse(query, no_location=True, max_tokens=limits.tokens)
    except (GraphQLSyntaxError, RecursionError, TypeError, ValueError):
        _reject("invalid_document", "GraphQL document is invalid or exceeds its token limit")

    operations: list[OperationDefinitionNode] = []
    fragments: dict[str, FragmentDefinitionNode] = {}
    for definition in document.definitions:
        if isinstance(definition, OperationDefinitionNode):
            operations.append(definition)
            continue
        if isinstance(definition, FragmentDefinitionNode):
            name = definition.name.value
            if name in fragments:
                _reject("invalid_document", "GraphQL fragment names must be unique")
            fragments[name] = definition
            if len(fragments) > limits.fragments:
                _reject("too_many_fragments", "GraphQL fragment count exceeds its limit")
            continue
        _reject("invalid_document", "GraphQL document must be executable only")
    if not operations:
        _reject("missing_operation", "GraphQL document has no operation")
    if any(operation.operation not in {OperationType.QUERY, OperationType.MUTATION} for operation in operations):
        _reject("unsupported_operation", "GraphQL subscriptions are unsupported")

    operation_names: set[str] = set()
    anonymous_count = 0
    for operation in operations:
        if operation.name is None:
            anonymous_count += 1
        elif operation.name.value in operation_names:
            _reject("invalid_document", "GraphQL operation names must be unique")
        else:
            operation_names.add(operation.name.value)
        list(_selection_nodes(operation.selection_set, limits))
    for fragment in fragments.values():
        list(_selection_nodes(fragment.selection_set, limits))
    if anonymous_count and len(operations) != 1:
        _reject("ambiguous_operation", "Anonymous GraphQL operation must be alone")
    return operations, fragments


def _select_operation(
    operations: list[OperationDefinitionNode], operation_name: str | None
) -> OperationDefinitionNode:
    if operation_name is None:
        if len(operations) != 1:
            _reject("ambiguous_operation", "GraphQL operationName is required")
        return operations[0]
    matches = [
        operation
        for operation in operations
        if operation.name is not None and operation.name.value == operation_name
    ]
    if len(matches) != 1:
        _reject("missing_operation", "GraphQL operationName did not select one operation")
    return matches[0]


def _fragment_edges(
    operations: list[OperationDefinitionNode],
    fragments: dict[str, FragmentDefinitionNode],
    limits: GraphQLParseLimits,
) -> dict[str, set[str]]:
    edges = {name: set() for name in fragments}
    all_sets = [operation.selection_set for operation in operations]
    all_sets.extend(fragment.selection_set for fragment in fragments.values())
    for selection_set in all_sets:
        for selection in _selection_nodes(selection_set, limits):
            if isinstance(selection, FragmentSpreadNode) and selection.name.value not in fragments:
                _reject("undefined_fragment", "GraphQL fragment spread is undefined")
    for name, fragment in fragments.items():
        for selection in _selection_nodes(fragment.selection_set, limits):
            if isinstance(selection, FragmentSpreadNode):
                edges[name].add(selection.name.value)
    return edges


def _validate_fragment_graph(
    edges: dict[str, set[str]], limits: GraphQLParseLimits
) -> None:
    state: dict[str, int] = {name: 0 for name in edges}
    for start in edges:
        if state[start] != 0:
            continue
        state[start] = 1
        stack: list[tuple[str, object, int]] = [
            (start, iter(sorted(edges[start])), 1)
        ]
        while stack:
            name, children, depth = stack[-1]
            try:
                child = next(children)  # type: ignore[arg-type]
            except StopIteration:
                state[name] = 2
                stack.pop()
                continue
            if state[child] == 1:
                _reject("cyclic_fragment", "GraphQL fragment spreads must not cycle")
            if state[child] == 2:
                continue
            if depth >= limits.fragment_depth:
                _reject("fragment_too_deep", "GraphQL fragment depth exceeds its limit")
            state[child] = 1
            stack.append((child, iter(sorted(edges[child])), depth + 1))


def _root_fields(
    operation: OperationDefinitionNode,
    fragments: dict[str, FragmentDefinitionNode],
    limits: GraphQLParseLimits,
) -> tuple[str, ...]:
    fields: set[str] = set()
    occurrences = 0
    visited = 0
    stack: list[tuple[SelectionSetNode, int]] = [(operation.selection_set, 1)]
    while stack:
        selection_set, fragment_depth = stack.pop()
        if fragment_depth > limits.fragment_depth:
            _reject("fragment_too_deep", "GraphQL root fragment depth exceeds its limit")
        for selection in selection_set.selections:
            visited += 1
            if visited > limits.traversal_nodes:
                _reject("document_too_complex", "GraphQL root traversal exceeds its limit")
            if isinstance(selection, FieldNode):
                occurrences += 1
                if occurrences > limits.root_field_occurrences:
                    _reject("too_many_root_fields", "GraphQL root field count exceeds its limit")
                fields.add(selection.name.value)
            elif isinstance(selection, FragmentSpreadNode):
                stack.append(
                    (fragments[selection.name.value].selection_set, fragment_depth + 1)
                )
            elif isinstance(selection, InlineFragmentNode):
                stack.append((selection.selection_set, fragment_depth + 1))
            else:  # pragma: no cover - graphql-core owns the closed selection union.
                _reject("invalid_document", "GraphQL root selection is invalid")
    if not fields:
        _reject("missing_root_field", "GraphQL operation has no root field")
    return tuple(sorted(fields))


def parse_graphql_post_request(
    *,
    method: str,
    headers: Iterable[tuple[str, str]],
    body: bytes,
    limits: GraphQLParseLimits = GraphQLParseLimits(),
) -> ParsedGraphQLRequest:
    """Validate and extract one POST GraphQL identity without rewriting bytes."""

    limits.validate()
    _validate_transport(method, headers, body, limits)
    query, operation_name = _decode_envelope(body)
    operations, fragments = _document_parts(query, limits)
    edges = _fragment_edges(operations, fragments, limits)
    _validate_fragment_graph(edges, limits)
    operation = _select_operation(operations, operation_name)
    operation_type = (
        "query" if operation.operation == OperationType.QUERY else "mutation"
    )
    return ParsedGraphQLRequest(
        operation_type=operation_type,
        root_fields=_root_fields(operation, fragments, limits),
        original_body=body,
    )
