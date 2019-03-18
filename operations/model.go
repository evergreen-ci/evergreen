package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/kardianos/osext"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const localConfigPath = ".evergreen.local.yml"

type ClientProjectConf struct {
	Name     string   `json:"name" yaml:"name,omitempty"`
	Default  bool     `json:"default" yaml:"default,omitempty"`
	Alias    string   `json:"alias" yaml:"alias,omitempty"`
	Variants []string `json:"variants" yaml:"variants,omitempty"`
	Tasks    []string `json:"tasks" yaml:"tasks,omitempty"`
}

func findConfigFilePath(fn string) (string, error) {
	currentBinPath, _ := osext.Executable()

	userHome, err := homedir.Dir()
	if err != nil {
		// workaround for cygwin if we're on windows but couldn't get a homedir
		if runtime.GOOS == "windows" && len(os.Getenv("HOME")) > 0 {
			userHome = os.Getenv("HOME")
		}
	}

	absfn, _ := filepath.Abs(fn)

	files := []string{
		fn,
		absfn,
		filepath.Join(userHome, ".evergreen.yml"),
		filepath.Join(filepath.Dir(currentBinPath), ".evergreen.yml"),
	}

	for _, path := range files {
		stat, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}

		if stat.IsDir() {
			continue
		}

		return path, nil
	}

	return "", errors.New("could not evergreen cli client configuration on the local system")
}

// Client represents the data stored in the user's config file, by default
// located at ~/.evergreen.yml
// If you change the JSON tags, you must also change an anonymous struct in hostinit/setup.go
type ClientSettings struct {
	APIServerHost string              `json:"api_server_host" yaml:"api_server_host,omitempty"`
	UIServerHost  string              `json:"ui_server_host" yaml:"ui_server_host,omitempty"`
	APIKey        string              `json:"api_key" yaml:"api_key,omitempty"`
	User          string              `json:"user" yaml:"user,omitempty"`
	Projects      []ClientProjectConf `json:"projects" yaml:"projects,omitempty"`
	Admin         ClientAdminConf     `json:"admin" yaml:"admin,omitempty"`
	LoadedFrom    string              `json:"-" yaml:"-"`
}

func NewClientSettings(fn string) (*ClientSettings, error) {
	path, err := findConfigFilePath(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "could find file %s", fn)
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "problem reading configuration from file")
	}

	conf := &ClientSettings{}
	if err = yaml.Unmarshal(data, conf); err != nil {
		return nil, errors.Wrap(err, "problem reading yaml data from configuration file")
	}
	conf.LoadedFrom = path

	localData, err := ioutil.ReadFile(localConfigPath)
	if os.IsNotExist(err) {
		return conf, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "problem reading local configuration from file")
	}

	// Unmarshalling into the same struct will only override fields which are set
	// in the new YAML
	if err = yaml.Unmarshal(localData, conf); err != nil {
		return nil, errors.Wrap(err, "problem reading yaml data from local configuration file")
	}

	return conf, nil
}

func (s *ClientSettings) Write(fn string) error {
	if fn == "" {
		if s.LoadedFrom != "" {
			fn = s.LoadedFrom
		}
	}
	if fn == "" {
		return errors.New("no output location specified")
	}

	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "could not marshal data")
	}

	return errors.Wrap(ioutil.WriteFile(fn, yamlData, 0644), "could not write file")
}

func (s *ClientSettings) GetRestCommunicator(ctx context.Context) client.Communicator {
	c := client.NewCommunicator(s.APIServerHost)

	c.SetAPIUser(s.User)
	c.SetAPIKey(s.APIKey)

	banner, err := c.GetBannerMessage(ctx)
	if err != nil {
		grip.Debug(err)

	} else if len(banner) > 0 {
		grip.Noticef("Banner: %s", banner)
	}

	update, err := checkUpdate(c, true, false)
	if err != nil {
		grip.Debug(err)
	}
	if update.needsUpdate {
		if runtime.GOOS == "windows" {
			fmt.Fprintf(os.Stderr, "A new version is available. Run '%s get-update' to fetch it.\n", os.Args[0])
		} else {
			fmt.Fprintf(os.Stderr, "A new version is available. Run '%s get-update --install' to download and install it.\n", os.Args[0])
		}
	}

	return c
}

func (s *ClientSettings) getLegacyClients() (*legacyClient, *legacyClient, error) {
	// create client for the REST APIs
	apiURL, err := url.Parse(s.APIServerHost)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Settings file contains an invalid URL")
	}

	ac := &legacyClient{
		APIRoot:   s.APIServerHost,
		APIRootV2: s.APIServerHost + "/rest/v2",
		User:      s.User,
		APIKey:    s.APIKey,
		UIRoot:    s.UIServerHost,
	}

	rc := &legacyClient{
		APIRoot:   apiURL.Scheme + "://" + apiURL.Host + "/rest/v1",
		APIRootV2: apiURL.Scheme + "://" + apiURL.Host + "/rest/v2",
		User:      s.User,
		APIKey:    s.APIKey,
		UIRoot:    s.UIServerHost,
	}

	return ac, rc, nil
}

func (s *ClientSettings) FindDefaultProject() string {
	for _, p := range s.Projects {
		if p.Default {
			return p.Name
		}
	}
	return ""
}

func (s *ClientSettings) FindDefaultVariants(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Variants
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultVariants(project string, variants ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Variants = variants
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    "",
		Variants: variants,
		Tasks:    nil,
	})
}

func (s *ClientSettings) FindDefaultTasks(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Tasks
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultTasks(project string, tasks ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Tasks = tasks
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    "",
		Variants: nil,
		Tasks:    tasks,
	})
}

func (s *ClientSettings) FindDefaultAlias(project string) string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Alias
		}
	}
	return ""
}

func (s *ClientSettings) SetDefaultAlias(project string, alias string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Alias = alias
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    alias,
		Variants: nil,
		Tasks:    nil,
	})
}

func (s *ClientSettings) SetDefaultProject(name string) {
	var foundDefault bool
	for i, p := range s.Projects {
		if p.Name == name {
			s.Projects[i].Default = true
			foundDefault = true
		} else {
			s.Projects[i].Default = false
		}
	}

	if !foundDefault {
		s.Projects = append(s.Projects, ClientProjectConf{
			Name:     name,
			Default:  true,
			Alias:    "",
			Variants: []string{},
			Tasks:    []string{},
		})
	}
}
