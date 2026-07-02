# shellcheck-embedded-tekton-scripts

Extracts `Script:` strings from `internal/tekton/pipeline_run_builder.go` and runs
`shellcheck` on them. Go source is not modified.

```sh
make shellcheck
```

Example output:

```text
step log-analyzer - skipped (uses renovate-log-analyzer image entrypoint)
step prepare-db - ok
step prepare-rpm-cert - ok
step renovate - ok
```

## Skipped

| Step | Reason |
| ---- | ------ |
| `log-analyzer` | Empty `Script`; uses image entrypoint |
| `suite_util_test.go` | Test fixture, not in `pipeline_run_builder.go` |

Empty `Script` on other steps must be listed in `emptyScriptSkip` in `main.go`.

Literals with both `Name` and `Script` must use static string literals (or `+` concatenation). The tool exits with an error if a step script cannot be extracted.

## shellcheck exclusions

| Code | Reason |
| ---- | ------ |
| SC2046 | Unquoted `$(find …)` in `prepare-rpm-cert` |

Workflow shell: `make actionlint`.
