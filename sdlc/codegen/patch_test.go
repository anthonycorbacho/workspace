package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddResource(t *testing.T) {
	tests := []struct {
		name    string
		content string
		item    string
		want    string
	}{
		{
			name:    "append to the list",
			content: "kind: Kustomization\nresources:\n- ../../sample\n",
			item:    "../../demo",
			want:    "kind: Kustomization\nresources:\n- ../../sample\n- ../../demo\n",
		},
		{
			name:    "file without trailing newline",
			content: "resources:\n- namespace.yaml\n- sampleapp/kube.yaml",
			item:    "billing/kube.yaml",
			want:    "resources:\n- namespace.yaml\n- sampleapp/kube.yaml\n- billing/kube.yaml\n",
		},
		{
			name:    "indented list and following key",
			content: "resources:\n  - a\n  # comment\n  - b\nimages: []\n",
			item:    "c",
			want:    "resources:\n  - a\n  # comment\n  - b\n  - c\nimages: []\n",
		},
		{
			name:    "already listed",
			content: "resources:\n- a\n- b\n",
			item:    "a",
			want:    "resources:\n- a\n- b\n",
		},
		{
			name:    "no resources",
			content: "kind: Kustomization\n",
			item:    "a",
			want:    "kind: Kustomization\nresources:\n- a\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AddResource(tt.content, tt.item))
		})
	}
}

func TestAddLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "append with comment", content: "k8s_yaml('a')\n", want: "k8s_yaml('a')\n\n# demo/demoapp\ninclude('./demo')\n"},
		{name: "already present", content: "include('./demo')\n", want: "include('./demo')\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AddLine(tt.content, "include('./demo')", "demo/demoapp"))
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		kind    string
		name    string
		wantErr bool
	}{
		{kind: "project", name: "billing"},
		{kind: "project", name: "app2"},
		{kind: "project", name: "", wantErr: true},
		{kind: "project", name: "Billing", wantErr: true},
		{kind: "project", name: "my-app", wantErr: true},
		{kind: "project", name: "2app", wantErr: true},
		{kind: "project", name: "kit"},
		{kind: "namespace", name: "kit", wantErr: true},
		{kind: "namespace", name: "vendor", wantErr: true},
		{kind: "namespace", name: "a123456789012345678901234567890123456789012345678901234567890123", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.name, func(t *testing.T) {
			err := ValidateName(tt.kind, tt.name)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
