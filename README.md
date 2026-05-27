# MintMaker

MintMaker automates dependency updates for [Konflux](https://github.com/konflux-ci) components. It is a Kubernetes operator that runs [Renovate](https://docs.renovatebot.com) via Tekton `PipelineRun` jobs when a `DependencyUpdateCheck` custom resource is created. Images are published on [quay.io/konflux-ci/mintmaker](https://quay.io/konflux-ci/mintmaker).

## How it works

1. Konflux creates a `DependencyUpdateCheck` (DUC) CR in the `mintmaker` namespace.
2. MintMaker finds Konflux `Component` resources (all clusters, or filtered by namespace/application/component in the DUC CR spec).
3. For each enabled component and branch, it creates a Tekton `PipelineRun` that runs the Renovate image.
4. Renovate opens or updates pull requests on the component’s Git repository.

Supported Git hosts: **GitHub**, **GitLab**, and **Forgejo**. See [docs/architecture.md](docs/architecture.md) for credentials and flow.

## Documentation

| Document | Description |
|----------|-------------|
| [docs/contributing.md](docs/contributing.md) | **Development** — prerequisites, commands, conventions, PR checks |
| [docs/README.md](docs/README.md) | Index of all documentation |
| [docs/architecture.md](docs/architecture.md) | Controllers, CRDs, and data flow |
| [docs/developer.md](docs/developer.md) | Stage testing and manual release |
| [docs/automated-release-workflow.md](docs/automated-release-workflow.md) | Automated promotion via infra-deployments |
| [AGENTS.md](AGENTS.md) | AI coding agents only |

## Quick start (local cluster)

Default image: `quay.io/konflux-ci/mintmaker:latest` (override with `IMG=...`).

```sh
make docker-build docker-push IMG=<registry>/mintmaker:tag
make install
make deploy IMG=<registry>/mintmaker:tag
kubectl apply -k config/samples/
```

Uninstall:

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

See [docs/contributing.md](docs/contributing.md) for prerequisites, all `make` targets, and local development details.

## Contributing

See **[docs/contributing.md](docs/contributing.md)** for how to build, test, and open a pull request.

Maintainers: [.github/CODEOWNERS](.github/CODEOWNERS).

## Release process

- [Automated release workflow](docs/automated-release-workflow.md)
- [Manual / stage release steps](docs/developer.md)

## License

Copyright contributors to the MintMaker project.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
