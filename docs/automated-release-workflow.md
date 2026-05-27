# Automated release workflow

MintMaker releases are driven by Konflux builds and pull requests in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments). For manual steps and stage debugging, see [developer.md](developer.md).

## Configuration structure in infra-deployments

Under `components/mintmaker/`:

| Overlay | Use |
|---------|-----|
| `development/` | Local or development clusters |
| `staging/` | Staging clusters |
| `production/` | Production clusters |
| `base/` | Shared resources referenced by environments |

## Two deployable artifacts

1. **MintMaker controller** — this repository ([konflux-ci/mintmaker](https://github.com/konflux-ci/mintmaker))
2. **Renovate image** — [konflux-ci/mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image)

> [!NOTE]
> For simplicity, this document uses the term "image(s)" throughout, although this terminology is not always technically precise. In some contexts, we are indeed talking about container images, while in others, we're actually referring to Konflux Snapshot CR(s). This simplified terminology is used to maintain readability.

## Controller release

When a PR merges to `main` in mintmaker:

1. Konflux builds the controller image.
2. Integration tests and Enterprise Contract policies run.
3. On success, the image is pushed to Quay (`latest` and commit SHA tags).
4. The release pipeline runs `update-infra-deployments` (script from `ReleasePlanAdmission`), opening a PR that updates **staging** clusters.
5. After sucessful update on **staging**, another PR is opened to update **production** clusters.

Example script (abbreviated):

```yaml
infra-deployment-update-script: |
  sed -i \
  -e 's|\(https://github.com/konflux-ci/mintmaker/config/.*?ref=\)\(.*\)|\1{{ revision }}|' \
  -e '/newName:\s*quay.io\/konflux-ci\/mintmaker\s*$/{n;s|\(newTag:\s*\).*|\1{{ revision }}|}' \
  components/mintmaker/staging/base/kustomization.yaml
```

### What the PR updates

**1. Configuration manifests**:

```yaml
resources:
- https://github.com/konflux-ci/mintmaker/config/default?ref=<sha>
- https://github.com/konflux-ci/mintmaker/config/renovate?ref=<sha>
```

The Renovate config file is worth particular attention. It provides global configuration for Renovate and is managed as a separate resource to allow for flexible, independent updates or rollbacks rather than being included in the default resource.

**2. Controller image tag**:

```yaml
images:
  - name: quay.io/konflux-ci/mintmaker
    newName: quay.io/konflux-ci/mintmaker
    newTag: <image-sha-tag>
```

### Approval

Automated PRs merge when they have `/approve` and `/lgtm` labels and pass checks. Maintainers with push access can amend the PR branch before merge (for example, to pin an older Renovate config ref).

Example controller update PR: [infra-deployments#11886](https://github.com/redhat-appstudio/infra-deployments/pull/11886)

## Renovate image updates

When [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image) releases, a similar script updates **staging** only:

```yaml
infra-deployment-update-script: |
  sed -i \
  -e '/newName:\s*quay.io\/konflux-ci\/mintmaker-renovate-image\s*$/{n;s|\(newTag:\s*\).*|\1{{ revision }}|}' \
  components/mintmaker/staging/base/kustomization.yaml
```

Example PR: [infra-deployments#11913](https://github.com/redhat-appstudio/infra-deployments/pull/11913)

The update is similiar to the controller image update:

```yaml
images:
  - name: quay.io/konflux-ci/mintmaker-renovate-image
    newName: quay.io/konflux-ci/mintmaker-renovate-image
    newTag: <image-sha-tag>
```

infra-deployments staging/production use [kustomizeconfig.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/staging/base/kustomizeconfig.yaml) to map `images:` rewrites onto that annotation path.

## Production promotion (staging → production)

When `components/mintmaker/staging/base/kustomization.yaml` changes on `main` in infra-deployments, [.tekton/mintmaker-prod-overlay-update.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/.tekton/mintmaker-prod-overlay-update.yaml) runs. The `promote-component` task compares staging and production manifests; if they differ, it opens a PR to align production with staging.

Example: [infra-deployments#11921](https://github.com/redhat-appstudio/infra-deployments/pull/11921)

Production PRs follow the same review and merge process as staging PRs with one additional approval. A `prod/needs-approval` label is automatically added to the PRs updating production. A short description (or comment) needs to be included in the PR containing:

- **What is changed**
- **Risk Assessment** (with _Risk Level_ and _Rollback_)
- **Validation** (how was this tested - e.g. in staging)

When an authorized person reviews the PR a `prod/approved` label is added. Once all the necessary approvals are given, the PR will be automatically merged.

## Links

- [infra-deployments](https://github.com/redhat-appstudio/infra-deployments)
- [mintmaker](https://github.com/konflux-ci/mintmaker)
- [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image)
- [Architecture](architecture.md) · [Contributing](contributing.md) · [Manual release guide](developer.md)
