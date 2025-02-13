# Developer documentation

## On-demand runs in the stage environment
This trick might be useful during the development phase. In case you want to
check if your changes are working, before submitting a PR and having it merged
then automatically built by Konflux, it will be useful to have these on-demand
runs. This is not a mandatory step, so feel free to skip it.

Follow these steps to make an on-demand run in the stage environment:
- login to one of the stage clusters listed in the Konflux Link page (use RH SSO)
    - if you prefer to use the cli, you can use the 'Get Token' buttons in that
    page, or go through the familiar route in the UI after logging in
- manually build and push your new image to Quay (or your preferred image repository)
- get the yaml of the renovate-job that was last run in in Konflux's
- change its 'image' field and point to your repo/image
- optional (but encouraged during development): set renovate's dry run parameter
  to `full`, in case you don't want to create PRs in the stage repositories:
```yaml
    env:
    - name: LOG_LEVEL
        value: debug
    - name: RENOVATE_DRY_RUN
        value: 'full'
```
- manually create the job, let it run and check the logs for any error
    - if you are using the UI, you can click on the "+" button on the top right,
    paste your yaml in the window that will appear, then clicking on 'Create'
    - if you prefer using the cli, you can use `oc create -f <path to your yaml file>` 

Creating a job in this manner will make it run immediatelly, so you don't
need to wait until the next cronjob-invoked run. If you need to make another
run, simply delete the job and then recreate it (demanding another run to the
same job might not be possible from the UI):
- to delete your job via the UI: navigate to your job's page, then under the
  'Actions' dropdown, select 'Delete Job'
    - if you don't have a 'Jobs' tab in your interface, you can go to Search > 
    check 'Jobs' in the 'Resources' dropdown > select 'Name' in the filter
    dropdown, then enter the beginning of your job name > sort results by
    creation date > click on the latest job
- to delete it via the cli, use `oc delete job <your job name>`

## Release process

> [!NOTE]
> The release process is now automated. After merging PRs into either
> konflux-ci/mintmaker or konflux-ci/mintmaker-renovate-image, a PR
> for updating stage will open in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments) repository
> with the title `mintmaker update` or `mintmaker-renovate-image update`.
> 
> After merging the stage PRs, a PR titled
>  `Promoting component mintmaker from stage to prod` will open automatically
> to update the digests in production.
> 
> All you have to do now is to check that the digests are the ones
> you intend to release and that the images exist in their respective Quay
> repositories.
> 
> The following text describes the manual release process, which can
> still occasionally be helpful.

In this section we will describe the steps to make a release. We will see each
step separately in detail, and also provide a checklist by the end.

### Propose and merge your changes

Changes to the MintMaker project fall into one of these three categories:
- changes to the controller code (this repo)
- changes to the custom image used by MintMaker
- changes to the configuration file

Propose your changes as usual, get your reviews and finally merge them. In the
first two cases, the automatic build in Konflux will be triggered when your
changes are merged into the main branch, and the next step will be to wait for
them to finish.

If you are making routine changes for the weekly release and believe no reviews
are necessary, you can skip the reviews (e.g., the changes are simply konflux
reference udpates).

