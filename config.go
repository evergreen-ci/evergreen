package evergreen

import (
	"io/ioutil"
	"time"

	legacyDB "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2017-09-14"
)

// AuthUser configures a user for our Naive authentication setup.
type AuthUser struct {
	Username    string `yaml:"username"`
	DisplayName string `yaml:"display_name"`
	Password    string `yaml:"password"`
	Email       string `yaml:"email"`
}

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []*AuthUser
}

// CrowdConfig holds settings for interacting with Atlassian Crowd.
type CrowdConfig struct {
	Username string
	Password string
	Urlroot  string
}

// GithubAuthConfig holds settings for interacting with Github Authentication including the
// ClientID, ClientSecret and CallbackUri which are given when registering the application
// Furthermore,
type GithubAuthConfig struct {
	ClientId     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Users        []string `yaml:"users"`
	Organization string   `yaml:"organization"`
}

// AuthConfig has a pointer to either a CrowConfig or a NaiveAuthConfig.
type AuthConfig struct {
	Crowd  *CrowdConfig      `yaml:"crowd"`
	Naive  *NaiveAuthConfig  `yaml:"naive"`
	Github *GithubAuthConfig `yaml:"github"`
}

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int
	MaxRepoRevisionsToSearch   int
	MaxConcurrentRequests      int
}

type ClientBinary struct {
	Arch string `yaml:"arch" json:"arch"`
	OS   string `yaml:"os" json:"os"`
	URL  string `yaml:"url" json:"url"`
}

type ClientConfig struct {
	ClientBinaries []ClientBinary `yaml:"client_binaries"`
	LatestRevision string         `yaml:"latest_revision"`
}

// APIConfig holds relevant log and listener settings for the API server.
type APIConfig struct {
	HttpListenAddr string
}

// UIConfig holds relevant settings for the UI server.
type UIConfig struct {
	Url            string
	HelpUrl        string
	HttpListenAddr string
	// Secret to encrypt session storage
	Secret string
	// Default project to assume when none specified, e.g. when using
	// the /waterfall route use this project, while /waterfall/other-project
	// then use `other-project`
	DefaultProject string
	// Cache results of template compilation, so you don't have to re-read files
	// on every request. Note that if this is true, changes to HTML templates
	// won't take effect until server restart.
	CacheTemplates bool
	// SecureCookies sets the "secure" flag on user tokens. Evergreen
	// does not yet natively support SSL UI connections, but this option
	// is available, for example, for deployments behind HTTPS load balancers.
	SecureCookies bool
}

// MonitorConfig holds logging settings for the monitor process.
type MonitorConfig struct{}

// RunnerConfig holds logging and timing settings for the runner process.
type RunnerConfig struct {
	IntervalSeconds int64
}

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	SSHTimeoutSeconds int64
}

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	SMTP *SMTPConfig `yaml:"smtp"`
}

// SMTPConfig holds SMTP email settings.
type SMTPConfig struct {
	Server     string   `yaml:"server"`
	Port       int      `yaml:"port"`
	UseSSL     bool     `yaml:"use_ssl"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	AdminEmail []string `yaml:"admin_email"`
}

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	MergeToggle         int  `yaml:"mergetoggle"`
	UseLegacyTaskFinder bool `yaml:"use_legacy_task_finder"`
}

// TaskRunnerConfig holds logging settings for the scheduler process.
type TaskRunnerConfig struct {
}

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS          AWSConfig          `yaml:"aws"`
	DigitalOcean DigitalOceanConfig `yaml:"digitalocean"`
	Docker       DockerConfig       `yaml:"docker"`
	GCE          GCEConfig          `yaml:"gce"`
	OpenStack    OpenStackConfig    `yaml:"openstack"`
	VSphere      VSphereConfig      `yaml:"vsphere"`
}

// AWSConfig stores auth info for Amazon Web Services.
type AWSConfig struct {
	Secret string `yaml:"aws_secret"`
	Id     string `yaml:"aws_id"`
}

// DigitalOceanConfig stores auth info for Digital Ocean.
type DigitalOceanConfig struct {
	ClientId string `yaml:"client_id"`
	Key      string `yaml:"key"`
}

// DockerConfig stores auth info for Docker.
type DockerConfig struct {
	APIVersion string `yaml:"api_version"`
}

// OpenStackConfig stores auth info for Linaro using Identity V3. All fields required.
//
// The config is NOT compatible with Identity V2.
type OpenStackConfig struct {
	IdentityEndpoint string `yaml:"identity_endpoint"`

	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	DomainName string `yaml:"domain_name"`

	ProjectName string `yaml:"project_name"`
	ProjectID   string `yaml:"project_id"`

	Region string `yaml:"region"`
}

// GCEConfig stores auth info for Google Compute Engine. Can be retrieved from:
// https://developers.google.com/identity/protocols/application-default-credentials
type GCEConfig struct {
	ClientEmail  string `yaml:"client_email"`
	PrivateKey   string `yaml:"private_key"`
	PrivateKeyID string `yaml:"private_key_id"`
	TokenURI     string `yaml:"token_uri"`
}

// VSphereConfig stores auth info for VMware vSphere. The config fields refer
// to your vCenter server, a centralized management tool for the vSphere suite.
type VSphereConfig struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host     string
	Username string
	Password string
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

type AlertsConfig struct {
	SMTP *SMTPConfig `yaml:"smtp"`
}
type WriteConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	FSync    bool   `yaml:"fsync"`
	J        bool   `yaml:"j"`
}

type DBSettings struct {
	Url                  string       `yaml:"url"`
	SSL                  bool         `yaml:"ssl"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings WriteConcern `yaml:"write_concern"`
}

