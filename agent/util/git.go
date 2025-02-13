package util

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

// getGitVersion retrieves the current Git version installed on the system.
func getGitVersion() (string, bool, error) {
	cmd := exec.Command("git", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", false, errors.Wrap(err, "failed to get git version")
	}

	return thirdparty.ParseGitVersion(strings.TrimSpace(out.String()))
}

// IsGitVersionMinimumForScalar checks if the installed Git version is later than the specified version.
func IsGitVersionMinimumForScalar(minVersion string) (bool, error) {
	gitVersion, isApple, err := getGitVersion()
	if err != nil {
		return false, err
	}
	// scalar is not bundled with Apple's Git version 2.38.0
	if isApple {
		return false, nil
	}

	return !thirdparty.IsVersionMinimum(gitVersion, minVersion), nil
}
