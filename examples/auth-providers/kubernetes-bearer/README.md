# Kubernetes bearer-token provider example

This owner-controlled schema-v1 example projects one complete kubeconfig into a
Workspace and brokers one bearer token to one exact Kubernetes API authority.
It is not a built-in provider and is deliberately unusable until you replace
the example values.

## Customize the exact target

Before installing the manifest, edit `provider.json` and replace all three of
these values:

1. Replace both occurrences of `kubernetes-api.example.com` with the same
   lowercase, fully qualified API server DNS name.
2. Replace both occurrences of port `443` if the API server uses another port.
3. Replace `REPLACE_WITH_BASE64_PEM_CA_DATA` with the base64-encoded PEM CA
   bundle that authenticates that API server.

The `clusters[].cluster.server` authority and the header-binding target must
remain identical. Do not add `insecure-skip-tls-verify`, `proxy-url`, an `exec`
credential plugin, client keys, or additional cluster authorities to this
example. A complete-file projection replaces the Workspace's entire
`~/.kube/config`; preserve any existing configuration separately.

The current Gateway rejects dotted hostnames that resolve to non-global IP
addresses. Consequently, this example supports a public-DNS, publicly routed
API endpoint only; a private, loopback, link-local, or literal-IP cluster
endpoint is outside this slice.

## Install and import into the default Context

Copy the customized manifest to the owner-controlled provider directory and
apply the required owner-only permissions:

```sh
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers"
cp provider.json "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/kubernetes-api-token.json"
chmod 0700 "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers"
chmod 0600 "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/kubernetes-api-token.json"
```

Reconcile the shared cluster so it loads the owner manifest:

```sh
tobari cluster up
```

Then pipe the token from a trusted, non-interactive secret source; never put it
in argv or a shell environment variable:

```sh
trusted-token-source | tobari auth import kubernetes-api-token --context default
```

Leave and re-enter the default Context's Workspace after a successful import.
`kubectl` then reads `~/.kube/config`, sends the opaque Tobari handle as a
Bearer `Authorization` value, and the Gateway replaces it only after the exact
ordinary HTTP request is allowed by policy.

This slice does not acquire, refresh, or revoke the upstream token. It does not
support cloud-provider login, OIDC refresh, `credential-plugin`/`exec` auth,
client-certificate auth, multiple clusters, or kubeconfig merging.
