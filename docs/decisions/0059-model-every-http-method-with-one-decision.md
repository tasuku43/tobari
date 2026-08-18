# ADR 0059: Model every HTTP method with one decision

- Status: Accepted
- Date: 2026-08-17

## Context

The original preset schema combined named guardrail variants with an `all` or
exact method ceiling. It could express offline and GET-only review, but not
“allow public HTTPS GET and require exact review for every other method”
without another evaluator special case.

## Decision

Schema-V1 policy presets use one complete `method_policy`:

- `default` is `allow`, `exact_review`, or `deny`;
- `overrides` contains sorted, unique exact uppercase HTTP methods whose
  decisions differ from the default;
- every method, including an extension method, resolves through an exact
  override or the default.

The destination ceiling remains independent. `allow` grants the eligible
effect immediately, `exact_review` leaves it to exact baseline, learned, or
Advanced policy and permits candidate creation, and `deny` is terminal. Exact
Deny remains terminal over method Allow. Native readiness is independent and
is filtered by destination and method Deny.

Built-ins are data in this model: offline defaults to Deny; reviewed-exact and
agent-ready default to Exact Review; get-only-reviewed defaults to Deny with a
GET Exact Review override; and public-get-reviewed defaults to Exact Review
with a GET Allow override. GET is not intrinsically safe or read-only.

Context and preset list/show outputs expose the method default and overrides.
ADR 0060 applies the same complete model to argument-free interactive Context
creation without changing this evaluator decision.

## Consequences

- New network presets compose without new evaluator branches.
- Operators inspect effective method behavior without inferring it from names.
- Broad method Allow is Context-wide for every eligible destination and
  process, so destination/method Deny and exact Deny must precede it.
- Pre-public custom schema-V1 sources using `method_ceiling` must be rewritten
  to `method_policy` before validation.

## Verification

Domain tests cover complete resolution, override uniqueness, redundant
override rejection, and canonical ordering. Aggregate tests require terminal
method and destination decisions, readiness filtering, and exact-Deny
precedence. CLI contract tests require typed method-policy facts in preset and
Context list/show output.
