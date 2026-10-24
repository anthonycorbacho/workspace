package codegen

import (
	"regexp"
	"strings"
)

var _listItem = regexp.MustCompile(`^(\s*)- (.*)$`)

// AddResource adds an item to the `resources` list of a kustomization, keeping the file as is (comments, indent).
// The content is returned unchanged if the item is already listed.
func AddResource(content, item string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "resources:" {
			start = i
			break
		}
	}
	if start == -1 {
		return strings.Join(lines, "\n") + "\nresources:\n- " + item + "\n"
	}

	last, indent := start, ""
	for i := start + 1; i < len(lines); i++ {
		m := _listItem.FindStringSubmatch(lines[i])
		if m == nil {
			if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
				continue
			}
			break
		}
		if strings.TrimSpace(m[2]) == item {
			return content
		}
		last, indent = i, m[1]
	}

	lines = append(lines[:last+1], append([]string{indent + "- " + item}, lines[last+1:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

// AddLine appends a line (and an optional comment above it) to a file if the line is not already present.
func AddLine(content, line, comment string) string {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == line {
			return content
		}
	}
	content = strings.TrimRight(content, "\n") + "\n\n"
	if comment != "" {
		content += "# " + comment + "\n"
	}
	return content + line + "\n"
}
