# Optional local toolbox third-party notices

The Tobari toolbox is an optional image built locally by the user. It is not
part of the published Tobari runtime image. Its Docker build downloads the
following pinned upstream command-line tools for local use.

| Component | Version | License | Upstream source |
| --- | --- | --- | --- |
| GitHub CLI | 2.96.0 | MIT | https://github.com/cli/cli/releases |
| AWS CLI | 2.36.11 | Apache-2.0 | https://awscli.amazonaws.com/ |
| kubectl | 1.36.3 | Apache-2.0 | https://dl.k8s.io/release |
| TWG CLI | 1.1.1 | Upstream local-use terms | https://teamwork-graph.atlassian.com/cli/ |
| cwk | 0.2.4 | MIT | https://github.com/tasuku43/cwk/releases |
| Pup | 1.10.5 | Apache-2.0 | https://github.com/DataDog/pup/releases |

The cwk and Pup archives are verified against the architecture-specific
SHA-256 values in `versions.env` before extraction. Their upstream license and
third-party notice files are copied into `/usr/share/licenses/tobari-toolbox/`
inside the locally built image. The other tools retain their existing upstream
verification paths in the toolbox Dockerfile.
