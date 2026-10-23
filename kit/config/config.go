package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"

	envconfig "github.com/sethvargo/go-envconfig"
	"gopkg.in/yaml.v3"
)

// _configMapPath is the path where the configmap is mounted.
var _configMapPath = "/etc/app/config.yaml"

// From read from an io (file, byte buffer) and populates the specified struct based on
// the input and environment variables.
//
// The yaml input is applied first, then the environment variables (`env` tag):
//
//   - a variable only overrides a value set by the yaml input when the tag has the `overwrite` option.
//
//   - the `default` option is only used when the field has no value (neither from yaml nor env).
//
//     c := struct {
//     Name string `yaml:"name" env:"APP_NAME,overwrite,default=app"`
//     St   struct {
//     Name string
//     Port int
//     List []string
//     } `yaml:"astruct"`
//     }{}
func From(in io.Reader, i any) error {
	if err := validateTarget(i); err != nil {
		return err
	}

	// Unmarshal the input to the given struct, an empty input (eg: empty configmap)
	// is valid and only relies on env variables and defaults.
	if err := yaml.NewDecoder(in).Decode(i); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	// In order to apply the env var override, we need to process it at this stage.
	return envconfig.Process(context.TODO(), i)
}

// FromConfigMap read a config from the path `/etc/app/config.yaml` (see From),
// only the environment variables are used if the file doesn't exist.
func FromConfigMap(i any) error {
	if err := validateTarget(i); err != nil {
		return err
	}

	// because we want to be able to read file from env and configmap
	// might not be given or mounted, we need to continue
	b, err := os.ReadFile(_configMapPath)
	if errors.Is(err, fs.ErrNotExist) {
		return envconfig.Process(context.TODO(), i)
	}
	if err != nil {
		return err
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

// validateTarget makes sure the target can be populated, the yaml and env decoders panic otherwise.
func validateTarget(i any) error {
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config target must be a non-nil pointer to a struct, got %T", i)
	}
	return nil
}
