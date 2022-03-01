package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/kardianos/osext"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
)

const localConfigPath = ".evergreen.local.yml"

type ClientProjectConf struct {
	Name           string            `json:"name" yaml:"name,omitempty"`
	Default        bool              `json:"default" yaml:"default,omitempty"`
	Alias          string            `json:"alias" yaml:"alias,omitempty"`
	Variants       []string          `json:"variants" yaml:"variants,omitempty"`
	Tasks          []string          `json:"tasks" yaml:"tasks,omitempty"`
	Parameters     map[string]string `json:"parameters" yaml:"parameters,omitempty"`
	TriggerAliases []string          `json:"trigger_aliases" yaml:"trigger_aliases"`
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

	if fn != "" {
		if isValidPath(fn) {
			return fn, nil
		}
		absfn, _ := filepath.Abs(fn)
		if isValidPath(absfn) {
			return absfn, nil
		}
	}
	defaultFiles := []string{
		filepath.Join(userHome, evergreen.DefaultEvergreenConfig),
		filepath.Join(filepath.Dir(currentBinPath), evergreen.DefaultEvergreenConfig),
	}
	for _, path := range defaultFiles {
		if isValidPath(path) {
			grip.WarningWhen(fn != "", "Couldn't find configuration file, falling back on default.")
			return path, nil
		}
	}

	return "", errors.New("could not find client configuration file on the local system")
}

func isValidPath(path string) bool {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) || stat.IsDir() {
		return false
	}
	return true
}

// Client represents the data stored in the user's config file, by default
// located at ~/.evergreen.yml
// If you change the JSON tags, you must also change an anonymous struct in hostinit/setup.go
type ClientSettings struct {
	APIServerHost         string              `json:"api_server_host" yaml:"api_server_host,omitempty"`
	UIServerHost          string              `json:"ui_server_host" yaml:"ui_server_host,omitempty"`
	APIKey                string              `json:"api_key" yaml:"api_key,omitempty"`
	User                  string              `json:"user" yaml:"user,omitempty"`
	UncommittedChanges    bool                `json:"patch_uncommitted_changes" yaml:"patch_uncommitted_changes,omitempty"`
	PreserveCommits       bool                `json:"preserve_commits" yaml:"preserve_commits,omitempty"`
	Projects              []ClientProjectConf `json:"projects" yaml:"projects,omitempty"`
	Admin                 ClientAdminConf     `json:"admin" yaml:"admin,omitempty"`
	LoadedFrom            string              `json:"-" yaml:"-"`
	DisableAutoDefaulting bool                `json:"disable_auto_defaulting" yaml:"disable_auto_defaulting"`
	ProjectsForDirectory  map[string]string   `json:"projects_for_directory,omitempty" yaml:"projects_for_directory,omitempty"`
}

func NewClientSettings(fn string) (*ClientSettings, error) {
	path, err := findConfigFilePath(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "could not find file %s", fn)
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

// setupRestCommunicator returns the rest communicator and prints any available info messages.
// Callers are responsible for calling (Communicator).Close() when finished with the client.
//
// To avoid printing these messages, call getRestCommunicator instead.
func (s *ClientSettings) setupRestCommunicator(ctx context.Context) client.Communicator {
	c := s.getRestCommunicator(ctx)
	printUserMessages(ctx, c)
	return c
}

// getRestCommunicator returns a client for communicating with the API server.
// Callers are responsible for calling (Communicator).Close() when finished with the client.
//
// Most callers should use setupRestCommunicator instead, which prints available info messages.
func (s *ClientSettings) getRestCommunicator(ctx context.Context) client.Communicator {
	c := client.NewCommunicator(s.APIServerHost)

	c.SetAPIUser(s.User)
	c.SetAPIKey(s.APIKey)

	return c
}

// printUserMessages prints any available info messages.
func printUserMessages(ctx context.Context, c client.Communicator) {
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

func (s *ClientSettings) FindDefaultProject(cwd string, useRoot bool) string {
	if project, exists := s.ProjectsForDirectory[cwd]; exists {
		return project
	}

	if useRoot {
		for _, p := range s.Projects {
			if p.Default {
				return p.Name
			}
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

func (s *ClientSettings) FindDefaultTriggerAliases(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.TriggerAliases
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultTriggerAliases(project string, triggerAliases []string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].TriggerAliases = triggerAliases
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{
		Name:           project,
		Default:        true,
		TriggerAliases: triggerAliases,
	})
}

func (s *ClientSettings) FindDefaultParameters(project string) []patch.Parameter {
	for _, p := range s.Projects {
		if p.Name == project {
			return parametersFromMap(p.Parameters)
		}
	}
	return nil
}

func parametersFromMap(params map[string]string) []patch.Parameter {
	res := []patch.Parameter{}
	for key, val := range params {
		res = append(res, patch.Parameter{Key: key, Value: val})
	}
	return res
}

func parametersToMap(params []patch.Parameter) map[string]string {
	res := map[string]string{}
	for _, param := range params {
		res[param.Key] = param.Value
	}
	return res
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

func (s *ClientSettings) SetDefaultProject(cwd, project string) {
	if s.DisableAutoDefaulting {
		return
	}

	_, found := s.ProjectsForDirectory[cwd]
	if found {
		return
	}
	if s.ProjectsForDirectory == nil {
		s.ProjectsForDirectory = map[string]string{}
	}
	s.ProjectsForDirectory[cwd] = project
	grip.Infof("Project '%s' will be set as the one to use for directory '%s'. To disable automatic defaulting, set 'disable_auto_defaulting' to true.", project, cwd)
}
