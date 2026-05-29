# MintMaker documentation

## Start here

| If you want to… | Read |
|-----------------|------|
| Understand the project quickly | [README.md](../README.md) |
| Develop or modify the operator | [contributing.md](contributing.md) |
| Learn how controllers and CRDs fit together | [architecture.md](architecture.md) |
| Test changes in Konflux stage | [developer.md](developer.md#on-demand-runs-in-the-stage-environment) |
| Understand how releases reach production | [automated-release-workflow.md](automated-release-workflow.md) |
| Run a manual release / promotion | [developer.md](developer.md#release-process) |
| Configure an AI coding agent | [AGENTS.md](../AGENTS.md) |

## Documents

### Development

- **[contributing.md](contributing.md)** — Prerequisites, `make` commands, conventions, testing, common edit locations, PR checklist.

### Architecture and design

- **[architecture.md](architecture.md)** — Controllers, custom resources, Git platforms, Renovate integration, configuration.
- **[adr/](adr/)** — Architecture Decision Records ([ADR 0001](adr/0001-record-architecture-decisions.md) introduces the process).

### Operations and release

- **[developer.md](developer.md)** — On-demand Renovate jobs in stage, manual release checklist, infra-deployments promotion.
- **[automated-release-workflow.md](automated-release-workflow.md)** — How Konflux builds update `infra-deployments` for dev/staging and promote to production.

### Repository root

- **[README.md](../README.md)** — Overview and quick start.
- **[AGENTS.md](../AGENTS.md)** — AI coding agents only (links to contributing and architecture).

## Related repositories

| Repository | Role |
|------------|------|
| [redhat-exd-rebuilds/renovate](https://github.com/redhat-exd-rebuilds/renovate) | Downstream Renovate fork. |
| [konflux-ci/mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image) | Renovate container image used in PipelineRuns. |
| [konflux-ci/renovate-log-analyzer](https://github.com/konflux-ci/renovate-log-analyzer) | A tool to analyze Renovate logs. Runs as a last step of the Tekton Pipeline. |
| [konflux-ci/mintmaker-osv-database](https://github.com/konflux-ci/mintmaker-osv-database) | The image containing the OSV database used by MintMaker. |
| [redhat-appstudio/infra-deployments](https://github.com/redhat-appstudio/infra-deployments) | Konflux deployment overlays (`components/mintmaker/`) |
| [konflux-ci/application-api](https://github.com/konflux-ci/application-api) | Konflux `Component` API (Go module dependency) |
| [konflux-ci/mintmaker-presets](https://github.com/konflux-ci/mintmaker-presets) | A repository hosting Renovate presets created by the MintMaker team. |

## Important configuration files

| Path | Purpose |
|------|---------|
| `config/renovate/renovate.json` | Global Renovate configuration (also deployed via infra-deployments) |
| `config/renovate/self_hosted.json` | Self-hosted preset fragment |
| `config/renovate/presets/` | Additional Renovate presets |
| `config/samples/` | Example `DependencyUpdateCheck` |
| `config/manager/manager.yaml` | Controller Deployment (Renovate image annotation) |
