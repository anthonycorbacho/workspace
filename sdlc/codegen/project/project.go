// Package project generates a project (service) in a namespace: a minimal main using the kit foundation,
// its kubernetes files, its Tiltfile and its CI workflows, registered in the namespace and the root Tiltfile.
package project

import (
	"fmt"
	"maps"
	"path"
	"slices"

	"github.com/anthonycorbacho/workspace/sdlc/codegen"
)

// Step generates the project files.
type Step struct{}

var _ codegen.Step = Step{}

// Generate creates the project files, it fails if a file already exists (unless the workspace forces it).
func (Step) Generate(state codegen.State, ws *codegen.Workspace) error {
	dir := path.Join(state.Namespace, state.Project)
	files := map[string]string{
		path.Join(dir, "cmd", state.Project, "main.go"): "project/main.go.tmpl",
		path.Join(dir, "kube.yaml"):                     "project/kube.yaml.tmpl",
		path.Join(dir, "Tiltfile"):                      "project/Tiltfile.tmpl",
		path.Join(dir, "README.md"):                     "project/README.md.tmpl",
		fmt.Sprintf(".github/workflows/%s.%s.test.yaml", state.Namespace, state.Project):  "project/workflow.test.yaml.tmpl",
		fmt.Sprintf(".github/workflows/%s.%s.build.yaml", state.Namespace, state.Project): "project/workflow.build.yaml.tmpl",
	}
	// sorted for a stable order of the changes.
	for _, target := range slices.Sorted(maps.Keys(files)) {
		content, err := codegen.Render(files[target], state)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", target, err)
		}
		if err := ws.Create(target, content); err != nil {
			return err
		}
	}

	if err := ws.Update(path.Join(state.Namespace, "kustomization.yaml"), func(content string) string {
		return codegen.AddResource(content, state.Project+"/kube.yaml")
	}); err != nil {
		return err
	}
	return ws.Update("Tiltfile", func(content string) string {
		return codegen.AddLine(content, fmt.Sprintf("include('./%s/Tiltfile')", dir), state.Namespace+"/"+state.Project)
	})
}
