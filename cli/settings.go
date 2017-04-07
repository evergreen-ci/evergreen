package cli

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/kardianos/osext"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// prompt writes a prompt to the user on stdout, reads a newline-terminated response from stdin,
// and returns the result as a string.
func prompt(message string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(message + " ")
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

// confirm asks the user a yes/no question and returns true/false if they reply with y/yes/n/no.
// if defaultYes is true, allows user to just hit enter without typing an explicit yes.
func confirm(message string, defaultYes bool) bool {
	var reply string

	yes := []string{"y", "yes"}
	no := []string{"n", "no"}

	if defaultYes {
		yes = append(yes, "")
	}

	for {
		reply = prompt(message)
		if util.SliceContains(yes, strings.ToLower(reply)) {
			return true
		}
		if util.SliceContains(no, strings.ToLower(reply)) {
			return false
		}
	}
}

// LoadSettings attempts to load the settings file
func LoadSettings(opts *Options) (*model.CLISettings, error) {
	confPath := opts.ConfFile
	if confPath == "" {
		userHome, err := homedir.Dir()
		if err != nil {
			// workaround for cygwin if we're on windows but couldn't get a homedir
			if runtime.GOOS == "windows" && len(os.Getenv("HOME")) > 0 {
				userHome = os.Getenv("HOME")
			} else {
				return nil, err
			}
		}
		confPath = filepath.Join(userHome, ".evergreen.yml")
	}
	var f io.ReadCloser
	var err, extErr, extOpenErr error
	f, err = os.Open(confPath)
	if err != nil {
		// if we can't find the yml file in the home directory,
		// try to find it in the same directory as where the binary is being run from.
		// If we fail to determine that location, just return the first (outer) error.
		var currentBinPath string
		currentBinPath, extErr = osext.Executable()
		if extErr != nil {
			return nil, err
		}
		f, extOpenErr = os.Open(filepath.Join(filepath.Dir(currentBinPath), ".evergreen.yml"))
		if extOpenErr != nil {
			return nil, err
		}
	}

	settings := &model.CLISettings{}
	err = util.ReadYAMLInto(f, settings)
	if err != nil {
		return nil, err
	}
	settings.LoadedFrom = confPath
	return settings, nil
}

type Options struct {
	ConfFile string `short:"c" long:"config" description:"path to config file (defaults to ~/.evergreen.yml)"`
}

func WriteSettings(s *model.CLISettings, opts *Options) error {
	confPath := opts.ConfFile
	if confPath == "" {
		if s.LoadedFrom != "" {
			confPath = s.LoadedFrom
		}
	}
	if confPath == "" {
		return errors.New("can't determine output location for settings file")
	}
	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(confPath, yamlData, 0644)
}
