package pod

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workingDir = "/data/mci"

func TestCurlCommandWithRetry(t *testing.T) {
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
			cmd, err := p.CurlCommandWithRetry(settings, 5, 10)
			require.NoError(t, err)
			require.NotZero(t, cmd)
			expected := "CMD-SHELL curl -LO 'www.test.com/clients/amd64/evergreen' --retry 5 --retry-max-time 10 && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen"
			assert.Equal(t, expected, cmd)
		})
		t.Run("Windows", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSWindows,
					Arch:       evergreen.ArchWindowsAmd64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			cmd, err := p.CurlCommandWithRetry(settings, 5, 10)
			require.NoError(t, err)
			require.NotZero(t, cmd)
			expected := "CMD-SHELL curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 5 --retry-max-time 10 && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen.exe"
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
			t.Run("CustomRetry", func(t *testing.T) {
				cmd, err := p.CurlCommandWithRetry(settings, 5, 10)
				require.NoError(t, err)
				require.NotZero(t, cmd)
				expected := fmt.Sprintf("CMD-SHELL (curl -LO 'https://foo.com/%s/amd64/evergreen' --retry 5 --retry-max-time 10 || curl -LO 'www.test.com/clients/amd64/evergreen' --retry 5 --retry-max-time 10) && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen", evergreen.BuildRevision)
				assert.Equal(t, expected, cmd)
			})
			t.Run("DefaultRetry", func(t *testing.T) {
				cmd, err := p.CurlCommandWithDefaultRetry(settings)
				require.NoError(t, err)
				require.NotZero(t, cmd)
				expected := fmt.Sprintf("CMD-SHELL (curl -LO 'https://foo.com/%s/amd64/evergreen' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/amd64/evergreen' --retry 10 --retry-max-time 100) && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen", evergreen.BuildRevision)
				assert.Equal(t, expected, cmd)
			})
		})
		t.Run("Windows", func(t *testing.T) {
			p := &Pod{
				ID: "id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					OS:         OSWindows,
					Arch:       evergreen.ArchWindowsAmd64,
					WorkingDir: workingDir,
				},
				Secret: "secret",
			}
			t.Run("CustomRetry", func(t *testing.T) {
				cmd, err := p.CurlCommandWithRetry(settings, 5, 10)
				require.NoError(t, err)
				require.NotZero(t, cmd)
				expected := fmt.Sprintf("CMD-SHELL (curl -LO 'https://foo.com/%s/windows_amd64/evergreen.exe' --retry 5 --retry-max-time 10 || curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 5 --retry-max-time 10) && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen.exe", evergreen.BuildRevision)
				assert.Equal(t, expected, cmd)
			})
			t.Run("DefaultRetry", func(t *testing.T) {
				cmd, err := p.CurlCommandWithDefaultRetry(settings)
				require.NoError(t, err)
				require.NotZero(t, cmd)
				expected := fmt.Sprintf("CMD-SHELL (curl -LO 'https://foo.com/%s/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100 || curl -LO 'www.test.com/clients/windows_amd64/evergreen.exe' --retry 10 --retry-max-time 100) && ./evergreen agent --api_server=www.test.com --mode=pod --pod_id=id --pod_secret=secret --log_prefix=/data/mci/agent --working_directory=/data/mci --cleanup && chmod +x evergreen.exe", evergreen.BuildRevision)
				assert.Equal(t, expected, cmd)
			})
		})
	})
}
