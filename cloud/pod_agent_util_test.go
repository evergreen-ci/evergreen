package cloud

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
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
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					OS:         pod.OSLinux,
					Arch:       pod.ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd := agentScript(settings, p)
			require.NotZero(t, cmd)

			expected := []string{
				"bash", "-c",
				"curl -LO 'www.test.com/clients/linux_amd64/evergreen' --retry 10 --retry-max-time 100 && " +
					"chmod +x evergreen && " +
					"./evergreen agent --api_server=www.test.com --mode=pod --log_prefix=/data/mci/agent --working_directory=/data/mci",
			}
			assert.Equal(t, expected, cmd)
		})
		t.Run("Windows", func(t *testing.T) {
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					OS:         pod.OSWindows,
					Arch:       pod.ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd := agentScript(settings, p)
			require.NotZero(t, cmd)

			expected := []string{
				"cmd.exe", "/c",
				"curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100 && " +
					".\\evergreen.exe agent --api_server=www.test.com --mode=pod --log_prefix=/data/mci/agent --working_directory=/data/mci",
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
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					OS:         pod.OSLinux,
					Arch:       pod.ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd := agentScript(settings, p)
			require.NotZero(t, cmd)

			expected := []string{
				"bash", "-c",
				fmt.Sprintf("(curl -LO 'https://foo.com/%s/linux_amd64/evergreen' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/linux_amd64/evergreen' --retry 10 --retry-max-time 100) && "+
					"chmod +x evergreen && "+
					"./evergreen agent --api_server=www.test.com --mode=pod --log_prefix=/data/mci/agent --working_directory=/data/mci", evergreen.BuildRevision),
			}
			assert.Equal(t, expected, cmd)
		})
		t.Run("Windows", func(t *testing.T) {
			p := &pod.Pod{
				ID: "id",
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					OS:         pod.OSWindows,
					Arch:       pod.ArchAMD64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd := agentScript(settings, p)
			require.NotZero(t, cmd)
			expected := []string{
				"cmd.exe", "/c",
				fmt.Sprintf("(curl -LO 'https://foo.com/%s/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100) && "+
					".\\evergreen.exe agent --api_server=www.test.com --mode=pod --log_prefix=/data/mci/agent --working_directory=/data/mci", evergreen.BuildRevision),
			}
			assert.Equal(t, expected, cmd)
		})
	})
}
