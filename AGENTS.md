# AGENTS.md

> This file is designed for AI agents. Human developers please go to [README.md](README.md) to find out more about the project.

MintMaker is a tool which automates dependency updates for repositories (hosted on **GitHub**, **GitLab**, **Forgejo**) onboarded to a CI pipeline called Konflux (components). It is a Kubernetes operator that runs Renovate via Tekton `PipelineRun` jobs when a `DependencyUpdateCheck` custom resource is created. The tool is ran via a Docker image.

## Read before making changes

1. [README.md](README.md) — details about what MintMaker does
2. [docs/architecture.md](docs/architecture.md) — project architecture
3. [docs/contributing.md](docs/contributing.md) — contribution guide

## Commands

Run from the repository root. See `make help` for every target.

| Command | What it does |
| ------- | ------------ |
| `make fmt` | Runs `go fmt ./...` on all packages. CI fails if code is not formatted. |
| `make vet` | Runs `go vet ./...` for static analysis. |
| `make generate` | Regenerates DeepCopy methods (`controller-gen object`); needed after API type changes. Output includes `zz_generated.deepcopy.go` files — do not edit. |
| `make manifests` | Regenerates CRD YAML under `config/crd/bases/` and RBAC under `config/rbac/` from kubebuilder markers and API types — do not edit generated YAML files. |
| `make test` | Runs `manifests`, `generate`, `fmt`, `vet`, then unit and integration tests with **envtest** (downloads kubebuilder assets for `ENVTEST_K8S_VERSION` in the Makefile). Produces `cover.out`. |
| `make build` | Compiles the manager binary to `bin/manager`. |
| `make test-e2e` | Runs Ginkgo e2e tests under `test/e2e/` against a real cluster (not part of default `make test` / default PR CI). |
| `make run` | Runs the controller locally (needs kubeconfig and CRDs installed). |
| `make install` / `make deploy` | Applies CRDs / deploys the operator (cluster access required). |
| `mockery` | Regenerates `internal/component/mocks/mock_GitComponent.go` when `GitComponent` in `internal/component/component.go` changes (config: `.mockery.yaml`). |
| `pre-commit run --all-files` | Runs repo hooks as configured in `.pre-commit-config.yaml`. |

## Conventions

- **Edits**: Match existing patterns in the package you edit (Apache 2.0 headers, logging, naming of variables); minimize diff scope.
- **Formatting**: Keep Go gofmt clean (`go fmt ./...`); for Markdown, respect `markdownlint` (see `.markdownlint.json`).
- **Tests**: Add or update `*_test.go` alongside any new behavior or behavior change. Run `make test` before committing.
- **Test style**: Follow the style used in the edited package. If the package has a `suite_test.go`, add Ginkgo/Gomega specs in separate `*_test.go` files (keep `suite_test.go` as runner/shared setup). Otherwise use standard Go table-driven tests.
- **API / CRD changes**: After edits under `api/v1alpha1/`, run `make generate manifests` and commit generated DeepCopy, CRD (`config/crd/bases/`), and RBAC YAML. Never edit those files.
- **GitComponent**: After changing `GitComponent` in `internal/component/component.go`, run `mockery` (`.mockery.yaml`) and commit `internal/component/mocks/mock_GitComponent.go`.
- **RBAC**: Do not add `*` wildcards to `config/rbac/*.yaml`.
- **Controller cache**: Watch `DependencyUpdateCheck`, `Event`, and `PipelineRun` only in the `mintmaker` namespace; do not widen that cache without an explicit design reason.
- **Uncached API reads**: Secrets, ServiceAccounts, ConfigMaps, Pods, and Konflux `Component`s are fetched without cache — avoid broadening list calls when touching the client.
- **Git platforms**: Add new platform in `internal/component/<platform>/` (detection: `internal/utils/utils.go`, `GetGitPlatform`). Implement all the functionality that the already supported platforms have (GitHub, GitLab, Forgejo).
- **Renovate config**: Global rules under `config/renovate/`. These can be changed. File `.github/renovate.json` is configuration for when Renovate runs on this repo. Avoid changing this file unless instructed otherwise.
- **Imports**: Order stdlib, external libraries, then `github.com/konflux-ci/mintmaker/...`.
- **External libraries**: Add new external libraries only when completely necessary. Run `go mod tidy` whenever dependencies are added, removed, or upgraded. Include resulting `go.mod` and `go.sum` in the commit.
- **Logging**: Use controller-runtime `log` from context with structured keys (`component`, `namespace`, `repository`).
- **Errors**: Wrap with `%w`; use `errors.IsNotFound` for missing API objects.
- **Reconcile invariants**: Respect `mintmaker.appstudio.redhat.com/processed` on DUCs, `disabled: "true"` on Components, PipelineRun deduplication by repo+branch hash, and registry pull secrets from `build-pipeline-<component>` ServiceAccounts.
- **OSV tools**: `tools/osv-generator/` and `tools/osv-downloader/` are standalone; not part of the operator reconcile loop unless the task targets them. Do not change this.
- **Secrets**: Never commit tokens, keys, or kubeconfig snippets.
- **Documentation**: Update the corresponding parts of documentation (root `README.md` and other documentation under `docs/`) to reflect changes made when applicable.
- **Commits**: Run `pre-commit install` before committing. Use conventional commits (e.g., `feat:`, `fix:`, `chore:`). Explain what and why was changed in the commit message.
