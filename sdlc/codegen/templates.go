package codegen

import (
	"bytes"
	"embed"
	"go/format"
	"path"
	"strings"
	"text/template"
)

//go:embed templates
var _templates embed.FS

// Render renders an embedded template (eg: "project/main.go.tmpl") with the state.
// The Go files are formatted, so a template error producing invalid Go fails here.
func Render(name string, state State) (string, error) {
	t, err := template.New(path.Base(name)).ParseFS(_templates, "templates/"+name)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, state); err != nil {
		return "", err
	}
	if strings.HasSuffix(name, ".go.tmpl") {
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return "", err
		}
		return string(formatted), nil
	}
	return buf.String(), nil
}
