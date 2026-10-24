package codegen

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Action is the kind of change made to a file.
type Action string

const (
	// Create is a new file.
	Create Action = "create"
	// Update is an existing file that is modified (eg: a registration in a kustomization).
	Update Action = "update"
)

// Change is a file to write.
type Change struct {
	Action  Action
	Path    string
	Content string
}

// Workspace records the changes of the steps before writing them, the steps read the pending content of a file
// so a step can update a file created by a previous step (eg: the namespace kustomization).
type Workspace struct {
	root    string
	force   bool
	changes map[string]*Change
	order   []string
}

// NewWorkspace returns a workspace for the given root directory.
// With force, the files that already exist are overwritten instead of failing.
func NewWorkspace(root string, force bool) *Workspace {
	return &Workspace{root: root, force: force, changes: map[string]*Change{}}
}

// Exists reports whether the file exists, either pending or on disk.
func (w *Workspace) Exists(path string) bool {
	if _, ok := w.changes[path]; ok {
		return true
	}
	_, err := os.Stat(filepath.Join(w.root, path))
	return err == nil
}

// Read returns the pending content of the file or its content on disk.
func (w *Workspace) Read(path string) (string, error) {
	if c, ok := w.changes[path]; ok {
		return c.Content, nil
	}
	b, err := os.ReadFile(filepath.Join(w.root, path))
	return string(b), err
}

// Create records a new file, it fails if the file already exists unless force is set.
func (w *Workspace) Create(path, content string) error {
	if w.Exists(path) && !w.force {
		return fmt.Errorf("%s already exists, use --force to overwrite it", path)
	}
	w.set(Create, path, content)
	return nil
}

// Update applies fn to the content of an existing file, the file is not changed if fn returns the same content.
func (w *Workspace) Update(path string, fn func(content string) string) error {
	content, err := w.Read(path)
	if err != nil {
		return fmt.Errorf("updating %s: %w", path, err)
	}
	updated := fn(content)
	if updated == content {
		return nil
	}
	action := Update
	if c, ok := w.changes[path]; ok {
		action = c.Action
	}
	w.set(action, path, updated)
	return nil
}

func (w *Workspace) set(action Action, path, content string) {
	if _, ok := w.changes[path]; !ok {
		w.order = append(w.order, path)
	}
	w.changes[path] = &Change{Action: action, Path: path, Content: content}
}

// Changes returns the recorded changes in the order they were made.
func (w *Workspace) Changes() []Change {
	changes := make([]Change, 0, len(w.order))
	for _, p := range w.order {
		changes = append(changes, *w.changes[p])
	}
	return changes
}

// Print writes the list of changes, it is used for the dry run and the summary.
func (w *Workspace) Print(out io.Writer) {
	changes := w.Changes()
	slices.SortStableFunc(changes, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
	for _, c := range changes {
		fmt.Fprintf(out, "  %-6s %s\n", c.Action, c.Path)
	}
}

// Commit writes the recorded changes.
func (w *Workspace) Commit() error {
	var errs []error
	for _, c := range w.Changes() {
		path := filepath.Join(w.root, c.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.WriteFile(path, []byte(c.Content), fs.FileMode(0o644)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
