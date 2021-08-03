package pod

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommandWithRetry(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, p *Pod, settings *evergreen.Settings){
		"WithoutS3Linux": func(t *testing.T, p *Pod, settings *evergreen.Settings) {
			settings = &evergreen.Settings{
				ApiUrl:            "www.test.com",
				ClientBinariesDir: "clients",
			}
			p = &Pod{
				ID: utility.RandomString(),
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:    "image",
					MemoryMB: 128,
					CPU:      128,
					OS:       OSLinux,
					Arch:     ArchARM64,
				},
				Resources: ResourceInfo{
					Cluster: "cluster",
				},
			}
			cmd, err := p.CurlCommandWithRetry(settings, 5, 10)
			require.NoError(t, err)
			require.NotZero(t, cmd)
			// TODO: are retry options before or after URL? 
			expected := "cd /data/mci && CMD-SHELL && curl -LO --retry 5 --retry-max-time 10 'www.test.com/clients/windows_amd64/evergreen.exe' && ./evergreen agent <agent args>"
			// TODO: continue with this mysteriously long command
			assert.Equal(t, expected, cmd)
		},
		"WithoutS3Windows": func(t *testing.T, p *Pod, settings *evergreen.Settings) {
			settings = &evergreen.Settings{
				ApiUrl:            "www.test.com",
				ClientBinariesDir: "clients",
			}
		},
	} {
		t.Run(tName, func(t *testing.T) {
			p := Pod{}

			tCase(t, &p, &evergreen.Settings{}) // TODO: check evergreen settings
		})
	}
}

func TestCurlCommandWithDefaulRetry(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, p *Pod, settings *evergreen.Settings){
		"WithoutS3Linux": func(t *testing.T, p *Pod, settings *evergreen.Settings) {
			settings = &evergreen.Settings{
				ApiUrl:            "www.test.com",
				ClientBinariesDir: "clients",
			}
			p = &Pod{
				ID: utility.RandomString(),
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:    "image",
					MemoryMB: 128,
					CPU:      128,
					OS:       OSLinux,
					Arch:     ArchARM64,
				},
				Resources: ResourceInfo{
					Cluster: "cluster",
				},
			}
			cmd, err := p.CurlCommandWithDefaultRetry(settings)
			require.NoError(t, err)
			require.NotZero(t, cmd)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			p := Pod{}

			tCase(t, &p, &evergreen.Settings{})
		})
	}
}
