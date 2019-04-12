package operations

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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
				Usage: "destination directory for the resource (if extract is set, the directory for the extracted resource)",
			},
			cli.StringSliceFlag{
				Name:  fileNameFlagName,
				Usage: "the name of the file(s) to be written",
			},
			cli.StringFlag{
				Name:  extractFlagName,
				Usage: "extract the resource and put in the destination",
			},
			cli.IntFlag{
				Name:  modeFlagName,
				Usage: "mode bits for the files (if 0, no change)",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(urlFlagName),
			requireStringFlag(destDirFlagName),
			requireStringSliceFlag(fileNameFlagName),
			requireIntValueBetween(modeFlagName, 0, 0777),
		),
		Action: func(c *cli.Context) error {
			url := c.String(urlFlagName)
			extract := c.Bool(extractFlagName)
			destDir := c.String(destDirFlagName)
			mode := c.Int(modeFlagName)
			fileNames := c.StringSlice(fileNameFlagName)

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
				if err := chmodFiles(fileNames, mode); err != nil {
					return errors.Wrap(err, "error occurred while changing file modes")
				}
			}

			// download the resource
			// req, err := http.NewRequest(http.MethodGet, url, nil)
			// if err != nil {
			//     return errors.Wrap(err, "error building request")
			// }
			//
			// resp, err := http.DefaultClient.Do(req)
			// if err != nil {
			//     return errors.Wrap(err, "error making request")
			// }
			// defer resp.Body.Close()
			//
			// if err := os.MkdirAll(dest, 0777); err != nil {
			//     return errors.Wrap(err, "error making directory structure")
			// }
			//
			// if extract {
			//     // Extract from temporary file to destination and remove the
			//     // temporary.
			//     tmpFile, err := ioutil.TempFile("", "tmp")
			//     if err != nil {
			//         return errors.Wrap(err, "error creating temporary file")
			//     }
			//     defer os.Remove(tmpFile.Name())
			//
			//     if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			//         return errors.Wrap(err, "error writing to temporary file")
			//     }
			//
			//     if err := archiver.Unarchive(tmpFile.Name(), dest); err != nil {
			//         return errors.Wrap(err, "error extracting archive")
			//     }
			// } else {
			//     file, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0666)
			//     if err != nil {
			//         return errors.Wrap(err, "error opening file to write")
			//     }
			// }

			return nil
		},
	}
}

func downloadAndExtract(url, filePath string) error {
	tempFile, err := ioutil.TempFile("", "evergreen")
	if err != nil {
		return errors.Wrap(err, "error creating temporary archive file")
	}
	defer os.Remove(tempFile.Name())

	info := jasper.DownloadInfo{URL: url}
	info.Path = tempFile.Name()
	info.ArchiveOpts.ShouldExtract = true
	info.ArchiveOpts.Format = jasper.ArchiveAuto
	info.ArchiveOpts.TargetPath = filePath

	return errors.Wrapf(info.Download(), "error downloading archive file '%s' to path '%s'", url, filePath)
}

func download(url, filePath string) error {
	info := jasper.DownloadInfo{
		URL:  url,
		Path: filePath,
	}

	return errors.Wrapf(info.Download(), "error downloading file '%s' to file '%s'", url, filePath)
}

func chmodFiles(filePaths []string, mode int) error {
	catcher := grip.NewBasicCatcher()
	for _, filePath := range filePaths {
		if err := os.Chmod(filePath, os.FileMode(mode)); err != nil {
			catcher.Add(errors.Wrapf(err, "error changing mode bits on file '%s'", filePath))
		}
	}
	return catcher.Resolve()
}
