---
name: local-controller-testing-workflow-github
description: Use when you need to run MintMaker controller code locally against a real Kubernetes cluster (minikube or kind) with repo hosted on GitHub, create test Component/DependencyUpdateCheck resources, and iterate quickly across macOS and Linux.
---

# Local controller testing workflow

Self-contained — follow this skill only; ask the user for GitHub App credentials and test repo before bootstrap.

## Overview

Run the **MintMaker controller as a local process** (`make run` on your machine) while using a real Kubernetes cluster for CRDs, Secrets/ConfigMaps, and Tekton workloads.

Important split:

- **On your machine**: controller (`make run`), `kubectl`, and your editor/debugger.
- **In the cluster**: Tekton `PipelineRun`s, Renovate/OSV pipeline pods, and other in-cluster resources the controller creates.

The controller is **not** deployed into the cluster for this workflow.

Test YAML templates live in `skills/local-controller-testing-workflow-github/assets/` — use those paths; do not rely on personal Desktop folders or other out-of-repo paths, unless instructed otherwise.

## For AI agents — required before starting

**Do not guess credentials or repository URLs.** Ask the user for:

1. **GitHub App ID** — numeric ID from GitHub App settings (not the client ID).
2. **GitHub App private key path** — absolute path to the `.pem` file on the developer's machine.
   - The private-key path is only for substituting into the documented `kubectl create secret ... --from-file=...` command.
   - Never Read, @-mention, or shell-cat that file.
   - Never dump the resulting Secret with `-o yaml` / `-o json`.
   - If something goes wrong with the secrets, stop and ask the user to run the `kubectl` line themselves.
3. **Test repository URL** — `https://github.com/<owner>/<repo>.git` where **that GitHub App is installed**.
4. **Git revision** — branch or tag that exists in that repo (e.g. `main`).

