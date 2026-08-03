# Architecture publication E2E transcript

This is the bounded local transcript for the static architecture presentation
and its publication gates. Commands are shown from the repository root. The
transcript uses only repository-relative paths and synthetic assertions; it
does not record host paths, usernames, tokens, or browser/session data.

## Scope and method

- Source: `docs/architecture-site/index.html` and `styles.css`.
- Local server: Python's standard-library `http.server` bound to loopback.
- Checks: parse HTML, validate local links and required semantic sections,
  check CSS balance and external-asset absence, fetch the served page, then
  run `git diff --check` and the required repository gates.
- Publication boundary: `.github/workflows/architecture-pages.yml` uploads
  only `docs/architecture-site` through the official Pages artifact/deploy
  actions. The agent does not deploy or push.

## Local E2E command

The following command was run from the repository root after the site files
were created:

```sh
set -eu
site=docs/architecture-site
work=$(mktemp -d)
python3 - "$site" "$work" <<'PY'
from html.parser import HTMLParser
from pathlib import Path
import sys

site = Path(sys.argv[1])
work = Path(sys.argv[2])
html_path = site / "index.html"
css_path = site / "styles.css"
html = html_path.read_text(encoding="utf-8")
css = css_path.read_text(encoding="utf-8")

class Document(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.ids = set()
        self.links = []
        self.tags = []
        self.errors = []

    def handle_starttag(self, tag, attrs):
        self.tags.append(tag)
        values = dict(attrs)
        if "id" in values:
            if values["id"] in self.ids:
                self.errors.append(f"duplicate id: {values['id']}")
            self.ids.add(values["id"])
        if tag == "a" and "href" in values:
            self.links.append(values["href"])
        if tag == "link" and "stylesheet" in values.get("rel", "").split():
            self.links.append(values.get("href", ""))

document = Document()
document.feed(html)
document.close()
assert not document.errors, document.errors
assert document.tags.count("h1") == 1
assert document.tags.count("h2") >= 5
assert '<html lang="en">' in html
assert "<script" not in html.lower()
assert "url(" not in (html + css).lower()
assert "http://" not in (html + css).lower()
assert "https://" not in (html + css).lower()

for link in document.links:
    if link.startswith("#"):
        assert link[1:] in document.ids, f"missing anchor: {link}"
    else:
        target = (site / link).resolve()
        target.relative_to(site.resolve())
        assert target.is_file(), f"missing local target: {link}"

assert css.count("{") == css.count("}"), "unbalanced CSS braces"
assert "@media (prefers-reduced-motion: reduce)" in css
for section in ["layers", "boundaries", "lifecycle", "policy-loop", "context"]:
    assert f'id="{section}"' in html, section
for term in ["Gateway", "OPA", "Workspace", "default deny", "runtime build"]:
    assert term.lower() in html.lower(), term

(work / "source.html").write_text(html, encoding="utf-8")
print("static source validation: OK")
print(f"html ids: {len(document.ids)}")
print(f"validated links: {len(document.links)}")
print("required sections and terms: OK")
print("external asset/script dependency scan: OK")
PY
python3 -m http.server 8765 --directory "$site" >/dev/null 2>&1 &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true' EXIT
sleep 1
curl --fail --silent --show-error http://127.0.0.1:8765/ > "$work/fetched.html"
cmp -s "$site/index.html" "$work/fetched.html"
grep -Fq "Four layers. One inward pull." "$work/fetched.html"
grep -Fq "A denial is a handoff, not a hidden grant." "$work/fetched.html"
grep -Fq "Customize the runtime without moving the boundary." "$work/fetched.html"
echo "loopback fetch: HTTP 200 and byte match"
echo "served content assertions: OK"
```

Result:

```text
static source validation: OK
html ids: 17
validated links: 16
required sections and terms: OK
external asset/script dependency scan: OK
loopback fetch: HTTP 200 and byte match
served content assertions: OK
```

## Gate and scope commands

```sh
git diff --check
git status --short --untracked-files=all
task public:check
task check
task release:check
```

Result from a temporary clean checkout containing `HEAD` plus only the staged
allowed-path patch:

```text
git diff --cached --check: passed
task public:check: exit 0 (repoguard (public): OK; contractlint: OK)
task check: exit 0 (repoguard, archlint, contractlint, runtimecheck, Gateway snapshot, and full Go tests passed)
task release:check: exit 201 (the release script reports its ShellCheck failure as inner exit 1)
  ShellCheck scripts/test-integration.sh:238: SC2183 (warning), SC2016 (info)
```

The release failure is a pre-existing repository blocker in
`scripts/test-integration.sh`, outside the allowed path set. It was not
modified or staged. The exact source line in `HEAD` is:

```text
238    printf 'printf "tobari-shell:%s\\n" "$BASH"\\n'
```

The final shared worktree also retained unrelated changes that appeared after
the initial clean status and were never staged:

```text
 M README.md
 M docs/work/cli-catalog-audit/context.md
 M docs/work/cli-catalog-audit/goal.md
 M docs/work/cli-catalog-audit/plan.md
 M docs/work/cli-catalog-audit/tasks.md
 M docs/work/policy-review-tty/context.md
 M docs/work/policy-review-tty/goal.md
 M docs/work/policy-review-tty/tasks.md
 M docs/work/runtime-bash-shell/context.md
 M docs/work/runtime-bash-shell/goal.md
 M docs/work/runtime-bash-shell/plan.md
 M docs/work/runtime-bash-shell/tasks.md
 M docs/work/work-packet-retirement/context.md
 M docs/work/work-packet-retirement/goal.md
 M docs/work/work-packet-retirement/plan.md
 M docs/work/work-packet-retirement/tasks.md
?? docs/work/quickstart-runtime-docs/context.md
?? docs/work/quickstart-runtime-docs/e2e-transcript.md
?? docs/work/quickstart-runtime-docs/goal.md
?? docs/work/quickstart-runtime-docs/plan.md
?? docs/work/quickstart-runtime-docs/tasks.md
```

The allowed staged set remained exactly the ten paths listed below; no
unrelated change entered the scoped commit.

## Commit handoff

```sh
git diff --check
git add -- .github/workflows/architecture-pages.yml docs/README.md docs/architecture-site/README.md docs/architecture-site/index.html docs/architecture-site/styles.css docs/work/architecture-publication/context.md docs/work/architecture-publication/e2e-transcript.md docs/work/architecture-publication/goal.md docs/work/architecture-publication/plan.md docs/work/architecture-publication/tasks.md
git diff --cached --check
git diff --cached --name-only
git commit -m "docs: publish architecture presentation"
git status --short --branch
git log -1 --oneline --decorate
```

Result: the staged commit command will be run after this evidence update. Its
SHA will be reported in the final handoff; no push, deploy, reset, rebase,
force operation, or unrelated path change is authorized by this packet.
