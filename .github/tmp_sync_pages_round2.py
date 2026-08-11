from pathlib import Path

ROOT = Path("docs/architecture-site")


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    file = ROOT / path
    text = file.read_text()
    actual = text.count(old)
    if actual < count:
        raise SystemExit(f"{path}: expected at least {count}, found {actual}: {old[:90]!r}")
    file.write_text(text.replace(old, new, count))


auth = "src/content/docs/guides/authentication.mdx"
replace(auth, "| Auth Broker              | Encrypted Context vault under an installation root key          | A random project-bound handle, not static or renewable credential state | Introspects without secret, asks OPA, then resolves, refreshes, or signs after allow | Static exact bindings plus reviewed GitHub, AWS, and Datadog plans only |", "| Auth Broker              | Encrypted Context vault under an installation root key          | A random project-bound handle, not static or renewable credential state | Introspects without secret, asks OPA, then resolves, refreshes, or signs after allow | Exact reviewed bindings, including GitHub, AWS, Datadog, OpenAI, and Anthropic |")
replace(auth, '''<aside class="warning">
  <strong>Artifact status:</strong> the source and tests implement the Datadog
  acquisition and post-policy refresh path, but the current immutable Gateway
  and Auth Broker pins predate AWS console login and that request path. The
  standard cluster selects the earlier AWS Identity Center artifacts today; use
  [Component versions](/reference/component-versions/) to verify the selected
  identities before following the Datadog steps.
</aside>''', '''<aside class="warning">
  <strong>Current source versus published digests:</strong> this source requires
  Gateway API 4 and Auth Broker API 3 and includes the current OpenAI/Codex and
  Anthropic/Claude Code plans. The reviewed published digests selected by the
  ordinary build are one generation older. Tobari rejects that mismatch before
  cluster mutation. To exercise this checkout, use the explicit development
  resolver documented in [Install](../../start/install/) rather than replacing
  a reviewed digest with a moving image tag.
</aside>''')
replace(auth, "The reviewed helpers are GitHub, two AWS login methods, and Datadog US1 pup\nOAuth. For GitHub, on the trusted host:", "The reviewed built-in login helpers are OpenAI/Codex, Anthropic/Claude Code, GitHub, two AWS login methods, and Datadog US1 pup OAuth. Start on the trusted host:")
replace(auth, "tobari auth login --provider github --context default\ntobari auth status --context default", "tobari auth login --provider openai --context default\ntobari auth login --provider anthropic --context default\n# or: github / aws / datadog\ntobari auth status --context default")
replace(auth, "Workspace client Tool. The AWS-only `--method` flag requires\n`--provider aws`. Tobari runs the trusted host's identity-checked GitHub CLI", "Workspace client Tool. The AWS-only `--method` flag requires\n`--provider aws`.\n\nFor OpenAI, Tobari runs exact trusted-host Codex 0.146.0 with fixed device-login arguments in an isolated temporary HOME/CODEX_HOME and commits only canonical ChatGPT OAuth state. The Workspace receives a project-bound handle inside the version-pinned `.codex/auth.json` compatibility shape; it never receives the real access or refresh token. After OPA allow, Auth Broker selects or refreshes the access token and Gateway adds the validated account routing header.\n\nFor Anthropic, Tobari runs exact trusted-host Claude Code 2.1.220 with fixed `claude setup-token` behavior in a bounded private PTY. The captured inference token stays outside the Workspace. `CLAUDE_CODE_OAUTH_TOKEN` receives only the project handle, which resolves after exact OPA allow at `api.anthropic.com`; this plan does not auto-refresh.\n\nFor GitHub, Tobari runs the trusted host's identity-checked GitHub CLI")
replace(auth, '''Leave and re-enter a Workspace permanently bound to that Context:

```sh
tobari --context default
```

The Workspace receives''', '''Authentication mutations cannot rewrite an already-running Workspace in place. Inspect `tobari auth status --context default`: its Workspace activation rows report whether each known projection is current, missing, stale, unavailable, or unresolved, and provide an exact re-entry action only when that is the supported recovery. Do not restart every Workspace unconditionally.

On the next required reconciliation, the Workspace receives''')
replace(auth, "8. On a Datadog allow, Broker selects a sufficiently valid token or performs\n   one same-record strict refresh at the exact proxy-free, no-redirect US1\n   token endpoint and commits state before returning a bearer value.\n9. Gateway creates one exact HTTPS upstream connection. It does not replay.", "8. On a Datadog allow, Broker selects a sufficiently valid token or performs\n   one same-record strict refresh at the exact proxy-free, no-redirect US1\n   token endpoint and commits state before returning a bearer value.\n9. On an OpenAI allow, Broker selects a sufficiently valid access token or performs one bounded exact refresh at `auth.openai.com`, persists the updated OAuth state, and returns the validated account identity for Gateway routing. Anthropic uses the static resolve path and has no Broker refresh.\n10. Gateway creates one exact HTTPS upstream connection. It does not replay.")
replace(auth, "Replacing a Context/provider credential revokes every previously issued handle. A running process still holds its old environment value, but that value is invalid. Leave and re-enter to receive a new project-specific projection.", "Replacing a Context/provider credential revokes every previously issued handle. A running process can retain an old environment or complete-file projection, but that handle is invalid. Inspect `auth status`; re-enter only the Workspaces reported as missing or stale to receive the new project-specific projection.")
replace(auth, "Logout removes the local provider record and revokes its handles.", "Logout removes and revokes the local provider record when present; an already-absent credential reports `change=no_change` rather than pretending that state changed.")
replace(auth, "  affected AWS/Datadog provider, then leave and re-enter before retrying.", "  affected AWS/Datadog/OpenAI provider, then follow the Workspace activation action reported by `auth status` before retrying.")
replace(auth, "refresh/signing/OAuth beyond the reviewed AWS and Datadog plans.", "refresh/signing/OAuth beyond the reviewed AWS, Datadog, OpenAI, and Anthropic plans.")
replace(auth, "or Datadog token action.", "Datadog token action, OpenAI token refresh, or Anthropic static credential resolution.")

# Keep the browser contract aligned with the current six built-in/provider examples.
test = "tests/site.spec.ts"
replace(test, 'await expect(map.locator(".pairing-row")).toHaveCount(4);', 'await expect(map.locator(".pairing-row")).toHaveCount(6);')
replace(test, '''    const expected = [
      ["github", "GitHub CLI", "gh"],''', '''    const expected = [
      ["openai", "Codex CLI", "codex"],
      ["anthropic", "Claude Code", "claude"],
      ["github", "GitHub CLI", "gh"],''')

print("Applied English authentication and browser-contract follow-up.")
