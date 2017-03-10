package testutil

import (
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
)

const (
	TestDir      = "config_test"
	TestSettings = "evg_settings.yml"
)

// TestConfig creates test settings from a test config.
func TestConfig() *evergreen.Settings {
	file := filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings)
	settings, err := evergreen.NewSettings(file)
	if err != nil {
		panic(err)
	}
	return settings
}
