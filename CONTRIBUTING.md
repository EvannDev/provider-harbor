# Contributing

This repository follows the standard Crossplane contribution model, with a small amount of project-specific guidance below.

## Code Of Conduct And DCO

Please follow the Crossplane Code of Conduct and sign your commits with the Developer Certificate of Origin.

```text
Signed-off-by: John Doe <john.doe@example.org>
```

Use `git commit -s` to add the sign-off automatically.

## Getting Started

1. Fork the repository and create a branch for your work.
2. Clone the repository locally.
3. Run `make submodules` once so the shared build submodule is initialized.

## Working In This Repository

Keep changes focused and prefer small, reviewable commits. When adding or changing provider resources, make sure the corresponding controller, API registration, and tests are updated together.

## Makefile Overview

The most commonly used commands are:

* `make submodules`: initialize or refresh the shared build submodule.
* `make generate`: run code generation.
* `make lint`: run linters.
* `make test`: run unit tests.
* `make reviewable`: run generation, linting, and tests.
* `make build`: build the provider artifacts.
* `make dev`: start a local kind-based development loop.
* `make dev-clean`: delete the local development kind cluster.

## Adding A New Type

Use the helper target to scaffold a new resource type:

```shell
export provider_name=Harbor
export group=project
export type=Project
make provider.addtype provider=${provider_name} group=${group} kind=${type}
```

After scaffolding, register the new type in `internal/controller/register.go`, then run
`make reviewable`.

## Local Validation

Before opening a pull request, run `make reviewable`. If your change affects behavior, also run the most relevant focused tests for the area you changed.

## CI

The CI workflow runs these high-level checks on every pull request:

* linting
* a diff check to keep generated artifacts in sync
* unit tests with coverage publication
* artifact publishing for tagged and branch builds
