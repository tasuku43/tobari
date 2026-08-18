# ADR 0062: Project typed Context bootstrap snapshots once at Workspace creation

- Status: Accepted
- Date: 2026-08-18

## Context

An isolated Workspace home prevents ambient host credential access, but also
makes users repeat non-secret tool configuration. Copying arbitrary dotfiles,
mounting the host home, or synchronizing live files would transfer credentials,
token caches, executable helpers, unknown directives, and mutable host authority.

AWS IAM Identity Center exposes a narrower first slice: its shared configuration
separates a named profile and `sso-session` metadata from the credentials file
and SSO token cache.

## Decision

A Context may own one schema-versioned `aws_iam_identity_center` bootstrap
snapshot. Tobari reads only fixed host `~/.aws/config`, selects one explicit
profile and its referenced `sso-session`, rejects duplicate or unknown
directives in those sections, and normalizes only reviewed profile, account,
role, region, output, start-URL, SSO-region, and registration-scope fields. It
never reads `~/.aws/credentials`, `~/.aws/sso/cache`, a helper, executable,
include, or alternate path.

The snapshot has a semantic SHA-256 revision and a generation that increments
only when normalized meaning changes. `config bootstrap aws` can configure,
refresh, or remove the recipe. Its terminal flow shows current and candidate
revisions plus changed field names, then re-reads the source at Apply and
rejects a changed candidate without mutation.

Argument-free `context create` adds an optional bootstrap step. Direct creation
uses `--bootstrap-aws-profile`. Only creation of a new logical Workspace may
project the snapshot. Before the instance and root index become authoritative,
infrastructure creates owner-only `.aws/config` in the fresh private home and
records the applied revision. Runtime reconciliation and Context refresh never
write an existing Workspace home. Status reports `not_configured`,
`not_applied`, `current`, or `older` without treating any state as a credential.

## Consequences

- New Workspaces can reuse reviewed non-secret AWS configuration without
  inheriting host authentication.
- Existing Workspaces remain stable across Context refresh and recipe removal.
- AWS native login remains owned by each Workspace home.
- This is a closed AWS adapter, not a generic dotfile or provider framework.
- Unsupported keys in selected sections fail closed rather than being copied.

## Verification

Domain tests bind revisions, generations, reports, and diffs. Infrastructure
tests use synthetic homes to prove strict parsing, helper rejection, path and
mode bounds, exact private projection, credentials/cache absence, create-only
application, stale-preview rejection, and old/current status after refresh.
Catalog and CLI tests fix direct/staged contracts and reporting. Repository
security and public gates remain completion authority.
