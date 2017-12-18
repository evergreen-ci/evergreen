package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigChanged(t *testing.T) {
	assert := assert.New(t) //nolint
	remoteConfigPath := "config/evergreen.yml"
	p := &Patch{
		Patches: []ModulePatch{{
			PatchSet: PatchSet{
				Summary: []Summary{{
					Name:      remoteConfigPath,
					Additions: 3,
					Deletions: 3,
				}},
			},
		}},
	}

	assert.True(p.ConfigChanged(remoteConfigPath))

	p.Patches[0].PatchSet.Summary[0].Name = "dakar"
	assert.False(p.ConfigChanged(remoteConfigPath))
}
