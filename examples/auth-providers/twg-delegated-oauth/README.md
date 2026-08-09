# TWG delegated-OAuth provider example

This owner-controlled schema-v1 example projects one brokered handle through
`TWG_OAUTH_ACCESS_TOKEN` and replaces it with a user-supplied OAuth access token
only on requests to exact authority `https://api.atlassian.com:443`. It is not
a built-in provider and does not acquire an Atlassian credential.

The bounded slice can cover TWG operations whose HTTP requests stay on that
authority, including Atlassian OAuth GraphQL at `/graphql` and OAuth cloud REST
routes below `/ex/jira/{cloudId}/...` or `/ex/confluence/{cloudId}/...`.
Ordinary Tobari policy still authorizes each exact method and path before the
Gateway resolves the handle.

A TWG command name is not an authority guarantee. TWG versions and individual
commands can use other Atlassian or connector endpoints, so verify that the
operation stays on `api.atlassian.com`. Do not widen the manifest to wildcard
Atlassian or connector domains.

## Install and import into the default Context

Copy the manifest to the owner-controlled provider directory and apply the
required owner-only permissions:

```sh
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers"
cp provider.json "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/twg-delegated-oauth.json"
chmod 0700 "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers"
chmod 0600 "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/twg-delegated-oauth.json"
tobari cluster up
```

Supply a current delegated OAuth access token from a trusted, non-interactive
secret source; never put it in argv or a shell environment variable:

```sh
trusted-token-source | tobari auth import twg-delegated-oauth --context default
```

Leave and re-enter the default Context's Workspace after a successful import.
The projected value is an opaque, project-bound Tobari handle rather than the
upstream access token.

## Deliberate limits

The imported access token is short-lived. Tobari cannot refresh it; import a
new current token when needed. This example does not support `twg login`, OAuth
refresh or logout, `twg rovo auth`, federated connector authorization or calls,
site-domain APIs, Bitbucket token flows, signed-media downloads, Admin APIs,
multiple accounts, updater traffic, telemetry, or feedback submission. It
makes no claim that all Rovo, Jira, or Confluence commands remain within the
bounded authority.

The TWG binary is not part of this example. Atlassian's public documentation
permits installing and using TWG, but this project has not identified an
affirmative public license granting redistribution of the binary in a public
runtime image.
