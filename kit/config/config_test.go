package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromConfigMap(t *testing.T) {
	c := struct {
		Name string `yaml:"name" env:"NAME,default=toto"`
		Eman string `yaml:"eman" env:"EMAN"`
	}{}

	err := FromConfigMap(&c)
	assert.Nil(t, err)
	assert.Equal(t, "toto", c.Name)
	assert.Empty(t, c.Eman)

}

func TestFromConfigMapEnv(t *testing.T) {
	t.Setenv("KIT_CONFIG_TEST_NAME", "from-env")

	c := struct {
		Name string `env:"KIT_CONFIG_TEST_NAME,default=toto"`
	}{}

	err := FromConfigMap(&c)
	require.NoError(t, err)
	assert.Equal(t, "from-env", c.Name)
}

func TestFromConfigMapNotAPointer(t *testing.T) {
	c := struct {
		Name string `env:"KIT_CONFIG_TEST_NAME"`
	}{}

	err := FromConfigMap(c)
	assert.Error(t, err)
}

func TestFrom(t *testing.T) {
	var given = bytes.NewBuffer([]byte(`
name: myApplication
astruct:
  name: a struct name
  port: 4242
  list:
    - a
    - b
    - c
`))

	c := struct {
		Name string
		St   struct {
			Name string `yaml:"name"`
			Port int
			List []string
		} `yaml:"astruct"`
	}{}

	err := From(given, &c)
	assert.Nil(t, err)

	assert.Equal(t, "myApplication", c.Name)
	assert.Equal(t, "a struct name", c.St.Name)
	assert.Equal(t, 4242, c.St.Port)
	assert.Equal(t, 3, len(c.St.List))
}

type fromConfig struct {
	Name    string `yaml:"name" env:"KIT_CONFIG_TEST_NAME"`
	Port    int    `yaml:"port" env:"KIT_CONFIG_TEST_PORT,default=8080"`
	Region  string `yaml:"region" env:"KIT_CONFIG_TEST_REGION,overwrite"`
	Missing string `yaml:"missing" env:"KIT_CONFIG_TEST_MISSING"`
}

