# ADR 0007: Exclude provider adapters from the MVP

- Status: Accepted
- Date: 2026-07-29
- Deciders: Tobari maintainers
- Scope: Product
- Supersedes: None
- Superseded by: None

## Context

GitHub and other providers expose large, evolving APIs. An adapter that maps
requests to service operations would expand the trusted interpretation surface
before the isolation and policy loop are validated.

## Decision drivers

- Deliver one end-to-end effect boundary first
- Avoid false semantic claims and SDK credentials in application code
- Keep policy useful for arbitrary tools and destinations

## Considered options

- No provider adapters; write HTTP rules directly
- A GitHub-only adapter
- A general plugin API for provider translators

## Decision

The MVP exposes no GitHub, AWS, or other service adapter. A GitHub policy is
ordinary Rego over `host == "api.github.com"`, method, and path. Provider
operation translation remains a documented non-goal.

## Consequences

- Users write HTTP-level policy.
- Tobari does not explain SaaS business semantics.
- The trusted code and dependency surface stays small.

## Mechanical enforcement

The public catalog contains only shared-cluster lifecycle, named-Tobari
lifecycle, execution, logs, diagnostics, help, and version capabilities.
Architecture lint keeps provider SDKs out of CLI/application packages.

## Compatibility, security, and validation

No external provider schema or account is required for tests. Reconsider after
the generic boundary has evidence that a repeated user outcome needs a typed
adapter and can meet the external API contracts.
