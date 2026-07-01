# Contributing to MintMaker

Guide for developing and submitting changes to this repository. For system design, see [architecture.md](architecture.md). For Konflux stage testing and releases, see [developer.md](developer.md).

## Prerequisites

Match CI and the Makefile:

| Tool               | Version / notes                                                                                     |
| ------------------ | --------------------------------------------------------------------------------------------------- |
| Go                 | See `go` directive in [go.mod](../go.mod) (CI uses `go-version-file: go.mod`)                       |
| Kubernetes cluster | For deploy/e2e; envtest uses **1.31.0** (`ENVTEST_K8S_VERSION` in Makefile)                         |
| kubectl            | Compatible with your cluster                                                                        |
| Container tool     | **Podman** by default (`CONTAINER_TOOL ?= podman`); Docker works if you set `CONTAINER_TOOL=docker` |
| make               | Standard developer workflow                                                                         |
| shellcheck         | Optional locally; required in CI (`make shellcheck`)                                                |
| actionlint         | Optional locally; required in CI (`make actionlint`)                                                |

For local deployment you need cluster-admin or sufficient RBAC to install CRDs and deploy the manager.

## Repository map

| Path                                            | Purpose                                                                                                |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `api/v1alpha1/`                                 | `DependencyUpdateCheck` CRD types; run `make generate` after edits                                     |
| `cmd/manager/main.go`                           | Operator entrypoint, manager/cache setup, controller registration                                      |
| `internal/controller/`                          | Reconcilers: `dependencyupdatecheck`, `pipelinerun`, `event`                                           |
| `internal/component/`                           | `GitComponent` interface; `github/`, `gitlab/`, `forgejo/` implementations                             |
| `internal/component/mocks/`                     | mockery-generated `GitComponent` mock — regenerate after interface changes                             |
| `internal/tekton/`                              | `PipelineRun` builder (Renovate job spec, mounts, env)                                                 |
| `internal/config/`                              | JSON config (`MINTMAKER_CONFIG_PATH`, default `/etc/mintmaker/config.json`)                            |
| `internal/constant/`                            | Namespace name, annotation/label names, default Renovate image                                         |
| `internal/metrics/`                             | Prometheus metrics registration                                                                        |
| `config/crd/bases/`                             | Generated CRD YAML                                                                                     |
| `config/renovate/`                              | Global Renovate config (`renovate.json`, `self-hosted.json`, presets) — deployed via infra-deployments |
| `config/default/`, `config/manager/`            | Kustomize deployment manifests                                                                         |
| `config/samples/`                               | Example `DependencyUpdateCheck`                                                                        |
| `test/e2e/`                                     | Ginkgo e2e tests (excluded from default `make test`)                                                   |
| `tools/osv-generator/`, `tools/osv-downloader/` | OSV/CVE tooling (separate from operator runtime)                                                       |
| `hack/shellcheck-embedded-tekton-scripts/`    | CI tool: shellcheck on Tekton `Script:` strings in `pipeline_run_builder.go`                           |
| `docs/`                                         | Documentation                                                                                          |

## Git platforms

Platform detection: `internal/utils/utils.go` (`GetGitPlatform`).

| Platform | Package                       | Authentication                                                             |
| -------- | ----------------------------- | -------------------------------------------------------------------------- |
| GitHub   | `internal/component/github/`  | GitHub App installation token (Pipeline-as-Code app); cached installations |
| GitLab   | `internal/component/gitlab/`  | `BasicAuth` secrets in component namespace with App Studio SCM labels      |
| Forgejo  | `internal/component/forgejo/` | Token from namespace secrets (similar pattern to GitLab)                   |

When adding or changing platform behavior, update the matching package and tests under `internal/component/<platform>/`.

## Key constants and annotations

Defined in `internal/constant/constants.go`:

- `MintMakerNamespaceName` — `mintmaker` (controller watches CRs/events/PipelineRuns only here)
- `MintMakerProcessedAnnotationName` — CR processed once per creation
- `MintMakerDisabledAnnotationName` — set on a `Component` to `true` to skip it
- `KiteTokenSecretLabel` — optional Kite integration token in `mintmaker` namespace
- `RenovateImageEnvName` / `DefaultRenovateImageURL` — Renovate image for PipelineRuns

## Commands

```sh
make help          # all targets
make build         # compile manager to bin/manager
make test          # unit/integration tests + cover.out (excludes ./test/e2e)
make fmt           # go fmt — CI fails if not run
make vet
make generate      # DeepCopy code
make manifests     # CRD + RBAC YAML under config/
make run           # run controller locally (needs kubeconfig + CRDs)

# After editing api/ or +kubebuilder markers:
make generate manifests

# Regenerate GitComponent mock after changing internal/component/component.go:
mockery   # config in .mockery.yaml

# Local cluster deploy (set IMG if not using default quay.io/konflux-ci/mintmaker:latest):
make install       # CRDs
make deploy        # controller
make undeploy
make uninstall

make docker-build docker-push IMG=<registry>/mintmaker:<tag>

# E2E (requires a Kubernetes cluster):
make test-e2e

# Shell linting (also run in CI; needs shellcheck and actionlint on PATH):
make shellcheck   # Tekton Script: strings in pipeline_run_builder.go
make actionlint   # .github/workflows/
```

