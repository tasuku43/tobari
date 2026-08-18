# ADR 0063: Bridge native AWS SSO Workspace login

- Status: Accepted
- Date: 2026-08-18
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, runtime, and harness
- Revises: ADR 0046 and ADR 0055
- Related: ADR 0044, ADR 0048, and ADR 0062
- Revised by: None
- Superseded by: None

## Context

Pinned AWS CLI runs inside the standard Workspace and stores IAM Identity Center state in that Workspace home. Its default authorization-code flow binds a random loopback port in the Workspace, opens a regional AWS SSO OIDC authorization URL, and waits at `http://127.0.0.1:<port>/oauth/callback`. The host browser redirects to the host's loopback namespace, so the Workspace listener cannot receive the callback. A report against AWS CLI 2.34.49 and Tobari commit `b1ab49b` observed port 36901. Inspection of pinned AWS CLI 2.36.11 and local 2.36.19 source confirms the same dynamic flow.

AWS CLI documents `--use-device-code` as its cross-device alternative. That is a valid recovery, but forcing it would replace the client's default local experience. Generic publication or discovery of Workspace loopback listeners would create an undeclared host-ingress boundary.

## Decision

Extend ADR 0055's closed Workspace browser registry with one callback-bearing `aws-sso` driver for pinned AWS CLI 2.36.11. Accept only:

- canonical HTTPS `oidc.<commercial-region>.amazonaws.com/authorize` with no explicit port;
- exactly `response_type=code`, `code_challenge_method=S256`, and `scopes=sso:account:access`;
- one bounded unreserved dynamic DCR client ID;
- one lowercase UUID state and one 43-character base64url PKCE challenge; and
- exact `http://127.0.0.1:<dynamic-non-privileged-port>/oauth/callback` redirect.

Unknown, duplicate, widened, malformed, non-commercial, neighboring, or externally targeted values fail closed. After complete validation, the existing attachment bridge verifies the selected owned Workspace, binds the host's same `127.0.0.1` port before opening the URL, accepts one connection, and relays its opaque bytes through the fixed Docker exec proxy to AWS CLI's same-port Workspace listener. The listener closes after that connection, browser failure, or attachment exit.

AWS CLI continues to own DCR, state and PKCE validation, callback parsing, token exchange, cache persistence, result presentation, and `--use-device-code` recovery. Tobari does not retain or log the URL, client ID, state, code, callback bytes, token, Start URL, account, or user identity.

This transport grants no AWS network effect. SSO OIDC registration, token exchange, portal access, and later AWS API calls still follow the selected Context's ordinary exact policy. The base-runtime AWS CLI artifact lock remains separate from the native-readiness effect catalog.

## Consequences

- An attached Workspace can complete the pinned AWS CLI's default `aws sso login` callback without a manual URL, callback transfer, fixed port, or host credential inheritance.
- `aws sso login --use-device-code` remains the explicit recovery for unsupported AWS URL semantics, port collision, missing attachment support, or a user who prefers cross-device authorization.
- New AWS partitions, scopes, grant shapes, callback targets, or pinned-client drift require another compatibility and security review.
- This adds no generic host port forward, raw TCP ingress, listener discovery, or AWS policy grant.

## Mechanical enforcement

- URL tests fix the exact regional authority/path, seven query fields, default scope, bounded client/state/PKCE shapes, and dynamic non-privileged callback.
- Hostile canaries reject alternate partitions, case, explicit ports, neighboring paths, scope widening, callback host/path/privileged port changes, duplicate keys, and fragments.
- The compiled driver-registry contract fixes one `aws-sso` callback entry and rejects ambiguity.
- A provider-specific relay test proves bind-before-open behavior and one opaque callback to the label-verified selected Workspace's same dynamic port.
- Governing contracts and agent-readiness validation retain AWS CLI's own `--use-device-code` recovery and the absence of an AWS baseline policy grant.

## Compatibility and migration

No public command, flag, Context schema, policy schema, Gateway protocol, credential format, or persisted state changes. Existing Workspaces acquire the bridge on their next reconciled interactive entry; their home and AWS configuration remain unchanged.

## Security and public-boundary impact

The trusted host gains one strict AWS SSO authorization browser/callback shape. Repository fixtures use only synthetic client, state, challenge, and callback bytes. No live AWS account, Start URL, authorization URL, code, token, user identity, or authenticated transcript is retained.