type LogBuffering struct {
	DurationSeconds int `yaml:"duration_seconds"`
	BufferCount     int `yaml:"count"`
}

type AmboyConfig struct {
	Name           string `yaml:"name"`
	DB             string `yaml:"database"`
	PoolSizeLocal  int    `yaml:"pool_size_local"`
	PoolSizeRemote int    `yaml:"pool_size_remote"`
	LocalStorage   int    `yaml:"local_storage_size"`
}

// Settings contains all configuration settings for running Evergreen.
type Settings struct {
	Database            DBSettings                `yaml:"database"`
	WriteConcern        WriteConcern              `yaml:"write_concern"`
	ConfigDir           string                    `yaml:"configdir"`
	ApiUrl              string                    `yaml:"api_url"`
	AgentExecutablesDir string                    `yaml:"agentexecutablesdir"`
	ClientBinariesDir   string                    `yaml:"client_binaries_dir"`
	SuperUsers          []string                  `yaml:"superusers"`
	Jira                JiraConfig                `yaml:"jira"`
	Splunk              send.SplunkConnectionInfo `yaml:"splunk"`
	Providers           CloudProviders            `yaml:"providers"`
	Keys                map[string]string         `yaml:"keys"`
	Credentials         map[string]string         `yaml:"credentials"`
	AuthConfig          AuthConfig                `yaml:"auth"`
	RepoTracker         RepoTrackerConfig         `yaml:"repotracker"`
	Monitor             MonitorConfig             `yaml:"monitor"`
	Api                 APIConfig                 `yaml:"api"`
	Alerts              AlertsConfig              `yaml:"alerts"`
	Ui                  UIConfig                  `yaml:"ui"`
	HostInit            HostInitConfig            `yaml:"hostinit"`
	Notify              NotifyConfig              `yaml:"notify"`
	Runner              RunnerConfig              `yaml:"runner"`
	Scheduler           SchedulerConfig           `yaml:"scheduler"`
	TaskRunner          TaskRunnerConfig          `yaml:"taskrunner"`
	Amboy               AmboyConfig               `yaml:"amboy"`
	Expansions          map[string]string         `yaml:"expansions"`
	Plugins             PluginConfig              `yaml:"plugins"`
	IsNonProd           bool                      `yaml:"isnonprod"`
	LogBuffering        LogBuffering              `yaml:"log_buffering"`
	LogPath             string                    `yaml:"log_path"`
	PprofPort           string                    `yaml:"pprof_port"`
}

// NewSettings builds an in-memory representation of the given settings file.
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

