package operations

import (
	"context"
	"fmt"
	"io"
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
	const autoUpgradeFlagName = "auto"

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
			cli.BoolFlag{
				Name:  autoUpgradeFlagName,
				Usage: "setup automatic installations of a new CLI if the current CLI is out of date",
			},
		},
		Before: setPlainLogger,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(ConfFlagName)
			install := c.Bool(installFlagName)
			forceUpdate := c.Bool(forceFlagName)
			autoUpgrade := c.Bool(autoUpgradeFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			if !conf.AutoUpgradeCLI && !autoUpgrade {
				fmt.Printf("Automatic CLI upgrades are not set up; specifying the --%s flag will enable automatic CLI upgrades before each command if the current CLI is out of date.\n", autoUpgradeFlagName)
			}
			if conf.AutoUpgradeCLI && autoUpgrade {
				fmt.Printf("Automatic CLI upgrades are already set up, specifying the --%s flag is not necessary.\n", autoUpgradeFlagName)
			}
			if !conf.AutoUpgradeCLI && autoUpgrade {
				conf.SetAutoUpgradeCLI()
				if err := conf.Write(""); err != nil {
					return errors.Wrap(err, "setting auto-upgrade CLI option")
				}
				fmt.Println("Automatic CLI upgrades have successfully been setup.")
			}
			doInstall := install || autoUpgrade || conf.AutoUpgradeCLI
			return checkAndUpdateVersion(conf, ctx, doInstall, forceUpdate, false)
		},
	}
}

// checkAndUpdateVersion checks if the CLI is up to date. If there is no new CLI version it will simply notify that the current binary is up to date.
// If there is a new version available, it will be downloaded, and if doInstall is set the new binary will automatically be replaced at the current path the old binary existed in.
// Otherwise, it will simply be downloaded and a suggested 'mv' command will be printed so the user can replace the binary at their discretion.
// Toggling forceUpdate will download a new CLI even if the current CLI does not have an out-of-date CLI version string.
func checkAndUpdateVersion(conf *ClientSettings, ctx context.Context, doInstall bool, forceUpdate bool, silent bool) error {
	client, err := conf.SetupRestCommunicator(ctx, false, skipCheckingMinimumCLIVersion())
	if err != nil {
		return errors.Wrap(err, "getting REST communicator")
	}
	defer client.Close()

	update, err := checkUpdate(client, silent, forceUpdate)
	if err != nil {
		return err
	}
	if !update.needsUpdate || len(update.binaries) == 0 {
		return nil
	}

	updatedBin, err := tryAllPrepareUpdate(update)
	if err != nil {
		return err
	}
	defer func() {
		if doInstall {
			grip.Error(os.Remove(updatedBin))
		}
	}()

	if doInstall {
		grip.Infoln("Upgraded binary successfully downloaded to temporary file:", updatedBin)

		var binaryDest string
		binaryDest, err = osext.Executable()
		if err != nil {
			return errors.Wrap(err, "getting installation path")
		}

		winTempFileBase := strings.TrimSuffix(filepath.Base(binaryDest), ".exe")
		winTempDest := filepath.Join(filepath.Dir(binaryDest), winTempFileBase+"-old.exe")
		if runtime.GOOS == "windows" {
			grip.Infoln("Moving existing binary", binaryDest, "to", winTempDest)
			if err = os.Rename(binaryDest, winTempDest); err != nil {
				return err
			}
		} else {
			grip.Infoln("Unlinking existing binary at", binaryDest)
			if err = syscall.Unlink(binaryDest); err != nil {
				return err
			}
		}

		grip.Infoln("Moving upgraded binary to", binaryDest)
		err = copyFile(binaryDest, updatedBin)
		if err != nil {
			return err
		}

		grip.Info("Setting binary permissions...")
		err = os.Chmod(binaryDest, 0755)
		if err != nil {
			return err
		}
		grip.Info("Upgrade complete!")

		if runtime.GOOS == "windows" {
			grip.Infoln("Deleting old binary", winTempDest)
			// Source: https://stackoverflow.com/a/19748576
			// Since Windows does not allow a binary that's currently in
			// use to be deleted, wait 2 seconds for the CLI process to
			// exit and then delete it.
			cmd := exec.Command("cmd", "/c", fmt.Sprintf("ping localhost -n 3 > nul & del %s", winTempDest))
			if err = cmd.Start(); err != nil {
				return err
			}
		}

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
}

func copyFile(dst, src string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer func() {
		grip.Error(s.Close())
	}()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		grip.Error(d.Close())
		return err
	}
	return d.Close()
}

