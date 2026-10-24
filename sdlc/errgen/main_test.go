package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	cfg := config{
		input:  "testdata/catalog.yaml",
		output: filepath.Join(dir, "errors", "error.gen.go"),
		docs:   filepath.Join(dir, "docs"),
		pkg:    "errors",
	}
	require.NoError(t, run(cfg))

	golden, err := os.ReadFile("testdata/catalog.gen.go.golden")
	require.NoError(t, err)
	code, err := os.ReadFile(cfg.output)
	require.NoError(t, err)
	assert.Equal(t, string(golden), string(code))
	assert.Equal(t, 5, countMarkdown(t, cfg.docs))

	check := cfg
	check.check = true
	require.NoError(t, run(check))

	// the stability check of a pull request compares nothing.
	require.NoError(t, run(config{input: cfg.input, base: cfg.input, check: true}))

	require.NoError(t, os.WriteFile(cfg.output, []byte("package errors\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.docs, "err_00002.md"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.docs, "err_00099.md"), []byte("removed"), 0o644))
	err = run(check)
	require.Error(t, err)
	assert.Equal(t, "error.gen.go is not up to date, run make errgen\n"+
		"docs "+cfg.docs+" are not up to date (err_00002.md, err_00099.md), run make errgen", err.Error())

	// regenerating removes the stale page.
	require.NoError(t, run(cfg))
	require.NoError(t, run(check))
	assert.NoFileExists(t, filepath.Join(cfg.docs, "err_00099.md"))
}

func TestRunInvalid(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}
	base := write("base.yaml", catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
`))
	changed := write("changed.yaml", catalogYAML(`
  - {name: A, code: ERR_00002, kind: INTERNAL, message: a}
`))
	invalid := write("invalid.yaml", catalogYAML(`
  - {name: A, code: ERR_00001, kind: OK, message: a}
`))

	tests := []struct {
		name    string
		cfg     config
		wantErr string
	}{
		{
			name:    "invalid catalog",
			cfg:     config{input: invalid},
			wantErr: invalid + ":5: A: kind OK is not an error kind",
		},
		{
			name:    "unstable codes",
			cfg:     config{input: changed, base: base},
			wantErr: changed + ":5: A: code changed from ERR_00001 to ERR_00002, a released code must never change",
		},
		{
			name:    "unstable codes check",
			cfg:     config{input: changed, base: base, check: true},
			wantErr: changed + ":5: A: code changed from ERR_00001 to ERR_00002, a released code must never change",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			tt.cfg.output = filepath.Join(out, "error.gen.go")
			tt.cfg.docs = filepath.Join(out, "docs")
			tt.cfg.pkg = "errors"

			err := run(tt.cfg)
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
			assert.NoFileExists(t, tt.cfg.output)
			assert.NoDirExists(t, tt.cfg.docs)
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config
		wantErr string
	}{
		{name: "generate", cfg: config{input: "a.yaml", output: "a.go", docs: "docs", pkg: "errors"}},
		{name: "check", cfg: config{input: "a.yaml", output: "a.go", docs: "docs", pkg: "errors", check: true}},
		{name: "stability check", cfg: config{input: "a.yaml", base: "b.yaml", pkg: "errors", check: true}},
		{name: "missing input", cfg: config{output: "a.go", docs: "docs", pkg: "errors"}, wantErr: "-input is required"},
		{name: "missing docs", cfg: config{input: "a.yaml", output: "a.go", pkg: "errors"}, wantErr: "-output and -docs are required"},
		{name: "check without outputs", cfg: config{input: "a.yaml", pkg: "errors", check: true}, wantErr: "-output and -docs are required"},
		{name: "invalid package", cfg: config{input: "a.yaml", output: "a.go", docs: "docs", pkg: "my-errors"}, wantErr: `-package "my-errors" is not a valid package name`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