CI (`[.github/workflows/pr.yml](../.github/workflows/pr.yml)`) runs `Lint Go and PLR shell scripts` (go mod tidy, `make fmt`, `make generate manifests`, RBAC wildcard check, `make lint`, `make shellcheck` on embedded Tekton `Script:` strings), then `Test Go` (`make test`, `make build`, Codecov) only after that job passes. In parallel: `Lint Markdown`, `Lint Kubernetes manifests`, `Lint workflows` (actionlint on `.github/workflows/`, including inline `run:` shell when shellcheck is installed), and `Agent` (AGENTS.md line count).

Embedded Tekton step scripts live in `internal/tekton/pipeline_run_builder.go`; `make shellcheck` extracts and lints them without changing the source. See [hack/shellcheck-embedded-tekton-scripts/README.md](../hack/shellcheck-embedded-tekton-scripts/README.md) for skipped steps and shellcheck exclusions.

## Before opening a pull request

```sh
make fmt generate manifests lint shellcheck actionlint test build
```

We welcome contributions via pull requests and issues. Maintainers: [.github/CODEOWNERS](../.github/CODEOWNERS) (`@konflux-ci/mintmaker-maintainers`).

## Code conventions

- **Copyright header**: Apache 2.0 block at top of Go files (`hack/boilerplate.go.txt` for generated code).
- **Imports**: stdlib, third-party, then `github.com/konflux-ci/mintmaker/...`.
- **Logging**: controller-runtime `log` from context; use structured keys (`component`, `namespace`).
- **Errors**: wrap with `%w`; use `errors.IsNotFound` where appropriate.
- **Tests**: Ginkgo/Gomega in `*_test.go` with `suite_test.go` per package; envtest for controllers.
- **RBAC**: kubebuilder markers on reconcilers; **no wildcards** in `config/rbac/*.yaml` (CI enforces).
- **Generated files**: never hand-edit `zz_generated.deepcopy.go`, `config/crd/bases/`*, or mocks — regenerate.

## Testing

- **Unit and integration**: `*_test.go` files alongside the code they test; run with `make test` (uses envtest for Kubernetes API simulation).
- **E2E**: `test/e2e/` — run with `make test-e2e` (requires a cluster; not run in default CI).
- After changing `GitComponent` in `internal/component/component.go`:

```sh
go install github.com/vektra/mockery/v2@latest
mockery   # uses .mockery.yaml
```

Commit `internal/component/mocks/mock_GitComponent.go`.

- Controller changes: extend tests in `internal/controller/*_test.go`.
- Platform changes: test in `internal/component/<platform>/` and `internal/component/component_test.go`.

## Common change scenarios

| Task                                              | Where to work                                                             |
| ------------------------------------------------- | ------------------------------------------------------------------------- |
| CRD spec / validation                             | `api/v1alpha1/dependencyupdatecheck_types.go` → `make generate manifests` |
| Filtering components, preparing and creating PLRs | `internal/controller/dependencyupdatecheck_controller.go`, `common.go`    |
| Renovate PipelineRun shape                        | `internal/tekton/pipeline_run_builder.go`                                 |
| Specific platform component shapes and funcitons  | `internal/component`                                                      |
| Global Renovate rules                             | `config/renovate/`                                                        |
| Operator config (GitHub TTL, Kite)                | `internal/config/config.go`, deployment ConfigMap in infra-deployments    |
| Metrics                                           | `internal/metrics/`                                                       |
| Failure handling after Renovate                   | `internal/controller/event_controller.go`                                 |

## Renovate configuration

- **In-cluster**: ConfigMap built from `config/renovate/` kustomization; mounted into Renovate PipelineRuns.
- **Per-component**: `GitComponent.GetRenovateConfig()` merges base config with component-specific settings (registry secrets, branches).
- **Validator**: `.github/workflows/renovate-config-validator.yml` validates `config/renovate/`.

Production/staging pin config commits in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments) — changing `renovate.json` here affects Konflux after release promotion.

There are two `renovate.json` files. One file under `config/renovate/`, which serves as a global configuration for all repositories where MintMaker runs. The other under `.github/`, which is a configuration file for when Renovate suggests dependency updates on this repository.

## Guidelines

- Do not add `*` wildcards to RBAC YAML.
- Do not skip `make generate manifests` after API/RBAC marker changes.
- Keep in mind that GitHub, GitLab and Forgejo are supported.
- Do not widen controller cache to all namespaces without an explicit design reason.

## Related repositories

| Repository                                                                                    | Role                                                  |
| --------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| [konflux-ci/mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image) | Renovate container image used in PipelineRuns         |
| [redhat-appstudio/infra-deployments](https://github.com/redhat-appstudio/infra-deployments)   | Konflux deployment overlays (`components/mintmaker/`) |

## Further reading

| Document                                                       | Topic                            |
| -------------------------------------------------------------- | -------------------------------- |
| [architecture.md](architecture.md)                             | Controllers, CRDs, data flow     |
| [developer.md](developer.md)                                   | Stage testing, manual release    |
| [automated-release-workflow.md](automated-release-workflow.md) | Automated release                |
| [README.md](../README.md)                                      | Project overview and quick start |
