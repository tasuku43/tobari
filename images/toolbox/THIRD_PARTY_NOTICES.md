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

Each tool retains its upstream integrity verification path in the toolbox
Dockerfile. The image carries no provider-specific helper owned by a retired
brokered-authentication flow.