// tryAllPrepareUpdate tries to prepare the binary update with any of the
// possible client downloads. It returns the first one that succeeds, or an
// error if none succeed.
func tryAllPrepareUpdate(update updateStatus) (string, error) {
	var err error
	for _, binary := range update.binaries {
		grip.Infoln("Fetching update from", binary.URL)
		var updatedBin string
		updatedBin, err = prepareUpdate(binary.URL, update.newVersion)
		if err != nil {
			grip.Error(err)
			continue
		}
		return updatedBin, nil
	}
	return "", err
}

// prepareUpdate fetches the update at the given URL, writes it to a temporary file, and returns
// the path to the temporary file.
func prepareUpdate(url, newVersion string) (string, error) {
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}

	response, err := http.Get(url)
	if err != nil {
		grip.Error(tempFile.Close())
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		grip.Error(tempFile.Close())
		return "", errors.Errorf("received status code '%s'", response.Status)
	}

	defer response.Body.Close()
	_, err = io.Copy(tempFile, response.Body)
	if err != nil {
		grip.Error(tempFile.Close())
		return "", err
	}

	if err = tempFile.Close(); err != nil {
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
			return "", errors.Wrapf(err, "renaming file '%s' to '%s'", tempPath, winTempPath)
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
		return "", errors.Errorf("Update failed - expected new binary to have version '%s', but got '%s' instead", newVersion, updatedVersion)
	}

	return tempPath, nil
}

type updateStatus struct {
	binaries    []evergreen.ClientBinary
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
		return updateStatus{
			binaries:    nil,
			needsUpdate: false,
			newVersion:  "",
		}, err
	}

	// No update needed
	if !force && clients.LatestRevision == evergreen.ClientVersion {
		grip.NoticeWhen(!silent, message.Fields{
			"message":  "Binary is already up to date - not updating.",
			"revision": evergreen.ClientVersion,
		})
		return updateStatus{
			binaries:    nil,
			needsUpdate: false,
			newVersion:  clients.LatestRevision,
		}, nil
	}

	binarySources := findClientUpdates(*clients)
	if len(binarySources) == 0 {
		// Client is out of date but no update available
		grip.NoticeWhen(!silent, message.WrapError(err, message.Fields{
			"message":  "Client is out of date but update is unavailable.",
			"revision": evergreen.ClientVersion,
		}))
		return updateStatus{
			binaries:    nil,
			needsUpdate: true,
			newVersion:  clients.LatestRevision,
		}, nil
	}

	grip.NoticeWhen(!silent, message.WrapError(err, message.Fields{
		"message":      "Update to version found",
		"revision":     evergreen.ClientVersion,
		"new_revision": clients.LatestRevision,
	}))
	return updateStatus{
		binaries:    binarySources,
		needsUpdate: true,
		newVersion:  clients.LatestRevision,
	}, nil
}

// findClientUpdates searches a ClientConfig for all ClientBinaries with
// non-empty URLs, whose architecture and OS match that of the current system.
func findClientUpdates(clients evergreen.ClientConfig) []evergreen.ClientBinary {
	var binaries []evergreen.ClientBinary
	for _, c := range clients.ClientBinaries {
		if c.Arch == runtime.GOARCH && c.OS == runtime.GOOS && len(c.URL) > 0 {
			binaries = append(binaries, c)
		}
	}
	return binaries
}
