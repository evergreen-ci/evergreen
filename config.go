package mci

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v1"
	"io/ioutil"
)

const (
	DefaultConfFile = "/etc/mci_settings.yml"
)

type CrowdConfig struct {
	Username string
	Password string
	Urlroot  string
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

type CleanupConfig struct {
	LogFile string
}

type MonitorConfig struct {
	LogFile string
}

type HostInitConfig struct {
	LogFile string
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

type MCISettings struct {
	DbUrl               string
	Db                  string
	ConfigDir           string
	Motu                string
	AgentExecutablesDir string
	Jira                JiraConfig
	Providers           CloudProviders
	Keys                map[string]string
	Credentials         map[string]string
	Crowd               CrowdConfig
	RepoTracker         RepoTrackerConfig
	Cleanup             CleanupConfig
	Monitor             MonitorConfig
	Api                 ApiConfig
	Ui                  UIConfig
	HostInit            HostInitConfig
	Notify              NotifyConfig
	Scheduler           SchedulerConfig
	TaskRunner          TaskRunnerConfig
	Expansions          map[string]string
	Plugins             PluginConfig
	IsProd              bool
}

func NewMCISettings(filename string) (*MCISettings, error) {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	mciSettings := &MCISettings{}
	err = yaml.Unmarshal(configData, mciSettings)
	if err != nil {
		return nil, err
	}

	return mciSettings, nil
}

//Validate() checks the settings and returns nil if the config is valid,
//or an error with a message explaining why otherwise.
func (mciSettings *MCISettings) Validate(validators []ConfigValidator) error {
	for _, validator := range validators {
		err := validator(mciSettings)
		if err != nil {
			return err
		}
	}

	return nil
}

func MustConfigFile(settingsPath string) *MCISettings {
	settings, err := NewMCISettings(settingsPath)
	if err != nil {
		panic(err)
	}
	err = settings.Validate(ConfigValidationRules)
	if err != nil {
		panic(err)
	}
	return settings
}

func MustConfig() *MCISettings {
	var configFilePath = flag.String("conf", DefaultConfFile, "path to config file")
	flag.Parse()
	return MustConfigFile(*configFilePath)
}

type ConfigValidator func(settings *MCISettings) error

var ConfigValidationRules = []ConfigValidator{
	func(settings *MCISettings) error {
		if settings.DbUrl == "" || settings.Db == "" {
			return fmt.Errorf("DBUrl and DB must not be empty")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.Providers.AWS.Secret == "" || settings.Providers.AWS.Id == "" {
			return fmt.Errorf("AWS Secret and ID must not be empty")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.Motu == "" {
			return fmt.Errorf("MOTU hostname must not be empty")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.Ui.Secret == "" {
			return fmt.Errorf("UI Secret must not be empty")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.ConfigDir == "" {
			return fmt.Errorf("Config directory must not be empty")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.Ui.DefaultProject == "" {
			return fmt.Errorf("You must specify a default project in UI")
		}
		return nil
	},

	func(settings *MCISettings) error {
		if settings.Ui.Url == "" {
			return fmt.Errorf("You must specify a default UI url")
		}
		return nil
	},

	func(settings *MCISettings) error {
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
}
