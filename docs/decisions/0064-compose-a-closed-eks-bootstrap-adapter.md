# ADR 0064: Compose one closed EKS target with typed Workspace bootstrap

- Status: Accepted
- Date: 2026-08-18
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, runtime, CLI, and harness
- Related: ADR 0062 and ADR 0063
- Revised by: None
- Superseded by: None

## Context

ADR 0062 established that a Context may project a closed, secret-free AWS IAM
Identity Center snapshot once into a new Workspace. Users operating Amazon EKS
also need cluster target configuration. Kubernetes kubeconfig is not a safe
configuration bundle: it may contain bearer tokens, client keys, proxy
settings, file references, and arbitrary credential exec plugins. Copying or
mounting the host file would turn setup convenience into credential, filesystem,
and executable authority.

Amazon EKS has one narrower composition. AWS documents that its kubeconfig uses
`aws eks get-token`; that executable can consume the same named AWS profile
whose non-secret IAM Identity Center configuration is already projected while
the AWS CLI continues to own login and cached credentials inside the Workspace.

## Decision

Extend the typed Context bootstrap with one optional
`kubernetes_eks` service-target adapter. It reads only fixed host
`~/.kube/config`, selects one explicit context, and resolves exactly its cluster
and user. It normalizes only:

- the selected context name and optional namespace;
- one commercial-partition EKS HTTPS server and inline certificate-authority
  bundle; and
- one fixed AWS CLI exec contract for `eks get-token`, exact cluster name and
  region, JSON output, and an `AWS_PROFILE` value equal to the Context's
  `aws_iam_identity_center` profile.

The adapter rejects tokens, passwords, client certificates or keys, auth
providers, proxy or insecure TLS options, referenced files, arbitrary exec
commands, widened args or environment, role assumption, unknown fields,
duplicates, alternate kubeconfig paths, symlinks, and unsafe or oversized host
files. Tobari parses but never executes the host entry. Projection emits a
canonical private kubeconfig that selects the reviewed `aws` command and exact
normalized arguments; it never copies source bytes.

The EKS adapter depends on the AWS adapter. Removing EKS preserves AWS; removing
AWS or replacing its profile while EKS remains fails closed. Existing AWS-only schema-1 snapshots retain
their exact semantic revision and remain readable without rewrite. One aggregate
revision continues to describe the create-time recipe.

Projection still occurs only for a fresh Workspace. Context refresh does not
rewrite existing homes. Configuration performs no AWS or Kubernetes API call
and grants no destination, method, network, credential, or authorization
authority. Native `aws sso login --profile <name>` and credential persistence
remain Workspace-owned; subsequent `kubectl` effects pass through ordinary
Gateway policy.

## Consequences

- New Workspaces can reuse one reviewed EKS target without inheriting kubeconfig
  credentials or generic exec behavior.
- Context creation can offer None, AWS, or AWS + EKS as closed bootstrap choices.
- Non-EKS clusters, additional AWS partitions, role arguments, multiple targets,
  and other exec plugins require a later adapter decision.
- The YAML parser is an infrastructure-only, pinned, license-reviewed dependency;
  CLI and domain remain third-party-free.

## Mechanical enforcement

- Domain tests bind validation, AWS-only revision compatibility, composed
  revisions, deterministic diffs, dependency rules, and secret-free reports.
- Infrastructure tests bind fixed-path/mode/size checks, YAML duplicate and
  hostile-field rejection, exact AWS exec parsing, profile equality, canonical
  private output, and absence of credential/cache paths.
- Catalog and CLI tests bind exact action grammar, Context-create dependencies,
  task identity, mutation result correlation, and recovery.
- Security and public gates verify the dependency, documentation, fixtures, and
  public boundary.
