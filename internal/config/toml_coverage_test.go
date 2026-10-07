package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A toml-tagged Config field missing from tomlConfig is silently dropped from
// config.toml (the [okf] table was, before v5).
func TestTomlConfigCoversConfig(t *testing.T) {
	decoded := tomlKeys(reflect.TypeOf(tomlConfig{}))
	for key := range tomlKeys(reflect.TypeOf(Config{})) {
		assert.True(t, decoded[key], "config.toml key %q is not decoded by tomlConfig", key)
	}
}

// A toml-tagged Config field missing from tomlOutput is deleted whenever a crud
// command rewrites config.toml.
func TestTomlOutputCoversConfig(t *testing.T) {
	written := tomlKeys(reflect.TypeOf(tomlOutput{}))
	for key := range tomlKeys(reflect.TypeOf(Config{})) {
		assert.True(t, written[key], "config.toml key %q is not written by tomlOutput", key)
	}
}
