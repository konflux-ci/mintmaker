# Automated Release Workflow

This document describes our automated release workflow for MintMaker managed through the [infra-deployments] repository.

## Configuration Structure

In the [infra-deployments] repository, MintMaker's main configurations are located under `components/mintmaker`:

- **development**: Used for local deployments or development clusters
- **staging**: Used for deployment in staging clusters
- **production**: Used for deployment in production clusters
- **base**: Contains shared resources referenced by the different environments

## MintMaker Components

MintMaker consists of two main components:
1. MintMaker controller (in [mintmaker] repository)
2. Renovate image (from the [mintmaker-renovate-image] repository)

## Release workflow

### Controller Release

When a PR is merged into the [mintmaker] repository, it automatically triggers a build in Konflux. After successful build completion, an image is generated and, after passing integration tests and enterprise contract policy checks, is pushed to the appropriate Quay repository with tags including `latest` and the git commit hash.

[!NOTE]
For simplicity, this document uses the term "image(s)" throughout, although this terminology is not always technically precise. In some contexts, we are indeed talking about container images, while in others, we're actually referring to Konflux Snapshot CR(s). This simplified terminology is used to maintain readability.

The release pipeline includes an `update-infra-deployments` task that executes a custom script specified in `ReleasePlanAdmission`. For the MintMaker controller, this script automatically creates a PR in the [infra-deployments] repository:

```yaml
infra-deployment-update-script: |
  sed -i \
  -e 's|\(https://github.com/konflux-ci/mintmaker/config/.*?ref=\)\(.*\)|\1{{ revision }}|' \
  -e '/newName:\s*quay.io\/konflux-ci\/mintmaker\s*$/{n;s|\(newTag:\s*\).*|\1{{ revision }}|}' \
  components/mintmaker/development/kustomization.yaml

  sed -i \
  -e 's|\(https://github.com/konflux-ci/mintmaker/config/.*?ref=\)\(.*\)|\1{{ revision }}|' \
  -e '/newName:\s*quay.io\/konflux-ci\/mintmaker\s*$/{n;s|\(newTag:\s*\).*|\1{{ revision }}|}' \
  components/mintmaker/staging/base/kustomization.yaml
```

This script updates the following elements in both development and staging environments:

### 1. Configuration Manifests

```yaml
resources:
- https://github.com/konflux-ci/mintmaker/config/default?ref=ad50001d2aba01949fd8a1458a3a58a5c4d1391f
- https://github.com/konflux-ci/mintmaker/config/renovate?ref=ad50001d2aba01949fd8a1458a3a58a5c4d1391f
```

Deploying the MintMaker controller requires other resource manifests (such as Deployments), which are referenced directly from the repository.

The Renovate config file is worth particular attention. It provides global configuration for Renovate and is managed as a separate resource to allow for flexible, independent updates or rollbacks rather than being included in the default resource.

### 2. Controller Image

```yaml
images:
  - name: quay.io/konflux-ci/mintmaker
    newName: quay.io/konflux-ci/mintmaker
    newTag: 65df7db7c7b09386fb147e410819267fcf92ac32
```

This instructs Kustomize to replace any image named `quay.io/konflux-ci/mintmaker` in the manifests with `quay.io/konflux-ci/mintmaker:65df7db7c7b09386fb147e410819267fcf92ac32`, allowing us to control which specific version of the image is used in deployment.

## Approval Process

After a controller release, an automated PR is created. It can be approved by adding `/approve` and `/lgtm` comments, which add the required "approved" and "lgtm" labels. When the PR has both labels and passed all tests, it is automatically merged.

If needed, you can modify automated PRs (for example, if you don't want to update the Renovate config file to the latest version). You can fetch the PR locally, make changes, and push back to the source branch, you need to have push permissions to the [infra-deployments] repository first.

Here is an example of the automated controller update PR: https://github.com/redhat-appstudio/infra-deployments/pull/6111

## Renovate Image Updates

The [mintmaker-renovate-image] repository handles building the Renovate image used by MintMaker. When this image is released, it also triggers an automated PR through a similar script:

```yaml
infra-deployment-update-script: |
  sed -i \
  -e '/newName:\s*quay.io\/konflux-ci\/mintmaker-renovate-image\s*$/{n;s|\(newTag:\s*\).*|\1{{ revision }}|}' \
  components/mintmaker/staging/base/kustomization.yaml
```

This script only updates the Renovate image in staging environment's `kustomization.yaml` file.

Here is an example of the automated Renovate image update PR: https://github.com/redhat-appstudio/infra-deployments/pull/6227

The Renovate image is referenced in `config/manager/manager.yaml` from [mintmaker] repository:

```yaml
apiVersion: apps/v1
kind: Deployment
...
spec:
  ...
  template:
    metadata:
      annotations:
        mintmaker.appstudio.redhat.com/renovate-image: quay.io/konflux-ci/mintmaker-renovate-image:latest
    spec:
      containers:
      - command:
        - /manager
        ...
        - name: RENOVATE_IMAGE
          valueFrom:
            fieldRef:
              fieldPath: metadata.annotations['mintmaker.appstudio.redhat.com/renovate-image']
```

The image is not directly specified in the `RENOVATE_IMAGE` environment variable because Kustomize doesn't support identifying images in environment variables. Instead, we use an annotation as a workaround. Although this isn't natively supported by Kustomize, it's achieved through through this [kustomizeconfig.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/staging/base/kustomizeconfig.yaml):

```yaml
images:
- path: spec/template/metadata/annotations/mintmaker.appstudio.redhat.com\/renovate-image
  kind: Deployment
```

## Production Environment Updates

When changes to `components/mintmaker/staging/base/kustomization.yaml` are merged into the [infra-deployments] repository, a PipelineRun (defined in [mintmaker-prod-overlay-update.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/.tekton/mintmaker-prod-overlay-update.yaml)) is triggered. In this PipelineRun, the `promote-component` task checks if the manifests and image SHAs in the staging environment differ from those in production. If differences are found, it automatically opens a PR to update the production environment to match the staging environment.

Here is an example of the automated staging to production update PR: https://github.com/redhat-appstudio/infra-deployments/pull/5984

These production update PRs follow the same review, modification, and merge process described earlier.

[infra-deployments]: https://github.com/redhat-appstudio/infra-deployments
[mintmaker]: https://github.com/konflux-ci/mintmaker
[mintmaker-renovate-image]: https://github.com/konflux-ci/mintmaker-renovate-image
