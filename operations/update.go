package operations

import (
	"context"
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
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/kardianos/osext"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Update() cli.Command {
	const installFlagName = "install"
	const forceFlagName = "force"

	return cli.Command{
		Name:    "get-update",
		Aliases: []string{"update"},
		Usage:   "fetch the latest version of this binary",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  joinFlagNames(installFlagName, "i", "yes", "y"),
				Usage: "after downloading the update, install the updated binary",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(forceFlagName, "f"),
				Usage: "download a new CLI even if the current CLI is not out of date",
			},
		},
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			doInstall := c.Bool(installFlagName)
			forceUpdate := c.Bool(forceFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.GetRestCommunicator(ctx)

			update, err := checkUpdate(client, false, forceUpdate)
			if err != nil {
				return err
			}
			if !update.needsUpdate || update.binary == nil {
				return nil
			}

			grip.Infoln("Fetching update from", update.binary.URL)
			updatedBin, err := prepareUpdate(update.binary.URL, update.newVersion)
			if err != nil {
				return err
			}

			if doInstall {
				grip.Infoln("Upgraded binary successfully downloaded to temporary file:", updatedBin)

				var binaryDest string
				binaryDest, err = osext.Executable()
				if err != nil {
					return errors.Errorf("Failed to get installation path: %s", err)
				}

				grip.Infoln("Unlinking existing binary at", binaryDest)
				err = syscall.Unlink(binaryDest)
				if err != nil {
					return err
				}
				grip.Infoln("Moving upgraded binary to:", binaryDest)
				err = os.Rename(updatedBin, binaryDest)
				if err != nil {
					return err
				}

				grip.Info("Setting binary permissions...")
				err = os.Chmod(binaryDest, 0755)
				if err != nil {
					return err
				}
				grip.Info("Upgrade complete!")
				return nil
			}

			grip.Infoln("New binary downloaded (but not installed) to path:", updatedBin)

			// Attempt to generate a command that the user can copy/paste to complete the install.
			binaryDest, err := osext.Executable()
			if err != nil {
				// osext not working on this platform so we can't generate command, give up (but ignore err)
				return nil
			}
			installCommand := fmt.Sprintf("\tmv %s %s", updatedBin, binaryDest)
			if runtime.GOOS == "windows" {
				installCommand = fmt.Sprintf("\tmove %s %s", updatedBin, binaryDest)
			}
			grip.Infoln("To complete the install, run the following command:", installCommand)

			return nil
		},
	}
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
		return "", errors.Errorf("empty response from URL: %s", url)
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

	// Executables on windows must end in .exe
	if runtime.GOOS == "windows" {
		winTempPath := tempPath + ".exe"
		if err = os.Rename(tempPath, winTempPath); err != nil {
			return "", errors.Wrapf(err, "problem renaming file %s to %s", tempPath, winTempPath)
		}
		tempPath = winTempPath
	}

	fmt.Println("Upgraded binary downloaded to", tempPath, "- verifying")

	// Run the new binary's "version" command to verify that it is in fact the correct upgraded
	// version
	cmd := exec.Command(tempPath, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "Update failed - checking version of new binary returned error")
	}

	updatedVersion := string(out)
	updatedVersion = strings.TrimSpace(updatedVersion)

	if updatedVersion != newVersion {
		return "", errors.Errorf("Update failed - expected new binary to have version %s, but got %s instead", newVersion, updatedVersion)
	}

	return tempPath, nil
}

type updateStatus struct {
	binary      *evergreen.ClientBinary
	needsUpdate bool
	newVersion  string
}

func checkUpdate(client client.Communicator, silent bool, force bool) (updateStatus, error) {
	// This version of the cli has been built with a version, so we can compare it with what the
	// server says is the latest
	clients, err := client.GetClientConfig(context.Background())
	grip.NoticeWhen(!silent, message.WrapError(err, message.Fields{
		"message": "Failed checking for updates",
	}))
	if err != nil {
		return updateStatus{nil, false, ""}, err
	}

	// No update needed
	if !force && clients.LatestRevision == evergreen.ClientVersion {
		grip.NoticeWhen(!silent, message.Fields{
			"message":  "Binary is already up to date - not updating.",
			"revision": evergreen.ClientVersion,
		})
		return updateStatus{nil, false, clients.LatestRevision}, nil
	}

	binarySource := findClientUpdate(*clients)
	if binarySource == nil {
		// Client is out of date but no update available
		grip.NoticeWhen(!silent, message.WrapError(err, message.Fields{
			"message":  "Client is out of date but update is unavailable.",
			"revision": evergreen.ClientVersion,
		}))
		return updateStatus{nil, true, clients.LatestRevision}, nil
	}

	grip.NoticeWhen(!silent, message.WrapError(err, message.Fields{
		"message":      "Update to version found",
		"revision":     evergreen.ClientVersion,
		"new_revision": clients.LatestRevision,
	}))
	return updateStatus{binarySource, true, clients.LatestRevision}, nil
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
