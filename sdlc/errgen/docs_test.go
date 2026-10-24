package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPage(t *testing.T) {
	c, err := load("testdata/catalog.yaml")
	require.NoError(t, err)

	pages := renderDocs(c)
	assert.Len(t, pages, len(c.Errors)+1)
	assert.Equal(t, "# ERR_00001\n\n"+
		"## Message\n\n"+
		"quota of `<USED>`% reached for \"`<ACCOUNT>`\"\n\n"+

		"## Details\n\n"+
		"| Name | Type |\n"+
		"| --- | --- |\n"+
		"| used | float64 |\n"+
		"| account | string |\n\n"+
		"## Further help\n\n"+
		"Contact [help@acme.test](mailto:help@acme.test?subject=ERR_00001) and include the error code `ERR_00001`.\n\n"+
		"[All errors](README.md)\n",
		string(pages["err_00001.md"]))

	internalOnly := string(pages["err_00004.md"])
	assert.Contains(t, internalOnly, "data loss \\| \\<stored\\> \\*chunk\\*\n")

	assert.Contains(t, internalOnly, "This error has no details.\n")
	assert.NotContains(t, internalOnly, "http_url_path")

	// the kind and the status are transport details, not part of the customer documentation.
	for name, page := range pages {
		for _, text := range []string{"Kind", "RESOURCE_EXHAUSTED", "Status", "429"} {
			assert.NotContains(t, string(page), text, name)
		}
	}
}

func TestRenderIndex(t *testing.T) {
	c, err := load("testdata/catalog.yaml")
	require.NoError(t, err)

	index := string(renderDocs(c)[readme])
	assert.Contains(t, index, "# Errors\n\nWhen a request to Acme fails")
	assert.Contains(t, index, "[help@acme.test](mailto:help@acme.test)")
	assert.Contains(t, index, "## Common errors\n\n"+
		"| Code | Message |\n"+
		"| --- | --- |\n"+
		"| [ERR_00001](err_00001.md) | quota of `<USED>`% reached for \"`<ACCOUNT>`\" |\n\n")
	assert.Contains(t, index, "## All errors\n\n")
	assert.Contains(t, index, "| [ERR_00004](err_00004.md) | data loss \\| \\<stored\\> \\*chunk\\* |\n")
}

func TestWriteDocs(t *testing.T) {
	pages := map[string][]byte{"README.md": []byte("index"), "err_00001.md": []byte("a")}

	// existing are the files of the docs dir before writing, a name ending with / is a dir.
	tests := []struct {
		name     string
		existing map[string]string
		wantErr  string
	}{
		{name: "missing dir"},
		{name: "empty dir", existing: map[string]string{}},
		{name: "stale pages are removed", existing: map[string]string{"README.md": "old", "err_00002.md": "removed error"}},
		{name: "non markdown file", existing: map[string]string{"err_00002.md": "b", "notes.txt": "keep me"}, wantErr: "found: notes.txt"},
		{name: "sub dir", existing: map[string]string{"err_00002.md": "b", "images/": ""}, wantErr: "found: images"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "docs")
			if tt.existing != nil {
				require.NoError(t, os.Mkdir(dir, 0o755))
			}
			for name, content := range tt.existing {
				if strings.HasSuffix(name, "/") {
					require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o755))
					continue
				}
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
			}

			err := writeDocs(dir, pages)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				// nothing is deleted nor written when the dir is refused.
				assert.Equal(t, tt.existing, readDir(t, dir))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"README.md": "index", "err_00001.md": "a"}, readDir(t, dir))

			outdated, err := outdatedDocs(dir, pages)
			require.NoError(t, err)
			assert.Empty(t, outdated)
		})
	}
}

func TestOutdatedDocs(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"README.md": "index", "err_00001.md": "old", "err_00003.md": "removed"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	outdated, err := outdatedDocs(dir, map[string][]byte{
		"README.md":    []byte("index"),
		"err_00001.md": []byte("new"),
		"err_00002.md": []byte("added"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"err_00001.md", "err_00002.md", "err_00003.md"}, outdated)
}

func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			files[e.Name()+"/"] = ""
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		files[e.Name()] = string(content)
	}
	return files
}

func countMarkdown(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	require.NoError(t, err)
	return len(matches)
}
