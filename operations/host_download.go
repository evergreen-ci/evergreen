package operations

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// hostDownload creates a command to download a file from a URL and extract it
// if desired.
func hostDownload() cli.Command {
	const (
		destFlagName     = "dest"
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
				Name:  destFlagName,
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
			requireStringFlag(destFlagName),
			requireStringSliceFlag(fileNameFlagName),
			requireIntValueBetween(modeFlagName, 0, 0777),
		),
		Action: func(c *cli.Context) error {
			url := c.String(urlFlagName)
			extract := c.Bool(extractFlagName)
			dest := c.String(destFlagName)
			mode := c.Int(modeFlagName)
			fileNames := c.StringSlice(fileNameFlagName)

			if !extract && len(fileNames) != 1 {
				return errors.New("if not extracting files, must specify exactly 1 file name")
			}

			filePaths := make([]string, 0, len(fileNames))
			for _, fileName := range fileNames {
				filePaths = append(filePaths, filepath.Join(dest, fileName))
			}

			info := jasper.DownloadInfo{URL: url}
			if extract {
				tempDir, err := ioutil.TempDir("", "evergreen")
				if err != nil {
					return errors.Wrap(err, "error creating temporary directory")
				}
				defer os.RemoveAll(tempDir)

				info.ArchiveOpts.ShouldExtract = true
				info.ArchiveOpts.Format = jasper.ArchiveAuto
				info.ArchiveOpts.TargetPath = dest
				info.Path = tempDir
			} else {
				info.Path = filePaths[0]
			}

			if err := info.Download(); err != nil {
				return errors.Wrap(err, "failed to download")
			}

			if mode != 0 {
				for _, filePath := range filePaths {
					if err := os.Chmod(filePath, os.FileMode(mode)); err != nil {
						return errors.Wrap(err, "error changing mode bits")
					}
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
