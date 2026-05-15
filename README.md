# provider-harbor

## Overview

`provider-harbor` brings [Harbor](https://goharbor.io) configuration under
[Crossplane](https://crossplane.io) management. Install it into a control plane
and declare Harbor resources as Kubernetes manifests instead of managing them
through ad hoc scripts or manual clicks.

The provider currently manages:

| Resource | API Group | Description |
|---|---|---|
| `Project` | `project.harbor.crossplane.io` | Harbor project (registry namespace) |
| `RobotAccount` | `iam.harbor.crossplane.io` | System or project-scoped robot account |

A third API group, `instance.harbor.crossplane.io`, is reserved for future
instance-level resources (settings, labels, registries, etc.).

## Why This Provider

Use `provider-harbor` when you want Harbor configuration to behave like the rest
of your platform:

- **GitOps-friendly reconciliation** — desired state lives in Git; the provider
  continuously reconciles drift.
- **Cross-resource references** — `RobotAccount` resolves project names from
  `Project` CRs automatically via Crossplane reference resolution.
- **Compositions and abstractions** — build reusable platform building blocks on
  top of these primitives.

## Getting Started

### 1. Install the provider

```bash
kubectl apply -f https://raw.githubusercontent.com/EvannDev/provider-harbor/main/package/crds/harbor.crossplane.io_providerconfigs.yaml
kubectl apply -f https://raw.githubusercontent.com/EvannDev/provider-harbor/main/package/crds/harbor.crossplane.io_clusterproviderconfigs.yaml
kubectl apply -f https://raw.githubusercontent.com/EvannDev/provider-harbor/main/package/crds/project.harbor.crossplane.io_projects.yaml
kubectl apply -f https://raw.githubusercontent.com/EvannDev/provider-harbor/main/package/crds/iam.harbor.crossplane.io_robotaccounts.yaml
```

### 2. Create a Kubernetes secret with Harbor credentials

The secret must contain a JSON object with `username` and `password` keys:

```bash
kubectl create secret generic harbor-credentials \
  --namespace crossplane-system \
  --from-literal=credentials='{"username":"admin","password":"Harbor12345"}'
```

For robot accounts use the full name including the `robot$` prefix:

```bash
kubectl create secret generic harbor-robot-credentials \
  --namespace crossplane-system \
  --from-literal=credentials='{"username":"robot$my-robot","password":"<token>"}'
```

### 3. Configure a ProviderConfig

**Namespaced** (managed resources must be in the same namespace):

```yaml
apiVersion: harbor.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: crossplane-system
spec:
  url: https://harbor.example.com
  # insecureSkipTlsVerify: true  # uncomment for self-signed certificates
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: harbor-credentials
      key: credentials
```

**Cluster-scoped** (managed resources can be in any namespace):

```yaml
apiVersion: harbor.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  url: https://harbor.example.com
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: harbor-credentials
      key: credentials
```

Apply the bundled example to get up and running quickly:

```bash
kubectl apply -f examples/provider/config.yaml
```

## API Groups

### `project.harbor.crossplane.io`

Manages Harbor **projects** — the top-level namespace for repositories.

```yaml
apiVersion: project.harbor.crossplane.io/v1alpha1
kind: Project
metadata:
  name: my-project
  namespace: crossplane-system
spec:
  providerConfigRef:
    name: default
  forProvider:
    storageLimit: -1          # bytes; -1 = unlimited
    metadata:
      public: false
      autoScan: true
      preventVulnerable: true
      severity: high          # none | low | medium | high | critical
      reuseSysCveAllowlist: false
```

See [`examples/project/project.yaml`](examples/project/project.yaml) for a
complete example.

### `iam.harbor.crossplane.io`

Manages Harbor **robot accounts** — service identities for automated access.

```yaml
apiVersion: iam.harbor.crossplane.io/v1alpha1
kind: RobotAccount
metadata:
  name: my-ci-robot
  namespace: crossplane-system
spec:
  providerConfigRef:
    name: default
  writeConnectionSecretToRef:
    name: my-ci-robot-secret
    namespace: crossplane-system
  forProvider:
    level: system             # system | project
    description: "CI/CD robot"
    duration: 365             # days; -1 = never expires
    permissions:
      - kind: project
        namespaceRef:
          name: my-project    # resolved to the Project's external name
        access:
          - resource: repository
            action: push
          - resource: repository
            action: pull
```

The generated robot token is written as connection details to the secret named
in `writeConnectionSecretToRef`, with keys `name` (full robot name) and
`secret` (token).

Cross-resource reference resolution is supported: `namespaceRef` resolves the
Harbor project name from an existing `Project` CR automatically.

See [`examples/iam/robotaccount.yaml`](examples/iam/robotaccount.yaml) for a
complete example with both system-level and project-scoped robots.

### `harbor.crossplane.io`

Provider configuration only — `ProviderConfig` and `ClusterProviderConfig`.
No Harbor resources live in this group.

## Documentation

- [CRD documentation](https://marketplace.upbound.io/providers/EvannDev/provider-harbor/latest/crds)
- [Harbor documentation](https://goharbor.io/docs/)
- [Contributing guide](CONTRIBUTING.md)
- [Issue tracker](https://github.com/EvannDev/provider-harbor/issues)

## Support

If you run into a problem or want to request a feature, please open an issue in
the [provider repository](https://github.com/EvannDev/provider-harbor/issues).

## Licensing

`provider-harbor` is licensed under Apache 2.0. See [LICENSE](LICENSE) for the
full license text.

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2FEvannDev%2Fprovider-harbor.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2FEvannDev%2Fprovider-harbor?ref=badge_large)
