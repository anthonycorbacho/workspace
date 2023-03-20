package config

import (
	"bytes"
	"context"
	"io"
	"os"

	envconfig "github.com/sethvargo/go-envconfig"
	"gopkg.in/yaml.v3"
)

// From read from an io (file, byte buffer) and populates the specified struct based on
// the input and environment variables.
//
//	c := struct {
//		Name string
//		St   struct {
//			Name string
//			Port int
//			List []string
//		} `yaml:"astruct"`
//	}{}
func From(in io.Reader, i interface{}) error {
	// Unmarshall the input to the given struct
	if err := yaml.NewDecoder(in).Decode(i); err != nil {
		return err
	}

	// In order to apply the env var override, we need to process it at this stage.
	return envconfig.Process(context.TODO(), i)
}

// FromConfigMap read a config from the path `./config.yaml`.
func FromConfigMap(i interface{}) error {
	// because we want to be able to read file from env and configmap
	// might not be given or mounted, we need to continue
	b, err := os.ReadFile("/etc/app/config.yaml")
	if err != nil {
		return envconfig.Process(context.TODO(), i)
	}
	return From(bytes.NewBuffer(b), i)
}

// LookupEnv retrieves the value of the environment variable named
// by the key. If the variable is present in the environment the
// value is returned.
// Otherwise, the returned value will the defaultValue.
func LookupEnv(key, defaultValue string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	return v
}
