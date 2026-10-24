package codegen

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
)

// _name is a valid name for a namespace or a project: a kubernetes name (DNS-1123 label)
// that is also a valid Go package and directory name.
var _name = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// _reserved are the directories of the workspace that can't be namespaces.
var _reserved = []string{"api", "bin", "default", "foundation", "kit", "overlays", "sdlc", "vendor"}

// ValidateName validates a namespace or project name.
func ValidateName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if len(name) > 63 || !_name.MatchString(name) {
		return fmt.Errorf("%s %q must be lowercase letters and digits, starting with a letter (max 63)", kind, name)
	}
	if kind == "namespace" && slices.Contains(_reserved, name) {
		return fmt.Errorf("namespace %q is a reserved directory of the workspace", name)
	}
	return nil
}

// ProjectNamespace returns the namespace of an existing project with the given name, or "" if none.
// Project names are unique in the workspace since they name the Tilt resources and the images.
func ProjectNamespace(root, project string) string {
	matches, err := filepath.Glob(filepath.Join(root, "*", project, "kube.yaml"))
	if err != nil || len(matches) == 0 {
		// the pattern is only invalid if the root contains glob characters, there is nothing to compare.
		return ""
	}
	return filepath.Base(filepath.Dir(filepath.Dir(matches[0])))
}

// Namespaces returns the existing namespaces of the workspace (directories with a namespace.yaml).
func Namespaces(root string) []string {
	matches, err := filepath.Glob(filepath.Join(root, "*", "namespace.yaml"))
	if err != nil {
		return nil
	}
	namespaces := make([]string, 0, len(matches))
	for _, m := range matches {
		namespaces = append(namespaces, filepath.Base(filepath.Dir(m)))
	}
	return namespaces
}
