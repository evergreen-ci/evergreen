package operations

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	modeLength   = 3
	maxModeValue = 0777
)

// hostDownload creates a command to download a file from a URL and extract it
// if desired.
func hostDownload() cli.Command {
	const (
		destDirFlagName  = "dest"
		extractFlagName  = "extract"
		fileNameFlagName = "file_name"
		modeFlagName     = "mode"
		urlFlagName      = "url"
	)
	return cli.Command{
		Name:  "download",
		Usage: "download resources from a URL on a host and optionally extract",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  urlFlagName,
				Usage: "URL to download",
			},
			cli.StringFlag{
				Name:  destDirFlagName,
				Usage: "destination directory for the resource",
			},
			cli.StringSliceFlag{
				Name:  fileNameFlagName,
				Usage: "the name of the output file in the destination directory (can be multiple if extracting)",
			},
			cli.StringFlag{
				Name:  extractFlagName,
				Usage: "extract the resource and put in the destination directory",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "permission bits for the files (if unset, no change)",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(urlFlagName),
			requireStringFlag(destDirFlagName),
			requireStringSliceFlag(fileNameFlagName),
		),
		Action: func(c *cli.Context) error {
			url := c.String(urlFlagName)
			destDir := c.String(destDirFlagName)
			fileNames := c.StringSlice(fileNameFlagName)
			extract := c.Bool(extractFlagName)
			mode, err := validateAndParseMode(c.String(modeFlagName))
			if err != nil {
				return errors.Wrap(err, "error parsing mode")
			}

			if !extract && len(fileNames) != 1 {
				return errors.New("must specify exactly one file name if not extracting")
			}

			filePaths := make([]string, 0, len(fileNames))
			for _, fileName := range fileNames {
				filePaths = append(filePaths, filepath.Join(destDir, fileName))
			}

			if err := os.MkdirAll(destDir, 0755); err != nil {
				return errors.Wrap(err, "error making destination directory")
			}

			if extract {
				if err := downloadAndExtract(url, destDir); err != nil {
					return errors.Wrap(err, "error occurred during download and extract")
				}
			} else {
				if err := download(url, filePaths[0]); err != nil {
					return errors.Wrap(err, "error occurred during file download")
				}
			}

			if mode != 0 {
				if err := setFileModes(filePaths, mode); err != nil {
					return errors.Wrap(err, "error occurred while changing file modes")
				}
			}

			return nil
		},
	}
}

// downloadAndExtract downloads the archive file from url as a temporary
// file, extracts the archive file to the directory destDir, and removes the
// temporary archive.
func downloadAndExtract(url, destDir string) error {
	tempFile, err := ioutil.TempFile("", "evergreen")
	if err != nil {
		return errors.Wrap(err, "error creating temporary archive file")
	}
	defer os.Remove(tempFile.Name())

	info := jasper.DownloadInfo{URL: url}
	info.Path = tempFile.Name()
	info.ArchiveOpts.ShouldExtract = true
	info.ArchiveOpts.Format = jasper.ArchiveAuto
	info.ArchiveOpts.TargetPath = destDir

	return errors.Wrapf(info.Download(), "error downloading archive file '%s' to path '%s'", url, destDir)
}

// download downloads the file from url to the given filePath.
func download(url, filePath string) error {
	info := jasper.DownloadInfo{
		URL:  url,
		Path: filePath,
	}

	return errors.Wrapf(info.Download(), "error downloading file '%s' to file '%s'", url, filePath)
}

// validateAndParseMode checks that the mode string is the correct length and
// within the allowed range of file permission mode bits (000-777).
func validateAndParseMode(modeStr string) (uint64, error) {
	if len(modeStr) == 0 {
		return 0, nil
	}
	if len(modeStr) != modeLength {
		return 0, errors.New("mode string must be exactly three digits")
	}

	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return 0, errors.Wrap(err, "error parsing octal mode bits")
	}
	if mode > maxModeValue {
		return 0, errors.Errorf("mode bits must be between %03o and %03o", 0, maxModeValue)
	}
	return mode, nil
}

// setFileModes changes the mode on the files in filePaths to the given mode.
func setFileModes(filePaths []string, mode uint64) error {
	catcher := grip.NewBasicCatcher()
	for _, filePath := range filePaths {
		if err := os.Chmod(filePath, os.FileMode(mode)); err != nil {
			catcher.Add(errors.Wrapf(err, "error changing mode bits on file '%s'", filePath))
		}
	}
	return catcher.Resolve()
}
