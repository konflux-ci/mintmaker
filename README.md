# MintMaker
MintMaker is designed to automate the process of checking and updating dependencies for components in Konflux. It utilizes [https://docs.renovatebot.com/](Renovate), a dependency update tool.

## Description

MintMaker introduces the DependencyUpdateCheck custom resource, which acts as a trigger for the dependency update process. When a DependencyUpdateCheck CR is created, MintMaker springs into action, examining all components within Konflux for dependency updates.

Konflux components originate from repositories on two types of platforms, GitHub and GitLab. MintMaker adapts its functionality based on the platform:

* GitHub: If the repository has Konflux's Pipeline as Code GitHub Application installed, MintMaker utilizes the token generated from the application to run Renovate.
* GitLab: MintMaker scans the component's namespace for a secret containing the Renovate token. Upon finding the token, MintMaker employs it to execute Renovate for components within the same namespace.

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/mintmaker:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/mintmaker:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Contributing
To contribute to MintMaker you need to be part of the [MintMaker Maintainers](https://github.com/orgs/konflux-ci/teams/mintmaker-maintainers) team.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Release process

MintMaker is released to Konflux through the [infra-deployments](https://github.com/redhat-appstudio/infra-deployments) repository.

**Staging** environment uses the `latest` tag of [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image/),
referenced in [manager_patches.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/staging/base/manager_patches.yaml). The PRs for this environment are
created automatically.

**Production** environment uses a fixed tag and commit for the configuration.

In order to release MintMaker to *production*, create a PR in [infra-deployments](https://github.com/redhat-appstudio/infra-deployments)
with the following changes:

In [kustomization.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/production/base/kustomization.yaml):

- If there was a change to the [default MintMaker config](https://github.com/konflux-ci/mintmaker/blob/main/config/renovate/renovate.json), modify the git commit
  hashes in `resources` to reflect that.
- If there was a change to the [MintMaker controller](https://github.com/konflux-ci/mintmaker) (this repository),
  change the container image tag in `images.newTag` property. This needs
  to be a valid and existing tag available in the quay.io repository.

In [manager_patches.yaml](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/mintmaker/production/base/manager_patches.yaml):

- If there was a change to [mintmaker-renovate-image](https://github.com/konflux-ci/mintmaker-renovate-image/), change the
  container image tag.

> [!WARNING]
> Always check the image tag(s) you propose in the PR actually exist
> in the Quay repositories. It is possible (however unlikely) that the build
> pipeline fails to build or push the image to the registry. This would
> result in the pods not being able to start up in Konflux.

After the PR is approved and merged, the deployment will start automatically.

> [!IMPORTANT]
> During the release process, please make sure to actively monitor at least
> one cluster's `mintmaker` namespace for any issues, most often it would
> be a `ImagePullBackOff` error for the controller pod, if anything went wrong.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

