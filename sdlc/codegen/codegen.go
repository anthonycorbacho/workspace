// Package codegen scaffolds new namespaces and projects in the workspace monorepo, following its conventions:
// kubernetes files registered in the local overlay, a minimal main using the kit foundation, a Tiltfile included
// in the root Tiltfile and the CI workflows. The developer then builds the service from the generated skeleton.
//
// The generation is done in two phases: the steps (see namespace and project) record their changes in a
// Workspace, then the changes are written all at once, so nothing is written if a step fails.
package codegen

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State is the input of the steps.
type State struct {
	// Root is the root directory of the workspace (where go.mod is).
	Root string
	// Module is the Go module path of the workspace, read from go.mod (eg: github.com/acme/workspace).
	Module string
	// Namespace groups projects (eg: sample), it is a kubernetes namespace and a directory of the workspace.
	Namespace string
	// Project is the service to create in the namespace (eg: sampleapp).
	Project string
}

// Title returns the project name with an upper case first letter, for Go identifiers (eg: Sampleapp).
func (s State) Title() string {
	if s.Project == "" {
		return ""
	}
	return strings.ToUpper(s.Project[:1]) + s.Project[1:]
}

// Step generates the changes of a part of the workspace (eg: the namespace files).
type Step interface {
	Generate(state State, ws *Workspace) error
}

// ModulePath returns the module path declared in the go.mod of the root directory.
func ModulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("reading the workspace go.mod (is %q the workspace root?): %w", root, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if module, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
			return strings.TrimSpace(module), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("no module directive in go.mod")
}
