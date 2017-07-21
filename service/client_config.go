package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// getClientConfig should be called once at startup and looks at the
// current environment and loads all currently available client
// binaries for use by the API server in presenting the settings page.
//
// If there are no built clients, this returns an empty config
// version, but does *not* error.
func getClientConfig(settings *evergreen.Settings) (*evergreen.ClientConfig, error) {
	c := &evergreen.ClientConfig{}
	c.LatestRevision = evergreen.ClientVersion

	root := filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory)

	if _, err := os.Stat(root); os.IsNotExist(err) {
		grip.Warningf("client directory '%s' does not exist, creating empty "+
			"directory and continuing with caution", root)
		grip.Error(os.MkdirAll(root, 0755))
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || info.Name() == "version" {
			return nil
		}

		parts := strings.Split(path, string(filepath.Separator))
		buildInfo := strings.Split(parts[len(parts)-2], "_")

		c.ClientBinaries = append(c.ClientBinaries, evergreen.ClientBinary{
			URL: fmt.Sprintf("%s/%s/%s", settings.Ui.Url, evergreen.ClientDirectory,
				strings.Join(parts[len(parts)-2:], "/")),
			OS:   buildInfo[0],
			Arch: buildInfo[1],
		})

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding client binaries")
	}

	return c, nil
}
