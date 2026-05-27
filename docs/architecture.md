# MintMaker architecture

This document describes how MintMaker is structured and how data flows through the system. For development commands and file locations, see [contributing.md](contributing.md).

## Components in the Konflux ecosystem

MintMaker is deployed as a single controller Deployment in the `**mintmaker**` namespace on Konflux clusters. It does not run Renovate inside the controller process; instead it creates **Tekton PipelineRuns** that use the **[mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image)**.

Deployment manifests and environment-specific pins (image tags, Renovate config git refs) live in **[infra-deployments](https://github.com/redhat-appstudio/infra-deployments)** under `components/mintmaker/`.

## Custom resources

### DependencyUpdateCheck (`appstudio.redhat.com/v1alpha1`)

- **Scope**: Namespaced; in production, created in `mintmaker`.
- **Purpose**: Trigger one dependency-update pass.
- **Spec**: Optional `namespaces[]` tree to filter by Konflux namespace → application → component. Empty spec means all `Component` resources the controller can list.
- **Behavior**: Processed once per object (see `mintmaker.appstudio.redhat.com/processed` annotation).

Example: [config/samples/appstudio_v1alpha1_dependencyupdatecheck.yaml](../config/samples/appstudio_v1alpha1_dependencyupdatecheck.yaml).

### Konflux Component (external API)

- Type: `appstudio.redhat.com/v1alpha1` `Component` from [application-api](https://github.com/konflux-ci/application-api).
- MintMaker reads git URL, branches/revisions, and annotations from each Component.
- Skip when `mintmaker.appstudio.redhat.com/disabled: "true"` on the Component.

## Controllers

All controllers register in [cmd/manager/main.go](../cmd/manager/main.go). The manager caches **only the `mintmaker` namespace** for `DependencyUpdateCheck`, `Event`, and `PipelineRun`. Secrets, ServiceAccounts, ConfigMaps, Pods, and Components are read **without cache** to limit memory use.

### DependencyUpdateCheckReconciler

**File**: [internal/controller/dependencyupdatecheck_controller.go](../internal/controller/dependencyupdatecheck_controller.go)

1. Load `DependencyUpdateCheck`; exit if already processed.
2. Mark CR processed (annotation).
3. List/filter Konflux Components.
4. Optionally resolve Kite token secret if Kite is enabled in config.
5. For each component:
    - Build `GitComponent` via factory (`component.NewGitComponent`).
    - For each branch, skip if an active MintMaker PipelineRun exists for that repo+branch hash.
    - Build and create Tekton PipelineRun (Renovate job) via `internal/tekton`.

Also merges **registry pull secrets** from the component’s `build-pipeline-<component>` ServiceAccount for Renovate to access private images.

### PipelineRunReconciler

**File**: [internal/controller/pipelinerun_controller.go](../internal/controller/pipelinerun_controller.go)

Watches PipelineRun **updates** in `mintmaker`. When a run transitions to done, logs completion metadata (component, repository, success/failure). Used for observability rather than requeue logic.

### EventReconciler

**File**: [internal/controller/event_controller.go](../internal/controller/event_controller.go)

Reacts to Kubernetes **Events** related to failed Renovate pods. Can patch PipelineRuns and interact with components for remediation flows (including optional Kite log analysis when configured).

## Git platform abstraction

**Interface**: `GitComponent` in [internal/component/component.go](../internal/component/component.go).

| Method                | Role                                           |
| --------------------- | ---------------------------------------------- |
| `GetToken`            | Platform-specific credentials for git/API      |
| `GetBranches`         | Branches to run Renovate on                    |
| `GetRenovateConfig`   | Merged Renovate JSON for the run               |
| `GetRPMActivationKey` | RPM lockfile workflow support where applicable |

**Factory**: `NewGitComponent` selects implementation from git URL host:

| Platform  | Implementation                                               | Auth summary                                                     |
| --------- | ------------------------------------------------------------ | ---------------------------------------------------------------- |
| `github`  | [internal/component/github](../internal/component/github/)   | GitHub App installation token for the component repo             |
| `gitlab`  | [internal/component/gitlab](../internal/component/gitlab/)   | BasicAuth secrets in component namespace (App Studio SCM labels) |
| `forgejo` | [internal/component/forgejo](../internal/component/forgejo/) | Token from namespace secrets                                     |

Platform detection: [internal/utils/utils.go](../internal/utils/utils.go) (`GetGitPlatform`).

## Tekton integration

**Package**: [internal/tekton](../internal/tekton/)

`PipelineRunBuilder` constructs PipelineRuns with:

- Renovate image from `RENOVATE_IMAGE` env (from Deployment annotation `mintmaker.appstudio.redhat.com/renovate-image`, default `quay.io/konflux-ci/mintmaker-renovate-image:latest`).
- Mounted Renovate config (global + per-run).
- Git credentials and optional registry docker config.
- Labels for component, namespace, git host, repository hash (for deduplication).

## Configuration

### Operator JSON config

Loaded by [internal/config](../internal/config/) from `MINTMAKER_CONFIG_PATH` (default `/etc/mintmaker/config.json`):

- **GitHub**: installation token TTL and minimum validity before refresh.
- **Kite**: optional post-run log analysis (`enabled`, `api-url`).

### Renovate config

- **Global**: `config/renovate/` (Kustomize ConfigMap in deployment).
- **Per component**: merged in `GetRenovateConfig()` on each `GitComponent`.

Changes to global config require release through infra-deployments (see [automated-release-workflow.md](automated-release-workflow.md)).

## Metrics

Registered in [internal/metrics](../internal/metrics/) (e.g. DependencyUpdateCheck creation). Served on the controller metrics endpoint per `config/default` and RBAC in `config/rbac/`.

## OSV tooling (optional)

Under `tools/osv-generator/` and `tools/osv-downloader/` — standalone utilities for OSV/CVE data, not part of the runtime reconciliation loop. Entry: `cmd/osv-generator/main.go`.

## Security notes

- Controller RBAC is defined under `config/rbac/`; CI rejects wildcard `*` rules.
- GitHub App private key and tokens are cluster secrets (not in this repo).
- pprof is gated by `ENABLE_PROFILING=true` and binds to localhost when enabled.

## Diagram (deployment context)

```text
┌─────────────────────────────────────────────────────────────┐
│ Konflux cluster                                             │
│  namespace: mintmaker                                       │
│   ┌──────────────────────-┐    CronJob / manual             │
│   │ DependencyUpdateCheck │◄── creates CR                   │
│   └──────────┬───────────-┘                                 │
│              │ reconcile                                    │
│   ┌──────────▼───────────-┐                                 │
│   │ mintmaker controller  │                                 │
│   └──────────┬───────────-┘                                 │
│              │ creates                                      │
│   ┌──────────▼───────────-┐    ┌─────────────────────────┐  │
│   │ Tekton PipelineRun    │───►│ Renovate image pod      │  │
│   └──────────────────────-┘    │ (PRs on GitHub/GitLab/  │  │
│                                │  Forgejo)               │  │
│                                └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         ▲ lists/watches
         │ Konflux Component CRs (tenant namespaces)
```
