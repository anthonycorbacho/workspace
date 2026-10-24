package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const readme = "README.md"

// markdownEscaper escapes the message text that markdown would otherwise interpret, | breaks tables.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "|", `\|`,
)

// renderDocs returns the markdown documentation of a validated catalog, by file name.
func renderDocs(c *catalog) map[string][]byte {
	pages := map[string][]byte{readme: renderIndex(c)}
	for _, e := range c.Errors {
		pages[pageName(e)] = renderPage(c, e)
	}
	return pages
}

func pageName(e errorDef) string {
	return strings.ToLower(e.Code) + ".md"
}

func renderIndex(c *catalog) []byte {
	var b bytes.Buffer
	b.WriteString("# Errors\n\n")
	fmt.Fprintf(&b, "When a request to %s fails, the response contains a unique error code (eg: `%s`) ", c.Docs.Product, c.Errors[0].Code)
	b.WriteString("identifying the error, along with a message and details.\n")
	fmt.Fprintf(&b, "Include this code when contacting support at [%[1]s](mailto:%[1]s).\n", c.Docs.SupportEmail)

	common := slices.DeleteFunc(slices.Clone(c.Errors), func(e errorDef) bool { return !e.Common })
	if len(common) > 0 {
		b.WriteString("\n## Common errors\n\n")
		writeErrorTable(&b, common)
	}
	b.WriteString("\n## All errors\n\n")
	writeErrorTable(&b, c.Errors)
	return b.Bytes()
}

func writeErrorTable(b *bytes.Buffer, errs []errorDef) {
	b.WriteString("| Code | Message |\n")
	b.WriteString("| --- | --- |\n")
	for _, e := range errs {
		fmt.Fprintf(b, "| [%s](%s) | %s |\n", e.Code, pageName(e), messageMarkdown(e))
	}
}

func renderPage(c *catalog, e errorDef) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n", e.Code)
	fmt.Fprintf(&b, "## Message\n\n%s\n\n", messageMarkdown(e))

	b.WriteString("## Details\n\n")
	details := slices.DeleteFunc(slices.Clone(e.Vars), func(v varDef) bool { return v.Internal })
	if len(details) == 0 {
		b.WriteString("This error has no details.\n\n")
	} else {
		b.WriteString("| Name | Type |\n")
		b.WriteString("| --- | --- |\n")
		for _, v := range details {
			fmt.Fprintf(&b, "| %s | %s |\n", markdownEscaper.Replace(snakeCase(v.Name)), v.Type)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Further help\n\n")
	fmt.Fprintf(&b, "Contact [%[1]s](mailto:%[1]s?subject=%[2]s) and include the error code `%[2]s`.\n\n", c.Docs.SupportEmail, e.Code)
	fmt.Fprintf(&b, "[All errors](%s)\n", readme)
	return b.Bytes()
}

// messageMarkdown renders the message with its placeholders as `<NAME>`, the value is only known at runtime.
func messageMarkdown(e errorDef) string {
	var b strings.Builder
	for _, s := range e.Segments {
		if s.placeholder {
			fmt.Fprintf(&b, "`<%s>`", strings.ToUpper(snakeCase(s.text)))
			continue
		}
		b.WriteString(markdownEscaper.Replace(s.text))
	}
	return b.String()
}

// listDocs returns the markdown files of the docs dir, which is owned by the generator.
// Any other entry means the dir is not what we think it is: we refuse to touch it rather than delete it.
func listDocs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var md, others []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".md" {
			md = append(md, entry.Name())
		} else {
			others = append(others, entry.Name())
		}
	}
	if len(others) > 0 {
		return nil, fmt.Errorf("docs dir %s is owned by errgen and must only contain .md files, found: %s", dir, strings.Join(others, ", "))
	}
	return md, nil
}

// writeDocs replaces the markdown files of dir by pages, so that the docs of removed errors don't linger.
func writeDocs(dir string, pages map[string][]byte) error {
	stale, err := listDocs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	for name, content := range pages {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// outdatedDocs returns the files of dir that differ from pages: changed, missing or extra.
func outdatedDocs(dir string, pages map[string][]byte) ([]string, error) {
	existing, err := listDocs(dir)
	if err != nil {
		return nil, err
	}
	var outdated []string
	for _, name := range existing {
		if _, ok := pages[name]; !ok {
			outdated = append(outdated, name)
		}
	}
	for name, content := range pages {
		current, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if !bytes.Equal(current, content) || err != nil {
			outdated = append(outdated, name)
		}
	}
	slices.Sort(outdated)
	return outdated, nil
}
