# Tobari architecture presentation

This directory is the repository-owned static presentation for the current
main product line. It explains the four-layer dependency direction, host and
agent trust boundaries, the Gateway/OPA edge, the CWD-owned Workspace
lifecycle, denial-to-review-to-retry policy learning, and Context runtime
customization.

The presentation is intentionally plain HTML and CSS: no JavaScript, remote
fonts, CDN, image, or runtime dependency. Content is English, synthetic, and
safe to publish. The numbered documents remain the source of truth:

- [Project theses](../00_theses.md)
- [Product contract](../01_product_contract.md)
- [Architecture](../02_architecture.md)
- [Security model](../03_security_model.md)
- [Harness](../04_harness.md)
- [Public repository boundary](../05_public_repository.md)
- [Release model](../06_release.md)
- [Agent readiness validation](../09_agent_readiness_validation.md)

## Local preview

From the repository root:

```sh
python3 -m http.server 8000 --directory docs/architecture-site
```

Open `http://127.0.0.1:8000/` in a browser. The Pages workflow uploads only
this directory as its artifact.
