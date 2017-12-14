package operations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"

	"github.com/kardianos/osext"
	"github.com/urfave/cli"
)

func Update() cli.Command {
	const installFlagName = "install"

	return cli.Command{
		Name:    "get-update",
		Aliases: "update",
		Usage:   "fetch the latest version of this binary",
		Flags: clientConfigFlags(
			cli.BoolFlag{
				Name:    installFlagName,
				Aliases: []string{"i", "yes", "y"},
				Usage:   "after downloading the update, install the updated binary",
			}),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			doInstall := c.Bool(installFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
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

			if doInstall {
				fmt.Println("Upgraded binary successfully downloaded to temporary file:", updatedBin)

				var binaryDest string
				binaryDest, err = osext.Executable()
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
		},
	}
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
