package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/host"
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

func TestValidateVolumeIOPS(t *testing.T) {
	for testName, testCase := range map[string]struct {
		volume       *host.Volume
		expectedIOPS int
	}{
		"NotSpecified": {
			volume: &host.Volume{
				Type: VolumeTypeSc1,
				IOPS: 0,
			},
			expectedIOPS: 0,
		},
		"InvalidType": {
			volume: &host.Volume{
				Type: VolumeTypeSc1,
				IOPS: 100,
			},
			expectedIOPS: 0,
		},
		"LessThanMinumum": {
			volume: &host.Volume{
				Type: VolumeTypeGp3,
				Size: 1000,
				IOPS: 100,
			},
			expectedIOPS: 3000,
		},
		"GreaterThanMaximum": {
			volume: &host.Volume{
				Type: VolumeTypeGp3,
				Size: 1000,
				IOPS: 20000,
			},
			expectedIOPS: 16000,
		},
		"GreaterThanRatio": {
			volume: &host.Volume{
				Type: VolumeTypeGp3,
				Size: 10,
				IOPS: 10000,
			},
			expectedIOPS: 5000,
		},
		"RatioLowerThanMinimum": {
			volume: &host.Volume{
				Type: VolumeTypeGp3,
				Size: 1,
				IOPS: 10000,
			},
			expectedIOPS: 3000,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			validateVolumeIOPS(testCase.volume)
			assert.Equal(t, testCase.expectedIOPS, testCase.volume.IOPS)
		})
	}
}

func TestValidateVolumeThroughput(t *testing.T) {
	for testName, testCase := range map[string]struct {
		volume             *host.Volume
		expectedThroughput int
	}{
		"NotSpecified": {
			volume: &host.Volume{
				Type:       VolumeTypeIo1,
				Throughput: 0,
			},
			expectedThroughput: 0,
		},
		"InvalidType": {
			volume: &host.Volume{
				Type:       VolumeTypeIo1,
				Throughput: 500,
			},
			expectedThroughput: 0,
		},
		"LessThanMinumum": {
			volume: &host.Volume{
				Type:       VolumeTypeGp3,
				IOPS:       10000,
				Throughput: 100,
			},
			expectedThroughput: 125,
		},
		"GreaterThanMaximum": {
			volume: &host.Volume{
				Type:       VolumeTypeGp3,
				IOPS:       10000,
				Throughput: 2000,
			},
			expectedThroughput: 1000,
		},
		"GreaterThanRatio": {
			volume: &host.Volume{
				Type:       VolumeTypeGp3,
				IOPS:       1000,
				Throughput: 300,
			},
			expectedThroughput: 250,
		},
		"RatioLowerThanMinimum": {
			volume: &host.Volume{
				Type:       VolumeTypeGp3,
				IOPS:       1,
				Throughput: 250,
			},
			expectedThroughput: 125,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			validateVolumeThroughput(testCase.volume)
			assert.Equal(t, testCase.expectedThroughput, testCase.volume.IOPS)
		})
	}
}
