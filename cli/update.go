package cli

import (
	//"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/evergreen-ci/evergreen"
	"github.com/kardianos/osext"
	"github.com/pkg/errors"
)

// GetUpdateCommand attempts to fetch the latest version of the client binary and install it over
// the current one.
type GetUpdateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Install    bool     `long:"install" description:"after downloading update, overwrite old binary with new one"`
}

// VersionCommand prints the revision that the CLI binary was built with.
// Is used by auto-update to verify the new version.
type VersionCommand struct{}

func (vc *VersionCommand) Execute(_ []string) error {
	fmt.Println(evergreen.ClientVersion)
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

// prepareUpdate fetches the update at the given URL, writes it to a temporary file, and returns
// the path to the temporary file.
func prepareUpdate(url, newVersion string) (string, error) {
	tempFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}

	response, err := http.Get(url)
	if err != nil {
		return "", err
	}

	if response == nil {
		return "", errors.Errorf("empty response from URL: %v", url)
	}

	defer response.Body.Close()
	_, err = io.Copy(tempFile, response.Body)
	if err != nil {
		return "", err
	}
	err = tempFile.Close()
	if err != nil {
		return "", err
	}

	tempPath, err := filepath.Abs(tempFile.Name())
	if err != nil {
		return "", err
	}

	//chmod the binary so that it is executable
	err = os.Chmod(tempPath, 0755)
	if err != nil {
		return "", err
	}

	fmt.Println("Upgraded binary downloaded to", tempPath, "- verifying")

	// Run the new binary's "version" command to verify that it is in fact the correct upgraded
	// version
	cmd := exec.Command(tempPath, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Errorf("Update failed - checking version of new binary returned error: %v", err)
	}

	updatedVersion := string(out)
	updatedVersion = strings.TrimSpace(updatedVersion)

	if updatedVersion != newVersion {
		return "", errors.Errorf("Update failed - expected new binary to have version %v, but got %v instead", newVersion, updatedVersion)
	}

	return tempPath, nil
}

type updateStatus struct {
	binary      *evergreen.ClientBinary
	needsUpdate bool
	newVersion  string
}

// checkUpdate checks if an update is available and logs its activity. If "silent" is true, logging
// is suppressed.
// Returns the info on the new binary to be downloaded (nil if none was found), a boolean
// indicating if the binary needs an update (version on client and server don't match), the new version if found,
// and an error if relevant.
func checkUpdate(ac *APIClient, silent bool) (updateStatus, error) {
	var outLog io.Writer = os.Stdout
	if silent {
		outLog = ioutil.Discard
	}

	// This version of the cli has been built with a version, so we can compare it with what the
	// server says is the latest
	clients, err := ac.CheckUpdates()
	if err != nil {
		fmt.Fprintf(outLog, "Failed checking for updates: %v\n", err)
		return updateStatus{nil, false, ""}, err
	}

	// No update needed
	if clients.LatestRevision == evergreen.ClientVersion {
		fmt.Fprintf(outLog, "Binary is already up to date at revision %v - not updating.\n", evergreen.ClientVersion)
		return updateStatus{nil, false, clients.LatestRevision}, nil
	}

	binarySource := findClientUpdate(*clients)
	if binarySource == nil {
		// Client is out of date but no update available
		fmt.Fprintf(outLog, "Client is out of date (version %v) but update is unavailable.\n", evergreen.ClientVersion)
		return updateStatus{nil, true, clients.LatestRevision}, nil
	}

	fmt.Fprintf(outLog, "Update to version %v found at %v\n", clients.LatestRevision, binarySource.URL)
	return updateStatus{binarySource, true, clients.LatestRevision}, nil
}

// Silently check if an update is available, and print a notification message if it is.
func notifyUserUpdate(ac *APIClient) {
	update, err := checkUpdate(ac, true)
	if update.needsUpdate && err == nil {
		if runtime.GOOS == "windows" {
			fmt.Printf("A new version is available. Run '%s get-update' to fetch it.\n", os.Args[0])
		} else {
			fmt.Printf("A new version is available. Run '%s get-update --install' to download and install it.\n", os.Args[0])
		}
	}
}

func (uc *GetUpdateCommand) Execute(_ []string) error {
	ac, _, _, err := getAPIClients(uc.GlobalOpts)
	if err != nil {
		return err
	}

	update, err := checkUpdate(ac, false)
	if err != nil {
		return err
	}
	if !update.needsUpdate || update.binary == nil {
		return nil
	}

	fmt.Println("Fetching update from", update.binary.URL)
	updatedBin, err := prepareUpdate(update.binary.URL, update.newVersion)
	if err != nil {
		return err
	}

	if uc.Install {
		fmt.Println("Upgraded binary successfully downloaded to temporary file:", updatedBin)

		binaryDest, err := osext.Executable()
		if err != nil {
			return errors.Errorf("Failed to get installation path: %v", err)
		}

		fmt.Println("Unlinking existing binary at", binaryDest)
		err = syscall.Unlink(binaryDest)
		if err != nil {
			return err
		}
		fmt.Println("Copying upgraded binary to: ", binaryDest)
		err = copyFile(binaryDest, updatedBin)
		if err != nil {
			return err
		}

		fmt.Println("Setting binary permissions...")
		err = os.Chmod(binaryDest, 0755)
		if err != nil {
			return err
		}
		fmt.Println("Upgrade complete!")
		return nil
	}
	fmt.Println("New binary downloaded (but not installed) to path: ", updatedBin)

	// Attempt to generate a command that the user can copy/paste to complete the install.
	binaryDest, err := osext.Executable()
	if err != nil {
		// osext not working on this platform so we can't generate command, give up (but ignore err)
		return nil
	}
	installCommand := fmt.Sprintf("\tmv %v %v", updatedBin, binaryDest)
	if runtime.GOOS == "windows" {
		installCommand = fmt.Sprintf("\tmove %v %v", updatedBin, binaryDest)
	}
	fmt.Printf("\nTo complete the install, run the following command:\n\n")
	fmt.Println(installCommand)

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
		_ = d.Close()
		return err
	}
	return d.Close()
}