In case you are making changes only to the configuration file, you can skip to
the [stage run step](./developer.md#stage-run-to-check-the-new-build).

### Wait until Konflux automatic build finishes

The status of the Konflux build job can be checked in github via the 'Actions'
tab, or in the PR checks after the merge event. The build job is called
`mintmaker-on-pull-request`.

When it is finished, Konflux will push the new image to Quay, so you can watch
it to check when the job is done as well. Images are labelled with the git commit
sha, so you can use that to identify which image corresponds to which commit.

To promote an image to stage, you need to merge the MR that is automatically
created in infra-deployments after the Konflux build finishes.


### Stage run to check the new build

After the new image is built and pushed, wait until the next run and allow the
staging environment to pick up the new image. The **Staging** environment uses
the `latest` tag of [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image/),
referenced in [manager_patches.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/staging/base/manager_patches.yaml).
The PRs for this environment are created automatically.

Since the `latest` tag is dynamic, be mindful to check the image that is being
used in the run, to avoid headaches.

After the next MintMaker run starts, allow it to finish and check the logs. You
can also watch them during the run.

Check also that the pods used the correct image: look up the `image` field in
the pod yamls, and check that it has the digest sha corresponding to your commit.

In case you are making an update to the rpm-lockfile prototype, you can check
the PRs in [this test repository](https://github.com/staticf0x/mintmaker-test/).
This repository has only `.tekton` files and a RPM file, and you can check if
there aren't any regressions with the prototype.

If you do not wish to wait until the next automatic run, you can use the trick
explained before to make an on-demand run. Be sure to use the image built by Konflux.


### Make a change request to infra-deployments to push the new build to production

MintMaker is released to Konflux through the [infra-deployments](https://github.com/redhat-appstudio/infra-deployments)
repository. The **Production** environment uses a fixed tag and commit for the
configuration. This is safer, and we can be sure of which image is being used at
any given time.

When proceeding to create the change requests described next, change the values
to the same ones as in the stage environment. Failing to do thiis might void
the validation in the stage environment.

In order to release MintMaker to *production*, create a PR in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments)
with the following changes:

In [kustomization.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/production/base/kustomization.yaml):

- If there was a change to the [default MintMaker config](https://github.com/konflux-ci/mintmaker/blob/main/config/renovate/renovate.json),
  modify the git commit hashes in `resources` to reflect that.
- If there was a change to the [MintMaker controller](https://github.com/konflux-ci/mintmaker)
  (this repository), change the container image tag in `images.newTag` property.
  This needs to be a valid and existing tag available in the Quay repository.

In [manager_patches.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/production/base/manager_patches.yaml):

- If there was a change to [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image/),
  change the container image tag.

> [!WARNING]
> Always check the image tag(s) you propose in the PR actually exist
> in the Quay repositories. It is possible (however unlikely) that the build
> pipeline fails to build or push the image to the registry. This would
> result in the pods not being able to start up in Konflux.

Don't forget to ask review from the code owners to get it merged. After the PR 
is approved and merged, the deployment will start automatically.

> [!IMPORTANT]
> During the release process, please make sure to actively monitor at least
> one cluster's `mintmaker` namespace for any issues, most often it would
> be a `ImagePullBackOff` error for the controller pod, if anything went wrong.


### Check the deployment to production

The first thing to check is if the controller is running without any problems.
You can go to the Konflux Link page choose any production cluster (don't forget 
to switch to the mintmaker namespace),
and then:
- click on Openshift
- login via SSO
- make sure you are in the `mintmaker` namespace (use the 'Project' dropdown to
  select it)
- navigate to the 'Pods' view
- check that the pod `mintmaker-controller-manager-<hash id>` is running
    - if you don't have a 'Pods' tab on the left-most menu, go to Search > check
    'Pods' in the 'Resources' dropdown > select 'Name' in the filter dropdown,
    then start typing the controller pod's name, and sort the results by
    creation date; finally click on the latest job

After checking the controller, wait until the next prod run and monitor the
logs, or wait until the jobs are done and check the full logs. Also check the
image used in the renovate jobs: go to a pod yaml, and check if the `image`
field points to the correct image in Quay.


### Checklist

Deploying to production:
- open PR(s) with your changes
- ask for reviews, address suggestions, then get the changes merged
- wait until Konflux builds the new image and pushes it to [Quay](https://quay.io/repository/konflux-ci/mintmaker-renovate-image)
- stage run: wait until the next cronjob-invoked run and allow the stage
environment to pick it up; alternatively, use the trick explained before to make
an on-demand run
    - if making an on-demand run, be sure to use the image built by Konflux
    - check that the pods used the correct image (see `image` field in the pod
    yamls)
    - allow for the stage runs to finish and check the logs; you can also watch
    them during the run
    - in case you are making an update to the rpm-lockfile prototype, you can
    check the PRs in [the test repo](https://github.com/staticf0x/mintmaker-test/)
- update the image hash used in prod in the [infra-deployments repo](https://github.com/redhat-appstudio/infra-deployments)
    - example PR: https://github.com/redhat-appstudio/infra-deployments/pull/4432

Checking the deployment:
- check if the controller is running without issues
- wait until the next prod run and monitor the logs
- check the image used in the renovate jobs
