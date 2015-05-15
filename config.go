package evergreen

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

const (
	DefaultConfFile = "/etc/mci_settings.yml"
)

// AuthUser is a struct containing the Username, DisplayName, Password and Email.
type AuthUser struct {
	Username    string `yaml:"username"`
	DisplayName string `yaml:"display_name"`
	Password    string `yaml:"password"`
	Email       string `yaml:"email"`
}

// NaiveAuthConfig is a struct with a list of AuthUsers.
type NaiveAuthConfig struct {
	Users []*AuthUser
}

type CrowdConfig struct {
	Username string
	Password string
	Urlroot  string
}

// AuthConfig has a pointer to either a CrowConfig or a NaiveAuthConfig.
type AuthConfig struct {
	Crowd *CrowdConfig     `yaml:"crowd"`
	Naive *NaiveAuthConfig `yaml:"naive"`
}

type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int
	MaxRepoRevisionsToSearch   int
	LogFile                    string
}

type Project struct {
	Name        string
	Credentials string
}

type ApiConfig struct {
	LogFile         string
	HttpListenAddr  string
	HttpsListenAddr string
	HttpsKey        string
}

type UIConfig struct {
	Url            string
	HelpUrl        string
	HttpListenAddr string
	// Secret to encrypt session storage
	Secret  string
	LogFile string
	// Default project to assume when none specified, e.g. when using
	// the /waterfall route use this project, while /waterfall/other-project
	// then use `other-project`
	DefaultProject string
	// Cache results of template compilation, so you don't have to re-read files
	// on every request. Note that if this is true, changes to HTML templates
	// won't take effect until server restart.
	CacheTemplates bool
}

type MonitorConfig struct {
	LogFile string
}

type RunnerConfig struct {
	LogFile         string
	IntervalSeconds int64
}

type HostInitConfig struct {
	LogFile           string
	SSHTimeoutSeconds int64
}

type NotifyConfig struct {
	LogFile string
	SMTP    *SMTPConfig `yaml:"smtp"`
}

type SMTPConfig struct {
	Server     string   `yaml:"server"`
	Port       int      `yaml:"port"`
	UseSSL     bool     `yaml:"use_ssl"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	AdminEmail []string `yaml:"admin_email"`
}

type SchedulerConfig struct {
	LogFile     string
	MergeToggle int
}

type TaskRunnerConfig struct {
	LogFile string
}

type JiraConfig struct {
	Host     string
	Username string
	Password string
}

type CloudProviders struct {
	AWS          AWSConfig          `yaml:"aws"`
	DigitalOcean DigitalOceanConfig `yaml:"digitalocean"`
}

type AWSConfig struct {
	Secret string `yaml:"aws_secret"`
	Id     string `yaml:"aws_id"`
}

type DigitalOceanConfig struct {
	ClientId string `yaml:"client_id"`
	Key      string `yaml:"key"`
}

type PluginConfig map[string]map[string]interface{}

type Settings struct {
	DbUrl               string            `yaml:"dburl"`
	Db                  string            `yaml:"db"`
	ConfigDir           string            `yaml:"configdir"`
	Motu                string            `yaml:"motu"`
	AgentExecutablesDir string            `yaml:"agentexecutablesdir"`
	SuperUsers          []string          `yaml:"superusers"`
	Jira                JiraConfig        `yaml:"jira"`
	Providers           CloudProviders    `yaml:"providers"`
	Keys                map[string]string `yaml:"keys"`
	Credentials         map[string]string `yaml:"credentials"`
	AuthConfig          AuthConfig        `yaml:"auth"`
	RepoTracker         RepoTrackerConfig `yaml:"repotracker"`
	Monitor             MonitorConfig     `yaml:"monitor"`
	Api                 ApiConfig         `yaml:"api"`
	Ui                  UIConfig          `yaml:"ui"`
	HostInit            HostInitConfig    `yaml:"hostinit"`
	Notify              NotifyConfig      `yaml:"notify"`
	Runner              RunnerConfig      `yaml:"runner"`
	Scheduler           SchedulerConfig   `yaml:"scheduler"`
	TaskRunner          TaskRunnerConfig  `yaml:"taskrunner"`
	Expansions          map[string]string `yaml:"expansions"`
	Plugins             PluginConfig      `yaml:"plugins"`
	IsProd              bool              `yaml:"isprod"`
}

func NewSettings(filename string) (*Settings, error) {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	settings := &Settings{}
	err = yaml.Unmarshal(configData, settings)
	if err != nil {
		return nil, err
	}

	return settings, nil
}

//Validate() checks the settings and returns nil if the config is valid,
//or an error with a message explaining why otherwise.
func (settings *Settings) Validate(validators []ConfigValidator) error {
	for _, validator := range validators {
		err := validator(settings)
		if err != nil {
			return err
		}
	}

	return nil
}

func MustConfigFile(settingsPath string) *Settings {
	settings, err := NewSettings(settingsPath)
	if err != nil {
		panic(err)
	}
	err = settings.Validate(ConfigValidationRules)
	if err != nil {
		panic(err)
	}
	return settings
}

func MustConfig() *Settings {
	var configFilePath = flag.String("conf", DefaultConfFile, "path to config file")
	flag.Parse()
	return MustConfigFile(*configFilePath)
}

type ConfigValidator func(settings *Settings) error

var ConfigValidationRules = []ConfigValidator{
	func(settings *Settings) error {
		if settings.DbUrl == "" || settings.Db == "" {
			return fmt.Errorf("DBUrl and DB must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Providers.AWS.Secret == "" || settings.Providers.AWS.Id == "" {
			return fmt.Errorf("AWS Secret and ID must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Motu == "" {
			return fmt.Errorf("API hostname must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Secret == "" {
			return fmt.Errorf("UI Secret must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ConfigDir == "" {
			return fmt.Errorf("Config directory must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.DefaultProject == "" {
			return fmt.Errorf("You must specify a default project in UI")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Url == "" {
			return fmt.Errorf("You must specify a default UI url")
		}
		return nil
	},

	func(settings *Settings) error {
		notifyConfig := settings.Notify.SMTP

		if notifyConfig == nil {
			return nil
		}

		if notifyConfig.Server == "" || notifyConfig.Port == 0 {
			return fmt.Errorf("You must specify a SMTP server and port")
		}

		if len(notifyConfig.AdminEmail) == 0 {
			return fmt.Errorf("You must specify at least one admin_email")
		}

		if notifyConfig.From == "" {
			return fmt.Errorf("You must specify a from address")
		}

		return nil
	},
	func(settings *Settings) error {
		if settings.AuthConfig.Crowd == nil && settings.AuthConfig.Naive == nil {
			return fmt.Errorf("You must specify one form of authentication")
		}
		if settings.AuthConfig.Naive != nil {

			used := map[string]bool{}
			for _, x := range settings.AuthConfig.Naive.Users {
				if used[x.Username] {
					return fmt.Errorf("Duplicate user in list")
				}
				used[x.Username] = true
			}
		}
		return nil
	},
}
