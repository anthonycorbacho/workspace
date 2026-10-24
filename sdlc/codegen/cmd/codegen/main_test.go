package main

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _update = flag.Bool("update", false, "update the golden files")

// _fixture is a minimal workspace: go.mod, the local overlay, the root Tiltfile and the sample namespace.
const _fixture = "../../testdata/repo"

// newWorkspace copies the fixture workspace in a temporary directory.
func newWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS(_fixture)))
	return root
}

// readTree returns the files of a directory by relative path.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = string(b)
		return err
	}))
	return files
}

// assertGolden compares the workspace with the golden directory, -update rewrites the golden directory.
func assertGolden(t *testing.T, root, golden string) {
	t.Helper()
	if *_update {
		require.NoError(t, os.RemoveAll(golden))
		require.NoError(t, os.CopyFS(golden, os.DirFS(root)))
	}
	assert.Equal(t, readTree(t, golden), readTree(t, root))
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "new_namespace", args: []string{"-namespace", "demo", "-project", "demoapp"}},
		{name: "existing_namespace", args: []string{"-namespace", "sample", "-project", "billing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newWorkspace(t)
			var out bytes.Buffer
			require.NoError(t, run(append([]string{"-root", root}, tt.args...), &out, false))
			assertGolden(t, root, filepath.Join("../../testdata/golden", tt.name))
		})
	}
}

func TestGenerateTwice(t *testing.T) {
	root := newWorkspace(t)
	args := []string{"-root", root, "-namespace", "demo", "-project", "demoapp"}
	require.NoError(t, run(args, &bytes.Buffer{}, false))
	first := readTree(t, root)

	err := run(args, &bytes.Buffer{}, false)
	assert.ErrorContains(t, err, "already exists, use --force")
	assert.Equal(t, first, readTree(t, root), "nothing is written when a file already exists")

	require.NoError(t, run(append(args, "-force"), &bytes.Buffer{}, false))
	assert.Equal(t, first, readTree(t, root), "the registrations are not duplicated")
}

func TestDryRun(t *testing.T) {
	root := newWorkspace(t)
	before := readTree(t, root)

	var out bytes.Buffer
	require.NoError(t, run([]string{"-root", root, "-namespace", "demo", "-project", "demoapp", "-dry-run"}, &out, false))
	assert.Equal(t, before, readTree(t, root), "a dry run writes nothing")
	assert.Contains(t, out.String(), "create demo/demoapp/cmd/demoapp/main.go")
	assert.Contains(t, out.String(), "update overlays/local/kustomization.yaml")
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing values without terminal", args: []string{"-namespace", "demo"}, wantErr: "-namespace and -project are required"},
		{name: "invalid namespace", args: []string{"-namespace", "Demo", "-project", "app"}, wantErr: "must be lowercase"},
		{name: "reserved namespace", args: []string{"-namespace", "kit", "-project", "app"}, wantErr: "reserved directory"},
		{name: "project in another namespace", args: []string{"-namespace", "demo", "-project", "sampleapp"}, wantErr: `already exists in the namespace "sample"`},
		{name: "unknown flag", args: []string{"-unknown"}, wantErr: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newWorkspace(t)
			before := readTree(t, root)
			err := run(append([]string{"-root", root}, tt.args...), &bytes.Buffer{}, false)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, before, readTree(t, root))
		})
	}
}

func TestNotAWorkspace(t *testing.T) {
	err := run([]string{"-root", t.TempDir(), "-namespace", "demo", "-project", "app"}, &bytes.Buffer{}, false)
	assert.ErrorContains(t, err, "the workspace root?")
}