// Validate checks the settings and returns nil if the config is valid,
// or an error with a message explaining why otherwise.
func (settings *Settings) Validate() error {
	for _, validator := range configValidationRules {
		err := validator(settings)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Settings) GetSender() (send.Sender, error) {
	var (
		sender   send.Sender
		fallback send.Sender
		err      error
		senders  []send.Sender
	)

	fallback, err = send.NewErrorLogger("evergreen.err",
		send.LevelInfo{Default: level.Info, Threshold: level.Debug})
	if err != nil {
		return nil, errors.Wrap(err, "problem configuring err fallback logger")
	}

	if s.LogPath == LocalLoggingOverride {
		// log directly to systemd if possible, and log to
		// standard output otherwise.
		sender = getSystemLogger()
		err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback))
		if err != nil {
			return nil, errors.Wrap(err, "problem setting error handler")
		}
	} else {
		sender, err = send.MakeFileLogger(s.LogPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not configure file logger")
		}
		err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback))
		if err != nil {
			return nil, errors.Wrap(err, "problem setting error handler")
		}
	}
	senders = append(senders, sender)

	// set up external log aggregation services:
	if endpoint, ok := s.Credentials["sumologic"]; ok {
		sender, err = send.NewSumo("", endpoint)
		if err == nil {
			err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback))
			if err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LogBuffering.DurationSeconds)*time.Second,
					s.LogBuffering.BufferCount))
		}
	}

	if s.Splunk.Populated() {
		sender, err = send.NewSplunkLogger("", s.Splunk, grip.GetSender().Level())
		if err == nil {
			err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback))
			if err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LogBuffering.DurationSeconds)*time.Second,
					s.LogBuffering.BufferCount))
		}
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

// SessionFactory creates a usable SessionFactory from
// the Evergreen settings.
func (settings *Settings) SessionFactory() *legacyDB.SessionFactory {
	safety := mgo.Safe{}
	safety.W = settings.Database.WriteConcernSettings.W
	safety.WMode = settings.Database.WriteConcernSettings.WMode
	safety.WTimeout = settings.Database.WriteConcernSettings.WTimeout
	safety.FSync = settings.Database.WriteConcernSettings.FSync
	safety.J = settings.Database.WriteConcernSettings.J
	return legacyDB.NewSessionFactory(settings.Database.Url, settings.Database.DB, settings.Database.SSL, safety, defaultMgoDialTimeout)
}

// ConfigValidator is a type of function that checks the settings
// struct for any errors or missing required fields.
type configValidator func(settings *Settings) error

// ConfigValidationRules is the set of all ConfigValidator functions.
var configValidationRules = []configValidator{
	func(settings *Settings) error {
		if settings.Database.Url == "" || settings.Database.DB == "" {
			return errors.New("DBUrl and DB must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Amboy.Name == "" {
			settings.Amboy.Name = defaultAmboyQueueName
		}

		if settings.Amboy.DB == "" {
			settings.Amboy.DB = defaultAmboyDBName
		}

		if settings.Amboy.PoolSizeLocal == 0 {
			settings.Amboy.PoolSizeLocal = defaultAmboyPoolSize
		}

		if settings.Amboy.PoolSizeRemote == 0 {
			settings.Amboy.PoolSizeRemote = defaultAmboyPoolSize
		}

		if settings.Amboy.LocalStorage == 0 {
			settings.Amboy.LocalStorage = defaultAmboyLocalStorageSize
		}

		return nil
	},

	func(settings *Settings) error {
		if settings.LogBuffering.DurationSeconds == 0 {
			settings.LogBuffering.DurationSeconds = defaultLogBufferingDuration
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ClientBinariesDir == "" {
			settings.ClientBinariesDir = ClientDirectory
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.LogPath == "" {
			settings.LogPath = LocalLoggingOverride
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ApiUrl == "" {
			return errors.New("API hostname must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Secret == "" {
			return errors.New("UI Secret must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ConfigDir == "" {
			return errors.New("Config directory must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.DefaultProject == "" {
			return errors.New("You must specify a default project in UI")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Url == "" {
			return errors.New("You must specify a default UI url")
		}
		return nil
	},

	func(settings *Settings) error {
		notifyConfig := settings.Notify.SMTP

		if notifyConfig == nil {
			return nil
		}

		if notifyConfig.Server == "" || notifyConfig.Port == 0 {
			return errors.New("You must specify a SMTP server and port")
		}

		if len(notifyConfig.AdminEmail) == 0 {
			return errors.New("You must specify at least one admin_email")
		}

		if notifyConfig.From == "" {
			return errors.New("You must specify a from address")
		}

		return nil
	},

	func(settings *Settings) error {
		if settings.AuthConfig.Crowd == nil && settings.AuthConfig.Naive == nil && settings.AuthConfig.Github == nil {
			return errors.New("You must specify one form of authentication")
		}
		if settings.AuthConfig.Naive != nil {
			used := map[string]bool{}
			for _, x := range settings.AuthConfig.Naive.Users {
				if used[x.Username] {
					return errors.New("Duplicate user in list")
				}
				used[x.Username] = true
			}
		}
		if settings.AuthConfig.Github != nil {
			if settings.AuthConfig.Github.Users == nil && settings.AuthConfig.Github.Organization == "" {
				return errors.New("Must specify either a set of users or an organization for Github Authentication")
			}
		}
		return nil
	},
}
