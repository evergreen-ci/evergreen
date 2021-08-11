package pod

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentScript(t *testing.T) {
	const workingDir = "/data/mci"

	t.Run("WithoutS3", func(t *testing.T) {
		settings := &evergreen.Settings{
			ApiUrl:            "www.test.com",
			ClientBinariesDir: "clients",
		}
		t.Run("Linux", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSLinux,
					Arch:       ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd, err := p.AgentScript(settings)
			require.NoError(t, err)
			require.NotZero(t, cmd)

			expected := []string{
				"CMD-SHELL",
				"curl -LO 'www.test.com/clients/linux_amd64/evergreen' --retry 10 --retry-max-time 100 && chmod +x evergreen && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci",
			}
			assert.Equal(t, expected, cmd)
		})
		t.Run("Windows", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSWindows,
					Arch:       ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd, err := p.AgentScript(settings)
			require.NoError(t, err)
			require.NotZero(t, cmd)

			expected := []string{
				"CMD-SHELL",
				"curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100 && chmod +x evergreen.exe && ./evergreen.exe agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci",
			}
			assert.Equal(t, expected, cmd)
		})
	})

	t.Run("WithS3", func(t *testing.T) {
		settings := &evergreen.Settings{
			ApiUrl:            "www.test.com",
			PodInit:           evergreen.PodInitConfig{S3BaseURL: "https://foo.com"},
			ClientBinariesDir: "clients",
		}
		t.Run("Linux", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSLinux,
					Arch:       ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd, err := p.AgentScript(settings)
			require.NoError(t, err)
			require.NotZero(t, cmd)

			expected := []string{
				"CMD-SHELL",
				fmt.Sprintf("(curl -LO 'https://foo.com/%s/linux_amd64/evergreen' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/linux_amd64/evergreen' --retry 10 --retry-max-time 100) && chmod +x evergreen && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci", evergreen.BuildRevision),
			}
			assert.Equal(t, expected, cmd)
		})
		t.Run("Windows", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSWindows,
					Arch:       ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd, err := p.AgentScript(settings)
			require.NoError(t, err)
			require.NotZero(t, cmd)
			expected := []string{
				"CMD-SHELL",
				fmt.Sprintf("(curl -LO 'https://foo.com/%s/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100) && chmod +x evergreen.exe && ./evergreen.exe agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci", evergreen.BuildRevision),
			}
			assert.Equal(t, expected, cmd)
		})
	})
}
