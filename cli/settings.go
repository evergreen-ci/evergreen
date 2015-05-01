package cli

import (
	"bufio"
	"fmt"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
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
	reply := ""
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

// loadSettings attempts to load the settings file
func loadSettings(opts Options) (*Settings, error) {
	confPath := opts.ConfFile
	if confPath == "" {
		u, err := user.Current()
		if err != nil {
			return nil, err
		}
		confPath = filepath.Join(u.HomeDir, ".evergreen.yml")
	}
	f, err := os.Open(confPath)
	if err != nil {
		return nil, err
	}
	settings := &Settings{}
	err = util.ReadYAMLInto(f, settings)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

type Options struct {
	ConfFile string `short:"c" long:"config" description:"path to config file (defaults to ~/.evergreen.yml)"`
}

type ProjectConf struct {
	Name     string   `yaml:"name,omitempty"`
	Default  bool     `yaml:"default,omitempty"`
	Variants []string `yaml:"variants,omitempty"`
}

// Settings represents the data stored in the user's config file, by default
// located at ~/.evergreen.yml
type Settings struct {
	APIServerHost string        `yaml:"api_server_host,omitempty"`
	UIServerHost  string        `yaml:"ui_server_host,omitempty"`
	APIKey        string        `yaml:"api_key,omitempty"`
	User          string        `yaml:"user,omitempty"`
	Projects      []ProjectConf `yaml:"projects,omitempty"`
}

func (s *Settings) Write(opts Options) error {
	confPath := opts.ConfFile
	if confPath == "" {
		u, err := user.Current()
		if err != nil {
			return err
		}
		confPath = filepath.Join(u.HomeDir, ".evergreen.yml")
	}
	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(confPath, yamlData, 0644)
}

func (s *Settings) FindDefaultProject() string {
	for _, p := range s.Projects {
		if p.Default {
			return p.Name
		}
	}
	return ""
}

func (s *Settings) FindDefaultVariants(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Variants
		}
	}
	return nil
}

func (s *Settings) SetDefaultVariants(project string, variants ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Variants = variants
			return
		}
	}

	s.Projects = append(s.Projects, ProjectConf{project, true, variants})
}

func (s *Settings) SetDefaultProject(name string) {
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
		s.Projects = append(s.Projects, ProjectConf{name, true, []string{}})
	}
}
