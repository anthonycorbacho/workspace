package main

import (
	"encoding/json"
	"flag"
	kiterrors "github.com/anthonycorbacho/workspace/kit/errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update the golden files of testdata")

func TestGenerate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		golden string
		// goldenOwned is false for the reference of the repo, which -update must never overwrite.
		goldenOwned bool
	}{
		{name: "kit errors", input: "../../kit/errors/errors.defs.yaml", golden: "../../kit/errors/error.gen.go"},
		{name: "test catalog", input: "testdata/catalog.yaml", golden: "testdata/catalog.gen.go.golden", goldenOwned: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := load(tt.input)
			require.NoError(t, err)
			got, err := generate(c, "errors")
			require.NoError(t, err)

			if *update && tt.goldenOwned {
				require.NoError(t, os.WriteFile(tt.golden, got, 0o644))
			}
			want, err := os.ReadFile(tt.golden)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestGenerateWithoutPlaceholders(t *testing.T) {
	c, err := parseCatalog(t, catalogYAML(`
  - name: Internal
    code: ERR_00000
    kind: INTERNAL
    message: "internal error"
`))
	require.NoError(t, err)

	got, err := generate(c, "apperrors")
	require.NoError(t, err)
	assert.Contains(t, string(got), "package apperrors\n\n// Error codes.\n")
	assert.Contains(t, string(got), "Kind:     KindInternal,\n")
	assert.NotContains(t, string(got), "import")
	assert.NotContains(t, string(got), "fmt")
}

func TestMessageExpr(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
		usesFmt bool
	}{
		{name: "literal", message: `100% "done"`, want: `"100% \"done\""`},
		{name: "placeholder", message: "user {id} not found", want: `fmt.Sprintf("user %v not found", id)`, usesFmt: true},
		{name: "escaped percent", message: "{used}% of {max}", want: `fmt.Sprintf("%v%% of %v", used, max)`, usesFmt: true},
		{name: "repeated placeholder", message: "{a}-{a}", want: `fmt.Sprintf("%v-%v", a, a)`, usesFmt: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments, err := parseMessage(tt.message)
			require.NoError(t, err)
			got, usesFmt := messageExpr(segments)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.usesFmt, usesFmt)
		})
	}
}

func TestSnakeCase(t *testing.T) {
	tests := map[string]string{
		"id":          "id",
		"userId":      "user_id",
		"userID":      "user_id",
		"httpURLPath": "http_url_path",
		"i32":         "i32",
		"v2Name":      "v2_name",
	}
	for in, want := range tests {
		assert.Equal(t, want, snakeCase(in), in)
	}
}

// TestGeneratedCodeCompiles builds the code generated from the test catalog with a stub Error type,
// golden files only prove that the code is well formatted. The package only exists in a build overlay,
// so nothing is written in the module.
func TestGeneratedCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found")
	}

	c, err := load("testdata/catalog.yaml")
	require.NoError(t, err)
	code, err := generate(c, "errors")
	require.NoError(t, err)

	tmp := t.TempDir()
	stub := []byte(`package errors

type Kind int

const (
	KindUnknown Kind = iota
	KindInvalidArgument
	KindNotFound
	KindAlreadyExists
	KindFailedPrecondition
	KindAborted
	KindOutOfRange
	KindUnauthenticated
	KindPermissionDenied
	KindResourceExhausted
	KindCanceled
	KindDeadlineExceeded
	KindUnimplemented
	KindUnavailable
	KindInternal
	KindDataLoss
)

type Error struct {
	Code     string
	Kind     Kind
	Message  string
	Details  map[string]any
	internal map[string]any
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "error.gen.go"), code, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "error.go"), stub, 0o644))

	pkgDir, err := filepath.Abs("errgen_compile_check")
	require.NoError(t, err)
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{
		filepath.Join(pkgDir, "error.gen.go"): filepath.Join(tmp, "error.gen.go"),
		filepath.Join(pkgDir, "error.go"):     filepath.Join(tmp, "error.go"),
	}})
	require.NoError(t, err)
	overlayFile := filepath.Join(tmp, "overlay.json")
	require.NoError(t, os.WriteFile(overlayFile, overlay, 0o644))

	cmd := exec.Command(goBin, "build", "-overlay", overlayFile, "./errgen_compile_check")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// TestKinds checks that the catalog accepts every kit/errors.Kind, by its name, and maps it to its constant.
func TestKinds(t *testing.T) {
	all := 0
	for k := kiterrors.KindUnknown; k <= kiterrors.KindDataLoss; k++ {
		all++
		assert.Contains(t, kinds, k.String(), "kind %s is not accepted in the catalog", k)
	}
	assert.Len(t, kinds, all)
	assert.Equal(t, "KindCanceled", kinds["CANCELLED"])
	assert.Equal(t, "KindNotFound", kinds["NOT_FOUND"])
	assert.NotContains(t, kinds, "OK")
}
