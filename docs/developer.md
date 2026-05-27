# Developer guide

Operations and release documentation for MintMaker maintainers. For development setup and commands, see [contributing.md](contributing.md) and [architecture.md](architecture.md). For automated release (and more information about the release process), see [automated-release-workflow.md](automated-release-workflow.md).

## On-demand runs in the stage environment

Use this when you want to validate changes **before** merging a PR. This step is optional.

### Steps

1. Log in to a Konflux **stage** cluster ([Konflux Clusters page](https://konflux.pages.redhat.com/konflux-portal/developer/konflux-clusters.html), RH SSO). For CLI access, use **Get Token** on that page or the OpenShift console after SSO login.
2. Build and push your controller image to Quay (or another registry you can pull from).
3. Find the YAML of the last **renovate-job** PipelineRun (or Job) in the `mintmaker` namespace and copy it.
4. Set the `image` field to your test image.
5. (Recommended) Enable Renovate dry-run so stage repos do not get real PRs:

   ```yaml
   env:
     - name: LOG_LEVEL
       value: debug
     - name: RENOVATE_DRY_RUN
       value: "full"
   ```

6. Create the job:
   - **UI**: **+** (top right corner) → Import YAML → paste YAML content into the window → **Create**
   - **CLI**: `oc create -f <path-to-yaml>`

The job runs immediately instead of waiting for CronJob schedule. To run again, **delete** the job and recreate it (re-running the same job from the UI may not always work).

### Delete a test job

- **UI**: Job page → **Actions** → **Delete Job**
  (If there is no Jobs tab: **Search** → resource type **Jobs** → filter by name → sort by creation time → open latest)
- **CLI**: `oc delete job <job-name>`

## Release process

> [!NOTE]
> Releases are **automated**. See [automated-release-workflow.md](automated-release-workflow.md) for more details.
>
> The sections below describe the **manual** process, which is still useful for troubleshooting or exceptional releases.

### Types of changes

| Change type | Repository | After merge |
|-------------|------------|-------------|
| Controller code | [mintmaker](https://github.com/konflux-ci/mintmaker) (this repo) | Konflux builds controller image → auto PR to infra-deployments (dev/staging) |
| Renovate image | [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image) | Konflux builds image → auto PR updates staging Renovate image tag |
| Renovate config only | `config/renovate/` in this repo | No image build; update config ref in infra-deployments (can skip to [stage run](#stage-run-to-check-the-new-build)) |

### Propose and merge changes

Open PRs as usual, get review and wait for the automatic CI pipelines to pass.

Konflux build status:

- GitHub **Actions** tab or PR checks after merge
- Pipeline name is often referenced as **`mintmaker-on-pull-request`**.

Images are pushed to Quay with tags including the git commit SHA.

To promote to **staging**, merge the PR that Konflux opens in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments) after the build completes. Look for a PR named `mitnmaker update`. If no PR is opened automatically, open one. Example PR updating the controller image in stage: [infra-deployments#11886](https://github.com/redhat-appstudio/infra-deployments/pull/11886).

To promote mintmaker from stage to production, look for a PR named `Promoting component mintmaker from stage to prod` (always check the **Files changed** tab, if the PR contains the correct SHA tags). Never update production with tags which have not been tested in staging. If no PR is opened, open one. Example PR updating the production image SHAs: [infra-deployments#11921](https://github.com/redhat-appstudio/infra-deployments/pull/11921).

To merge the PRs in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments), a `lgtm` and `approve` labels need to be added to the PR. This is done by adding a comment to the PR (author of the PR can not add the labels):

```txt
/lgtm
/approve
```

> [!WARNING]
> Confirm every image tag in the PR exists in Quay. A failed build can leave tags missing and cause `ImagePullBackOff`.

### Stage run to check the new build

1. Wait for the next scheduled MintMaker run **or** use [on-demand runs](#on-demand-runs-in-the-stage-environment) with the image Konflux built.
2. Check PipelineRun / pod logs for errors.
3. Confirm pod `image` (or image ID) matches your expected commit SHA.

### Check production deployment

1. [Konflux Clusters](https://konflux.pages.redhat.com/konflux-portal/developer/konflux-clusters.html) → production cluster → OpenShift (SSO) → namespace **`mintmaker`**.
2. Confirm `mintmaker-controller-manager-*` pod is **Running**.
3. After the next production Renovate run, check job logs and that Renovate pods use the expected image digest.

### Checklist (manual release)

### Deploy to production

- [ ] Open and merge PR(s) with changes in MintMaker repositories
- [ ] Wait for Konflux build → image on [Quay](https://quay.io/konflux-ci/mintmaker).
- [ ] Release to stage: Comment on the automatically opened infra-deployments staging PR, or open a PR updating SHA tags (example: [infra-deployments#11886](https://github.com/redhat-appstudio/infra-deployments/pull/11886)).
- [ ] Verify in stage: Wait for scheduled run or [on-demand run](#on-demand-runs-in-the-stage-environment) with Konflux-built image. Check pod images and logs in stage.
- [ ] Optional: check [mintmaker-test](https://github.com/staticf0x/mintmaker-test/) for RPM prototype regressions
- [ ] Deploy to production: Comment on the automatically opened infra-deployments production PR, or open a PR updating SHA tags (example: [infra-deployments#11921](https://github.com/redhat-appstudio/infra-deployments/pull/11921))

### After production deploy

- [ ] Controller pod healthy in `mintmaker`
- [ ] Next prod Renovate run logs clean
- [ ] Renovate job pods use expected image
