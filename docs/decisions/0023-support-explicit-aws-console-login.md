# ADR 0023: Support explicit AWS console-based login

Status: Accepted

## Context

Tobari already supports one AWS IAM Identity Center role through a fixed host
AWS CLI device flow. AWS CLI 2.32 added `aws login`, which acquires a local-
development session from an AWS Management Console identity and refreshes its
temporary credentials from a login cache. Treating those flows as one implicit
"AWS login" would hide materially different prerequisites, prompts, state, and
recovery behavior.

Provider CLIs cannot move into the Auth Broker image: installed tools vary by
user, while the Broker must stay small and credential-bearing execution must
remain behind a reviewed host boundary. The existing companion already gives
AWS a post-policy host execution boundary without exposing a host listener or
provider home.

## Decision

`auth login aws` accepts one optional method selector:

- `identity-center` is the existing fixed IAM Identity Center device flow and
  remains the omission default.
- `console` is a fixed AWS CLI 2.32-or-newer `aws login --remote` flow.

Selection is explicit; Tobari never infers it from ambient AWS state. Console
mode runs against the same verified host `aws` executable in a private HOME,
sets a private `AWS_LOGIN_CACHE_DIRECTORY`, preconfigures one validated
commercial region, uses terminal stdin for the returned authorization code,
and starts no callback listener. After success Tobari accepts only one
validated profile `login_session` ARN, its 12-digit account identity, and a
bounded canonical JSON login cache.

The console plan has its own `aws_cli_console_login` driver ID and strict
schema-2 opaque state. Existing `aws_cli_sso` schema-1 states remain valid. The
Broker treats either state as opaque encrypted AWS CLI session material. The
companion requires the requested driver ID to equal the decoded state variant,
then runs the same fixed `aws configure export-credentials --format process`
operation only after OPA allow. Refreshed state remains protected by the
existing generation, revision, single-flight, and durable no-replay controls.

The existing public provider/credential schema identifiers remain compatibility
identifiers for the reviewed refreshable AWS CLI session plan; they do not
authorize a manifest to choose a driver, argv, refresh flow, or signing logic.

## Consequences

- The trusted host, not the base work image or Auth Broker image, must provide a
  compatible AWS CLI. Console mode reports a typed unsupported-version failure
  when the resolved CLI predates 2.32.
- A user can replace the Context's current AWS grant by logging in with either
  method. Replacement rotates every project handle as before.
- Automatic refresh lasts only while AWS's refresh token remains valid. On
  expiry or an unknown refresh outcome, the user explicitly repeats the same
  selected login method.
- Static access keys, ambient profiles, same-device callbacks, multiple AWS
  accounts per Context, custom endpoints, and arbitrary executable adapters
  remain unsupported.
