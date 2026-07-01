// Copyright 2024 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Extracts Tekton step Script strings from internal/tekton/pipeline_run_builder.go
// and runs shellcheck on them without changing the embedded source text.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

const pipelineRunBuilderPath = "internal/tekton/pipeline_run_builder.go"

// emptyScriptSkip documents steps with intentionally empty Script fields.
var emptyScriptSkip = map[string]string{
	"log-analyzer": "uses renovate-log-analyzer image entrypoint",
}

// shellcheckExcludes are warnings accepted on extracted Tekton one-liners (see README.md).
var shellcheckExcludes = []string{
	"SC2046", // unquoted find in prepare-rpm-cert matches original Tekton script
}

type stepScript struct {
	name   string
	script string
}

func main() {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find repo root: %v\n", err)
		os.Exit(1)
	}

	builderPath := filepath.Join(repoRoot, pipelineRunBuilderPath)
	steps, err := extractStepScripts(builderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract scripts: %v\n", err)
		os.Exit(1)
	}
	if len(steps) == 0 {
		fmt.Fprintf(os.Stderr, "no Tekton step scripts found in %s\n", pipelineRunBuilderPath)
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "mintmaker-tekton-scripts-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var failed bool
	for _, step := range steps {
		if step.script == "" {
			reason, ok := emptyScriptSkip[step.name]
			if !ok {
				fmt.Fprintf(os.Stderr, "step %q has empty Script (add to emptyScriptSkip if intentional)\n", step.name)
				os.Exit(1)
			}
			fmt.Printf("step %s - skipped (%s)\n", step.name, reason)
			continue
		}

		path := filepath.Join(tmpDir, step.name+".sh")
		// Tekton one-liners have no shebang; tell shellcheck to treat them as bash.
		content := "# shellcheck shell=bash\n" + step.script + "\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}

		if err := runShellcheck(path); err != nil {
			fmt.Printf("step %s - failed\n", step.name)
			failed = true
			continue
		}
		fmt.Printf("step %s - ok\n", step.name)
	}

	if failed {
		os.Exit(1)
	}
}

func runShellcheck(path string) error {
	args := []string{"-s", "bash"}
	for _, code := range shellcheckExcludes {
		args = append(args, "-e", code)
	}
	args = append(args, path)

	cmd := exec.Command("shellcheck", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}

func extractStepScripts(filename string) ([]stepScript, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var steps []stepScript
	var extractErr error
	ast.Inspect(file, func(n ast.Node) bool {
		if extractErr != nil {
			return false
		}
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		name, script, ok, err := stepFields(comp)
		if err != nil {
			pos := fset.Position(comp.Pos())
			extractErr = fmt.Errorf("%s:%d: %w", filename, pos.Line, err)
			return false
		}
		if !ok {
			return true
		}
		if _, dup := seen[name]; dup {
			pos := fset.Position(comp.Pos())
			extractErr = fmt.Errorf("%s:%d: duplicate Tekton step name %q", filename, pos.Line, name)
			return false
		}
		seen[name] = struct{}{}
		steps = append(steps, stepScript{name: name, script: script})
		return true
	})
	if extractErr != nil {
		return nil, extractErr
	}

	sort.Slice(steps, func(i, j int) bool { return steps[i].name < steps[j].name })
	return steps, nil
}

// stepFields inspects a composite literal for Tekton Step Name and Script fields.
// It returns ok=false with no error when the literal is not a step (missing either field).
// It returns an error when both fields are present but Name or Script is not a static string.
func stepFields(comp *ast.CompositeLit) (name, script string, ok bool, err error) {
	var nameExpr, scriptExpr ast.Expr
	var hasNameKey, hasScriptKey bool
	for _, elt := range comp.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		switch fieldName(kv.Key) {
		case "Name":
			hasNameKey = true
			nameExpr = kv.Value
		case "Script":
			hasScriptKey = true
			scriptExpr = kv.Value
		}
	}
	if !hasNameKey || !hasScriptKey {
		return "", "", false, nil
	}
	name, err = evalStringExpr(nameExpr)
	if err != nil {
		return "", "", false, fmt.Errorf("step Name is not a static string: %w", err)
	}
	script, err = evalStringExpr(scriptExpr)
	if err != nil {
		return "", "", false, fmt.Errorf("step Script is not a static string: %w", err)
	}
	return name, script, true, nil
}

func fieldName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func evalStringExpr(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", fmt.Errorf("expected string literal, got %s", e.Kind)
		}
		return strconv.Unquote(e.Value)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", fmt.Errorf("unsupported binary op %s", e.Op)
		}
		left, err := evalStringExpr(e.X)
		if err != nil {
			return "", err
		}
		right, err := evalStringExpr(e.Y)
		if err != nil {
			return "", err
		}
		return left + right, nil
	case *ast.ParenExpr:
		return evalStringExpr(e.X)
	default:
		return "", fmt.Errorf("unsupported script expression %T", expr)
	}
}
