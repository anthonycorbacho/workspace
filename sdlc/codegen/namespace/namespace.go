// Package namespace generates a namespace of the workspace: the kubernetes namespace and service account,
// registered in the local overlay.
package namespace

import (
	"path"

	"github.com/anthonycorbacho/workspace/sdlc/codegen"
)

// _overlay is the kustomization deploying the namespaces locally (see the root Tiltfile).
const _overlay = "overlays/local/kustomization.yaml"

// Step generates the namespace files when the namespace doesn't exist yet.
type Step struct{}

var _ codegen.Step = Step{}

// Generate creates the namespace files and registers the namespace in the local overlay,
// an existing namespace is left as is.
func (Step) Generate(state codegen.State, ws *codegen.Workspace) error {
	if ws.Exists(path.Join(state.Namespace, "namespace.yaml")) {
		return nil
	}
	for _, name := range []string{"namespace.yaml", "service-account.yaml", "kustomization.yaml"} {
		content, err := codegen.Render("namespace/"+name+".tmpl", state)
		if err != nil {
			return err
		}
		if err := ws.Create(path.Join(state.Namespace, name), content); err != nil {
			return err
		}
	}
	return ws.Update(_overlay, func(content string) string {
		return codegen.AddResource(content, "../../"+state.Namespace)
	})
}
