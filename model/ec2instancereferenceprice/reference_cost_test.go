package ec2instancereferenceprice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperatingSystemFromImageID(t *testing.T) {
	for _, tc := range []struct {
		imageID string
		want    string
	}{
		{"", "linux"},
		{"amazon-linux-2", "linux"},
		{"windows-2022-base", "windows"},
		{"suse-sap-15", "suse"},
		{"SomeSUSEImage", "suse"},
		{"macos-13-xcode", "macos"},
		{"MACOS-13", "macos"},
	} {
		t.Run(tc.imageID, func(t *testing.T) {
			assert.Equal(t, tc.want, OperatingSystemFromImageID(tc.imageID))
		})
	}
}
