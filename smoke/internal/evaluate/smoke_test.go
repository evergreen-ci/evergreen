package evaluate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/require"
)

// TestSmokeEvaluateWithNoClientConfig verifies that "evergreen evaluate" succeeds
// on a host where ~/.evergreen.yml does not exist, ensuring that evaluate works
// correctly in environments without a client config file (e.g. CI hosts running
// server-side tasks).
func TestSmokeEvaluateWithNoClientConfig(t *testing.T) {
	evgHome := evergreen.FindEvergreenHome()
	require.NotZero(t, evgHome, "EVGHOME must be set")

	cliPath := os.Getenv("CLI_PATH")
	if cliPath == "" {
		cliPath = filepath.Join(evgHome, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
	}
	_, err := os.Stat(cliPath)
	require.NoError(t, err, "CLI binary must exist at '%s'", cliPath)

	// Move ~/.evergreen.yml aside if it exists so that evaluate runs without
	// a client config. Restore it after the test so subsequent tasks on the
	// same (non-ephemeral) host are unaffected.
	configPath := filepath.Join(os.Getenv("HOME"), ".evergreen.yml")
	if _, statErr := os.Stat(configPath); statErr == nil {
		backupPath := configPath + ".smoke-bak"
		require.NoError(t, os.Rename(configPath, backupPath))
		t.Cleanup(func() {
			require.NoError(t, os.Rename(backupPath, configPath))
		})
	}

	projectYAML := `tasks:
  - name: hello
    commands:
      - command: shell.exec
        params:
          script: echo hello
buildvariants:
  - name: bv
    display_name: BV
    run_on: [ubuntu2004-small]
    tasks:
      - name: hello
`
	yamlFile := filepath.Join(t.TempDir(), "project.yml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(projectYAML), 0644))

	cmd := exec.Command(cliPath, "evaluate", "--path", yamlFile)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "evaluate should succeed without client config; output: %s", string(out))
}
