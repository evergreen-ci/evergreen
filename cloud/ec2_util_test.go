package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
)

func TestCleanLaunchTemplateName(t *testing.T) {
	for name, params := range map[string]struct {
		input    string
		expected string
	}{
		"AlreadyClean": {
			input:    "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
			expected: "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
		},
		"IncludesInvalid": {
			input:    "abcdef*123456",
			expected: "abcdef123456",
		},
		"AllInvalid": {
			input:    "!@#$%^&*",
			expected: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expected, cleanLaunchTemplateName(params.input))
		})
	}
}

func TestUsesHourlyBilling(t *testing.T) {
	for name, testCase := range map[string]struct {
		distro distro.Distro
		hourly bool
	}{
		"ubuntu": {
			distro: distro.Distro{
				Id:   "ubuntu",
				Arch: "linux",
			},
			hourly: false,
		},
		"windows": {
			distro: distro.Distro{
				Id:   "windows-large",
				Arch: "windows",
			},
			hourly: false,
		},
		"suse": {
			distro: distro.Distro{
				Id:   "suse15-large",
				Arch: "linux",
			},
			hourly: true,
		},
		"mac": {
			distro: distro.Distro{
				Id:   "macos-1100",
				Arch: "osx",
			},
			hourly: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, UsesHourlyBilling(&testCase.distro), testCase.hourly)
		})
	}
}
