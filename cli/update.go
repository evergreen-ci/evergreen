package cli

import (
	//"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/kardianos/osext"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// UpdateCommand attempts to fetch the latest version of the client binary and install it over
// the current one.
type UpdateCommand struct {
	GlobalOpts Options `no-flag:"true"`
	NoInstall  bool    `long:"no-overwrite" description:"only download updated binary, and don't overwrite the old one"`
}

// VersionCommand prints the revision that the CLI binary was built with.
// Is used by auto-update to verify the new version.
type VersionCommand struct{}

func (vc *VersionCommand) Execute(args []string) error {
	fmt.Println(evergreen.BuildRevision)
	return nil
}

// Searches a ClientConfig for a ClientBinary with a non-empty URL, whose architecture and OS
// match that of the current system.
func findClientUpdate(clients evergreen.ClientConfig) *evergreen.ClientBinary {
	for _, c := range clients.ClientBinaries {
		if c.Arch == runtime.GOARCH && c.OS == runtime.GOOS && len(c.URL) > 0 {
			return &c
		}
	}
	return nil
}

// prepareUpdate fetches the update at the given url, writes it to a temporary file, and returns
// the path to the temporary file.
func prepareUpdate(url, newVersion string) (string, error) {
	tempFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	response, err := http.Get(url)
	if err != nil {
		return "", err
	}

	if response == nil {
		return "", fmt.Errorf("empty response from url: %v", url)
	}

	defer response.Body.Close()
	_, err = io.Copy(tempFile, response.Body)
	if err != nil {
		return "", err
	}
	tempPath, err := filepath.Abs(tempFile.Name())
	if err != nil {
		return "", err
	}

	//chmod the binary so that it is executable
	os.Chmod(tempPath, 0755)

	fmt.Println("Upgraded binary downloaded to ", tempPath, " - verifying")

	// Run the new binary's "version" command to verify that it is in fact the correct upgraded
	// version
	cmd := exec.Command(tempPath, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Update failed - checking version of new binary returned error: %v", err)
	}

	updatedVersion := string(out)
	updatedVersion = strings.TrimSpace(updatedVersion)

	if updatedVersion != newVersion {
		return "", fmt.Errorf("Update failed - expected new binary to have version %v, but got %v instead", newVersion, updatedVersion)
	}

	return tempPath, nil
}

// checkUpdate checks if an update is available and logs its activity. If "silent" is true, logging
// is suppressed.
// Returns the info on the new binary to be downloaded (nil if none was found), a boolean
// indicating if the binary needs an update (version on client and server don't match), the new version if found,
// and an error if relevant.
func checkUpdate(ac *APIClient, silent bool) (*evergreen.ClientBinary, bool, string, error) {
	var outLog io.Writer = os.Stdout
	if silent {
		outLog = ioutil.Discard
	}

	// This version of the cli has been built with a version, so we can compare it with what the
	// server says is the latest
	clients, err := ac.CheckUpdates()
	if err != nil {
		fmt.Fprintf(outLog, "Failed checking for updates: %v\n", err)
		return nil, false, "", err
	}

	// No update needed
	if clients.LatestRevision == evergreen.BuildRevision {
		fmt.Fprintf(outLog, "Binary is already up to date at revision %v - not updating.\n", evergreen.BuildRevision)
		return nil, false, clients.LatestRevision, nil
	}

	binarySource := findClientUpdate(*clients)
	if binarySource == nil {
		// Client is out of date but no update available
		fmt.Fprintf(outLog, "Client is out of date (version %v) but update is unavailable.\n", evergreen.BuildRevision)
		return nil, true, clients.LatestRevision, nil
	}

	fmt.Fprintf(outLog, "Update to version %v found at %v\n", evergreen.BuildRevision, binarySource.URL)
	return binarySource, true, clients.LatestRevision, nil
}

// Silently check if an update is available, and print a notification message if it is.
func notifyUserUpdate(ac *APIClient) {
	_, needsUpdate, _, err := checkUpdate(ac, true)
	if needsUpdate && err == nil {
		fmt.Println("A new version is available. Run 'evergreen update' to get it.")
	}
}

func (uc *UpdateCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(uc.GlobalOpts)
	if err != nil {
		return err
	}

	binarySource, needsUpdate, newVersion, err := checkUpdate(ac, false)
	if err != nil {
		return err
	}
	if needsUpdate == false || binarySource == nil {
		return nil
	}

	fmt.Println("Fetching update from", binarySource.URL)
	updatedBin, err := prepareUpdate(binarySource.URL, newVersion)
	if err != nil {
		return err
	}

	if !uc.NoInstall {
		fmt.Println("Upgraded binary successfully downloaded to temporary file:", updatedBin)

		binaryDest, err := osext.Executable()
		if err != nil {
			return fmt.Errorf("Failed to get installation path: %v", err)
		}
		fmt.Println("Copying upgraded binary to:", binaryDest)
		err = copyFile(binaryDest, updatedBin)
		if err != nil {
			return err
		}
		fmt.Println("Upgrade complete!")
		return nil
	}
	fmt.Println("New binary downloaded (but not installed) to path: ", updatedBin)
	return nil
}

func copyFile(dst, src string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}
