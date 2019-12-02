package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestNeedsReprovision(t *testing.T) {
	for testName, testCase := range map[string]struct {
		d        distro.Distro
		h        *host.Host
		expected host.ReprovisionType
	}{
		"NewLegacyHostsDoNotNeedReprovisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h:        nil,
			expected: host.ReprovisionNone,
		},
		"NewHostsNeedNewProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h:        nil,
			expected: host.ReprovisionToNew,
		},
		"ExistingLegacyHostsWithNewDistroBootstrappingNeedNewProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
				},
			},
			expected: host.ReprovisionToNew,
		},
		"ExistingHostsWithLegacyDistroBootstrappingNeedLegacyProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
			},
			expected: host.ReprovisionToLegacy,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testCase.expected, needsReprovisioning(testCase.d, testCase.h))
		})
	}
}
