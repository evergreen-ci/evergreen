package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapContainerCommand(t *testing.T) {
	const workingDir = "/data/mci"

	settings := evergreen.Settings{
		ApiUrl: "https://example.com",
	}

	t.Run("Linux", func(t *testing.T) {
		opts := pod.TaskContainerCreationOptions{
			OS:         pod.OSLinux,
			Arch:       pod.ArchAMD64,
			WorkingDir: workingDir,
		}
		cmd := bootstrapContainerCommand(&settings, opts)
		require.NotZero(t, cmd)

		expected := []string{
			"bash", "-c",
			"curl --retry 10 --retry-max-time 60 -L -H \"Pod-Id: ${POD_ID}\" -H \"Pod-Secret: ${POD_SECRET}\" https://example.com/rest/v2/pods/${POD_ID}/provisioning_script | bash -s",
		}
		assert.Equal(t, expected, cmd)
	})
	t.Run("Windows", func(t *testing.T) {
		p := pod.TaskContainerCreationOptions{
			OS:         pod.OSWindows,
			Arch:       pod.ArchAMD64,
			WorkingDir: workingDir,
		}
		cmd := bootstrapContainerCommand(&settings, p)
		require.NotZero(t, cmd)

		expected := []string{
			"powershell.exe", "-noninteractive", "-noprofile", "-Command",
			"curl.exe --retry 10 --retry-max-time 60 -L -H \"Pod-Id: $env:POD_ID\" -H \"Pod-Secret: $env:POD_SECRET\" https://example.com/rest/v2/pods/$env:POD_ID/provisioning_script | powershell.exe -noprofile -noninteractive -",
		}
		assert.Equal(t, expected, cmd)
	})
}