func TestFromTable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		env     map[string]string
		want    fromConfig
		wantErr bool
	}{
		{
			name:  "yaml only, env default applied on unset field",
			input: "name: app\nregion: eu",
			want:  fromConfig{Name: "app", Port: 8080, Region: "eu"},
		},
		{
			name:  "yaml value takes precedence over default",
			input: "name: app\nport: 9090",
			want:  fromConfig{Name: "app", Port: 9090},
		},
		{
			name:  "env fills fields absent from yaml",
			input: "port: 1",
			env:   map[string]string{"KIT_CONFIG_TEST_NAME": "from-env", "KIT_CONFIG_TEST_MISSING": "set"},
			want:  fromConfig{Name: "from-env", Port: 1, Missing: "set"},
		},
		{
			// without the `overwrite` option, values set by yaml are kept.
			name:  "env does not overwrite yaml value without overwrite option",
			input: "name: app",
			env:   map[string]string{"KIT_CONFIG_TEST_NAME": "from-env"},
			want:  fromConfig{Name: "app", Port: 8080},
		},
		{
			name:  "env overwrites yaml value with overwrite option",
			input: "region: eu",
			env:   map[string]string{"KIT_CONFIG_TEST_REGION": "us"},
			want:  fromConfig{Port: 8080, Region: "us"},
		},
		{
			name:  "unknown yaml keys are ignored",
			input: "name: app\nunknown: value",
			want:  fromConfig{Name: "app", Port: 8080},
		},
		{
			// An empty config (eg: an empty configmap) should still apply
			// env vars and defaults instead of failing.
			name:  "empty input",
			input: "",
			env:   map[string]string{"KIT_CONFIG_TEST_NAME": "from-env"},
			want:  fromConfig{Name: "from-env", Port: 8080},
		},
		{
			name:    "invalid yaml",
			input:   "name: [app",
			wantErr: true,
		},
		{
			name:    "yaml type mismatch",
			input:   "port: not-a-number",
			wantErr: true,
		},
		{
			name:    "invalid env value",
			input:   "name: app",
			env:     map[string]string{"KIT_CONFIG_TEST_PORT": "not-a-number"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var got fromConfig
			err := From(bytes.NewBufferString(tt.input), &got)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromReaderError(t *testing.T) {
	var c fromConfig
	err := From(errReader{}, &c)
	assert.Error(t, err)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestEnvVarOverride(t *testing.T) {
	var given = bytes.NewBuffer([]byte(`
name: kasabian
db:
  hostAndPort: localhost:lol
  user: auser
  password: apassword
`))

	// override by env var.
	t.Setenv("DB_HOST_AND_PORT", "database.svc.cluster.local:5432")

	c := struct {
		Name string
		DB   struct {
			HostAndPort string `env:"DB_HOST_AND_PORT,default=localhost:5432"`
		}
	}{}
	err := From(given, &c)
	assert.Nil(t, err)
	assert.Equal(t, "kasabian", c.Name)
	assert.Equal(t, "database.svc.cluster.local:5432", c.DB.HostAndPort)
}

func TestConfigSquash(t *testing.T) {
	type Root struct {
		Project string
	}

	type Child struct {
		Root `yaml:",inline"`
		Data string
	}
	var given = bytes.NewBuffer([]byte(`
project: kasabian
data: one ring to rule them all
`))

	c := Child{}
	err := From(given, &c)
	assert.Nil(t, err)
	assert.Equal(t, "kasabian", c.Root.Project)
	assert.Equal(t, "one ring to rule them all", c.Data)
}

func TestLookupEnv(t *testing.T) {
	const key = "KIT_CONFIG_TEST_LOOKUP"

	tests := []struct {
		name         string
		set          bool
		value        string
		defaultValue string
		want         string
	}{
		{name: "set returns value", set: true, value: "value", defaultValue: "default", want: "value"},
		{name: "set to empty returns empty", set: true, value: "", defaultValue: "default", want: ""},
		{name: "unset returns default", defaultValue: "default", want: "default"},
		{name: "unset with empty default", defaultValue: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv restores the original environment at cleanup.
			t.Setenv(key, tt.value)
			if !tt.set {
				require.NoError(t, os.Unsetenv(key))
			}
			assert.Equal(t, tt.want, LookupEnv(key, tt.defaultValue))
		})
	}
}

func TestInvalidTarget(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name"`
	}
	var nilCfg *cfg
	tests := []struct {
		name   string
		target any
	}{
		{name: "nil", target: nil},
		{name: "not a pointer", target: cfg{}},
		{name: "nil pointer", target: nilCfg},
		{name: "pointer to a non struct", target: new(string)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				assert.Error(t, From(bytes.NewBufferString("name: app"), tt.target))
				assert.Error(t, FromConfigMap(tt.target))
			})
		})
	}
}

func TestFromConfigMapFile(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" env:"KIT_CONFIG_TEST_NAME,default=toto"`
	}
	tests := []struct {
		name    string
		content *string // nil means the file doesn't exist.
		want    string
		wantErr bool
	}{
		{name: "missing file uses env and defaults", want: "toto"},
		{name: "file", content: ptr("name: from-file"), want: "from-file"},
		{name: "empty file uses env and defaults", content: ptr(""), want: "toto"},
		{name: "invalid file", content: ptr("name: [app"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.content != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.content), 0o600))
			}
			previous := _configMapPath
			_configMapPath = path
			t.Cleanup(func() { _configMapPath = previous })

			var c cfg
			err := FromConfigMap(&c)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, c.Name)
		})
	}
}

func TestFromConfigMapUnreadableFile(t *testing.T) {
	previous := _configMapPath
	// a directory exists but can't be read as a file.
	_configMapPath = t.TempDir()
	t.Cleanup(func() { _configMapPath = previous })

	var c struct {
		Name string `env:"KIT_CONFIG_TEST_NAME,default=toto"`
	}
	assert.Error(t, FromConfigMap(&c))
}

func ptr(s string) *string { return &s }