Confirm the user has created and installed the GitHub App (see [GitHub App setup](#github-app-setup-one-time)) if they have not done so already.

Optional follow-ups if the environment is unclear:

- Whether they are on **Apple Silicon macOS** (may need an amd64 VM — see below).
- Whether they already have a local cluster running (`minikube status` / `kind get clusters`).

## Prerequisites

- _MacOS only_: VM (Lima VM is used in this example)
- Container runtime: Docker or Podman
- `kubectl`
- Go (repo currently expects Go 1.25+; see root `go.mod`)
- A local cluster: **minikube** (recommended) or **kind**

Run all commands from the **repository root** unless told otherwise.

## GitHub App setup (one-time)

Each developer needs their own GitHub App for local testing. Show these instructions to the user:

1. Create an app: [GitHub → Settings → Developer settings → GitHub Apps → New GitHub App](https://github.com/settings/apps/new)
2. Set a name and any homepage/webhook URL (webhook can be a placeholder for local testing).
3. Grant permissions:
   - Commit statuses: Read and write
   - Contents: Read and write
   - Pull requests: Read and write
   - Metadata: Read-only
   - Secrets: Read-only
4. Generate a **private key** and save the downloaded `.pem` file.
5. Note the numeric **App ID** on the app settings page.
6. **Install the app** on the account/org that owns the test repo. Choose **Only select repositories** and pick the repo you will use in the test Component.

If the app is not installed on the test repo, controller logs will show:

```text
failed to get installation ID: repository <owner>/<repo> not found in any GitHub App installation
```

## Pre-flight checks

### 1) Confirm a local cluster exists

```bash
minikube status
kind get clusters
```

Only one of these needs to succeed.

### 2) Point `kubectl` at a local cluster

If `kubectl get nodes` fails with authentication errors, you are likely on a remote/OpenShift context. Switch to minikube/kind or start a local cluster.

```bash
kubectl config current-context
kubectl config get-contexts -o name
kubectl get nodes -o wide
```

### 3) Check for stale test resources

`DependencyUpdateCheck.spec.namespaces` **filters** which Konflux Components are scanned:

- **Namespace only** — all Components in that namespace.
- **Namespace + `applications`** — only Components belonging to those applications.
- **Namespace + `applications` + `components`** — only the named Components (see `assets/test-duc.yaml`).
- **Omitted / empty `namespaces`** — all Components cluster-wide (avoid for local testing).

Leftover Components are only a problem when they still match your DUC filter. Prefer tightening the DUC spec (application/component names) over deleting unrelated Components.

```bash
kubectl get components -n test-app
```

Delete in-scope Components you no longer want processed, or narrow the DUC:

```bash
kubectl delete component <name> -n <namespace-name>
```

## Choose your cluster layout

Pick **one** branch by machine type. **Do not** run host minikube on Apple Silicon — use the Lima section below.

### Linux — minikube/kind on the host

Use minikube directly on the host (not applicable on Apple Silicon):

```bash
minikube start --memory 8192 --cpus 4
kubectl get nodes
```

Or kind:

```bash
kind create cluster
kubectl get nodes
```

Check node architecture (especially on Apple Silicon without a Lima VM):

```bash
kubectl get node minikube -o jsonpath='{.status.nodeInfo.architecture}{"\n"}'
```

If the node is **arm64**, pipeline images may fail with `exec format error` — use the Lima amd64 workflow below.

### Apple Silicon Mac — Lima VM with amd64 minikube (recommended)

MintMaker pipeline images (`quay.io/konflux-ci/*`) are **amd64-only**. On Apple Silicon, run minikube inside a **QEMU-based Lima VM** (x86_64) and connect from the Mac via a tunneled kubeconfig.

```text
┌───────────────────────────── Mac (host) ─────────────────────────────┐
│  make run  (MintMaker controller process)                             │
│  kubectl   (KUBECONFIG → tunneled API)                                │
└───────────────────────────────┬──────────────────────────────────────┘
                                │ SSH -L 127.0.0.1:<port>:127.0.0.1:<port>
                                │ (or Lima auto port-forward — see below)
┌───────────────────────────────▼──────────────────────────────────────┐
│  Lima VM (QEMU, x86_64)                                               │
│    minikube cluster (amd64 node)                                      │
│      ├─ Tekton controller/webhook                                     │
│      ├─ PipelineRuns / TaskRuns / pods (Renovate, OSV DB, …)        │
│      └─ CRs, Secrets, ConfigMaps                                      │
└──────────────────────────────────────────────────────────────────────┘
```

**One-time:** Install minikube and kubectl **inside** the Lima VM (not on the Mac). Use the amd64 minikube binary for Linux.

**Each session:**

1. Start the VM: `limactl start <lima-vm>`
2. Start minikube in the VM:

   ```bash
   limactl shell <lima-vm> -- minikube start --driver=docker --memory=8192 --cpus=4
   ```

   Use at least **8192 MB** if you want Renovate PipelineRuns to complete; lower values (e.g. 5120) may work for controller-only smoke tests but struggle with parallel pods.

3. Export kubeconfig on the Mac:

   ```bash
   limactl shell <lima-vm> -- minikube kubectl -- config view --minify --flatten > /tmp/vm-kubeconfig.yaml
   ```

4. Edit `/tmp/vm-kubeconfig.yaml`:
   - Set `clusters[].cluster.server` to `https://127.0.0.1:<api-port>` (port from the VM's minikube server URL).
   - Add `insecure-skip-tls-verify: true` on that cluster entry.

   Get the port:

   ```bash
   limactl shell <lima-vm> -- minikube kubectl -- config view --minify -o jsonpath='{.clusters[0].cluster.server}'
   ```

5. Start an **IPv4** SSH tunnel from the Mac (skip if Lima already forwards the port — check with `lsof -i :<api-port>`):

   ```bash
   ssh -F ~/.lima/<lima-vm>/ssh.config -N \
     -L 127.0.0.1:<api-port>:127.0.0.1:<api-port> <lima-ssh-host> -f
   ```

   `<lima-ssh-host>` is the `Host` entry from `~/.lima/<lima-vm>/ssh.config`.

6. Use the cluster from the Mac:

   ```bash
   export KUBECONFIG=/tmp/vm-kubeconfig.yaml
   kubectl get nodes
   ```

   Keep the tunnel (if used) running for the whole session. Re-export kubeconfig and update the tunnel port after `minikube delete`/recreate.

## One-time cluster bootstrap

Run from repo root. Substitute `<GITHUB_APP_ID>` and `<PATH_TO_PRIVATE_KEY_PEM>` with values from the user.

If using the Lima workflow, prefix commands with `export KUBECONFIG=/tmp/vm-kubeconfig.yaml`.

### 1) Install CRDs

```bash
make install

kubectl apply -f https://raw.githubusercontent.com/konflux-ci/application-api/main/config/crd/bases/appstudio.redhat.com_components.yaml
```

### 2) Namespaces and service account

```bash
kubectl create namespace mintmaker --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace test-app --dry-run=client -o yaml | kubectl apply -f -
kubectl create serviceaccount mintmaker-controller-manager -n mintmaker \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 3) GitHub App secret

```bash
kubectl create secret generic pipelines-as-code-secret -n mintmaker \
  --from-literal=github-application-id="<GITHUB_APP_ID>" \
  --from-file=github-private-key="<PATH_TO_PRIVATE_KEY_PEM>" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Use an **absolute path** to the `.pem` file; quote the path if it contains spaces.

### 4) Renovate ConfigMap

```bash
kubectl create configmap renovate-config -n mintmaker \
  --from-file=renovate.json=config/renovate/renovate.json \
  --from-file=self_hosted.json=config/renovate/self_hosted.json \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 5) Tekton Pipelines

```bash
kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

kubectl rollout status deployment/tekton-pipelines-controller -n tekton-pipelines --timeout=180s
kubectl rollout status deployment/tekton-pipelines-webhook -n tekton-pipelines --timeout=180s
```

### 6) Test Component

Template: `skills/local-controller-testing-workflow-github/assets/test-component.yaml`

Apply with the user's repo URL and revision (replace placeholders — do not commit user-specific values):

```bash
REPO_URL="https://github.com/<owner>/<repo>.git"   # from user
REVISION="<branch-or-tag>"                          # from user

sed -e "s|https://github.com/user/repo-name.git|${REPO_URL}|" \
    -e "s|revision: main|revision: ${REVISION}|" \
    skills/local-controller-testing-workflow-github/assets/test-component.yaml | kubectl apply -f -

kubectl get components -n test-app
```

## Daily dev loop

### 1) Run the controller locally

Terminal A, from repo root:

```bash
export KUBECONFIG=/tmp/vm-kubeconfig.yaml   # Lima workflow only
make run
```

Successful start logs include:

- `starting manager`
- `starting server` (health probe on `:8081`)
- `Starting Controller` for `dependencyupdatecheck`, `pipelinerun`, and `event`

If any controller fails to start, fix that before applying the DUC (e.g. port `8081` in use — see Troubleshooting).

### 2) Trigger via DependencyUpdateCheck

Terminal B:

```bash
kubectl apply -f skills/local-controller-testing-workflow-github/assets/test-duc.yaml
```

Verify (see [What success looks like](#what-success-looks-like) for expected controller log lines):

```bash
kubectl get dependencyupdatecheck test-dependency-check -n mintmaker -o yaml | sed -n '1,80p'
kubectl get pipelineruns -n mintmaker -o wide
kubectl get pods -n mintmaker -o wide
```

### 3) Re-test after code changes

The controller must be restarted. The DUC is processed once (`mintmaker.appstudio.redhat.com/processed: "true"`) — delete and re-apply to re-trigger:

```bash
kubectl delete -f skills/local-controller-testing-workflow-github/assets/test-duc.yaml
kubectl apply -f skills/local-controller-testing-workflow-github/assets/test-duc.yaml
```

## What success looks like

Controller logs are JSON (`make run`); search for the `msg` strings below.

### DUC reconcile (controller smoke test — **minimum bar**)

Apply `test-duc.yaml` while `make run` is active. Within ~10s you should see this **sequence** from `DependencyUpdateCheckController`:

| Order | Log `msg` (substring) | Notes |
| ----- | --------------------- | ----- |
| 1 | `new DependencyUpdateCheck found:` | Includes `mintmaker/test-dependency-check` |
| 2 | `Recorded DependencyUpdateCheck creation metrics` | Metrics recorded |
| 3 | `Following components are specified:` | Lists namespaces from the DUC spec |
| 4 | `1 components will be processed` | Expect **1** when the DUC targets only `test-backend` (see `test-duc.yaml`) |
| 5 | `found components with mintmaker disabled` with `"components":0` | None skipped via `disabled` annotation |
| 6 | `service account not found in component namespace` | **OK locally** — no Konflux `build-pipeline-*` SA |
| 7 | `no viable activation key secret has been found` | **OK locally** — no RPM activation Secret |
| 8 | `created PipelineRun` with `"pipelineRun":"renovate-...` | PipelineRun name like `renovate-06010935-507e82cb` |

**kubectl checks:**

```bash
# DUC marked processed (controller updates this before creating PipelineRuns)
kubectl get dependencyupdatecheck test-dependency-check -n mintmaker \
  -o jsonpath='{.metadata.annotations.mintmaker\.appstudio\.redhat\.com/processed}{"\n"}'
# expect: true

kubectl get pipelineruns -n mintmaker -o wide
# expect: one new row, REASON Running (or Unknown), name matching controller log
```

**Smoke test passed** when items 1–8 and the kubectl checks succeed. You do **not** need the PipelineRun to finish for day-to-day controller work.

### Non-fatal messages (local testing only)

These appear in **info** logs and do not block PipelineRun creation (rows 6–7 in the table above are expected locally):

- `config file not found, using defaults` with `"path":"/etc/mintmaker/config.json"` — no mounted operator config locally

When encountering other failures/errors/warnings, report them.

## Cleanup

Stop controller process in the `make run` terminal.

Light cleanup:

```bash
kubectl delete -f skills/local-controller-testing-workflow-github/assets/test-duc.yaml || true
kubectl delete component test-backend -n test-app || true
```

Cluster teardown:

```bash
minikube delete                    # host or inside Lima VM
kind delete cluster                # if using kind
```

## Troubleshooting

### Port 8081 already in use

```bash
lsof -i :8081
kill <pid>
# If the process does not shut down, force terminate it:
# kill -9 <pid>
```

### `exec format error` in Tekton steps (arm64 nodes)

PipelineRun is created but a step fails with `exec format error` — amd64-only image on an arm64 node. Use an amd64 minikube node (Lima QEMU VM) or wait for multi-arch pipeline images.

### Insufficient memory / Pending PipelineRun pods

Symptom: `ExceededNodeResources`, `Insufficient memory`, or pods stuck `Pending` while another Renovate pod is running.

Fixes:

- Increase minikube memory (`--memory=8192` or higher)
- Ensure only **one** test Component exists in `test-app`
- Wait for one PipelineRun to finish before triggering another

### Slow or stuck image pulls

Renovate pipeline images are large. A pod in `PodInitializing` pulling `mintmaker-renovate-image:latest` for 10+ minutes may still be normal on first pull. Check events:

```bash
kubectl describe pod <pipelinerun-pod> -n mintmaker
```

### Lima: `kubectl` connection refused from Mac

- Tunnel running? `lsof -i :<api-port>` should show `ssh` or `limactl` on **IPv4**
- Minikube running in VM? `limactl shell <lima-vm> -- minikube status`
- API port changed? Re-export `/tmp/vm-kubeconfig.yaml` and restart the tunnel

### Pod DNS failures inside Lima minikube

If pipeline pods cannot resolve external hostnames but can reach `8.8.8.8`, run the controller with:

```bash
export MINTMAKER_USE_PUBLIC_DNS=1
make run
```

This affects pipeline pod DNS only. In-cluster names (e.g. Redis) may not resolve when this is set.
