package config

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestEnvVarOverride(t *testing.T) {
	var given = bytes.NewBuffer([]byte(`
name: kasabian
db:
  hostAndPort: localhost:lol
  user: auser
  password: apassword
`))

	// override by env var.
	os.Setenv("DB_HOST_AND_PORT", "database.svc.cluster.local:5432")

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
