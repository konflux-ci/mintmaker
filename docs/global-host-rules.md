# Global Renovate host rules

MintMaker administrators can supply shared package service credentials to every new Renovate run. Repositories still declare the indexes and registries they use; MintMaker does not add or change package source URLs.

## Configure the Secret

Create an `Opaque` Secret in the `mintmaker` namespace with a `host-rules.json` data key. Its value is a JSON array. Each rule requires a nonempty `hostType`, an explicit `matchHost`, and either a nonempty `token` or both `username` and `password`. The two credential forms cannot be combined. The array may be empty. Only these five field names are accepted, with the exact capitalization shown; `hostType` is passed to Renovate without an allowlist.

For example, prepare a local `host-rules.json` file with credentials obtained from the service administrators:

```json
[
  {
    "hostType": "pypi",
    "matchHost": "https://index.example.com/simple/",
    "username": "index-user",
    "password": "<password>"
  },
  {
    "hostType": "helm",
    "matchHost": "charts.example.com",
    "token": "<token>"
  }
]
```

Create the Secret without putting its values in a checked-in manifest:

```sh
kubectl -n mintmaker create secret generic shared-renovate-host-rules \
  --from-file=host-rules.json=./host-rules.json
```

Set `host-rules-secret-name` in the operator's `MINTMAKER_CONFIG_PATH` JSON file, then restart the controller so it loads that configuration:

```json
{
  "host-rules-secret-name": "shared-renovate-host-rules"
}
```

When the field is absent or empty, MintMaker creates runs as before. If the field names a missing Secret, a Secret of the wrong type, or invalid rule data, MintMaker reports an error for each affected run without printing credentials. Fix the Secret and create a new `DependencyUpdateCheck`; already processed checks are not retried.

## Scope and rotation

MintMaker validates the Secret when it creates each PipelineRun and mounts only `host-rules.json` read-only on the Renovate step. Renovate reads it when the step starts. Updating the Secret changes the rules used by new runs without a MintMaker code change. Runs that already started keep the rules they loaded at startup. The Secret is shared with all component runs; use service credentials intended for that scope.

Renovate uses both `hostType` and `matchHost` when selecting a rule. A bare hostname matches that host and its subdomains. A `http://` or `https://` URL can restrict the match to a scheme, port, and path prefix. For example, `https://index.example.com/simple/` is narrower than `index.example.com`. MintMaker requires both fields to prevent a rule from applying to every host or every host type. Use the narrowest host that serves the intended packages. See [Renovate host rules](https://docs.renovatebot.com/configuration-options/#hostrules) for matching priority and [private Python packages](https://docs.renovatebot.com/getting-started/private-packages/#python) for repository-side source declarations.

Global rules are appended to each run's existing component Docker registry rules. The global values stay in the Secret and are not copied to the per-run ConfigMap or PipelineRun specification. Component registry credentials remain scoped to that component's run.
