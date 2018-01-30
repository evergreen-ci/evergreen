package evergreen

import (
	"io/ioutil"
	"strings"
	"time"

	legacyDB "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	newrelic "github.com/newrelic/go-agent"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2018-01-31"
)

// AuthUser configures a user for our Naive authentication setup.
type AuthUser struct {
	Username    string `bson:"username" json:"username" yaml:"username"`
	DisplayName string `bson:"display_name" json:"display_name" yaml:"display_name"`
	Password    string `bson:"password" json:"password" yaml:"password"`
	Email       string `bson:"email" json:"email" yaml:"email"`
}

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []*AuthUser `bson:"users" json:"users" yaml:"users"`
}

// CrowdConfig holds settings for interacting with Atlassian Crowd.
type CrowdConfig struct {
	Username string `bson:"username" json:"username" yaml:"username"`
	Password string `bson:"password" json:"password" yaml:"password"`
	Urlroot  string `bson:"url_root" json:"url_root" yaml:"urlroot"`
}

// GithubAuthConfig holds settings for interacting with Github Authentication including the
// ClientID, ClientSecret and CallbackUri which are given when registering the application
// Furthermore,
type GithubAuthConfig struct {
	ClientId     string   `bson:"client_id" bson:"client_id" yaml:"client_id"`
	ClientSecret string   `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	Users        []string `bson:"users" json:"users" yaml:"users"`
	Organization string   `bson:"organization" json:"organization" yaml:"organization"`
}

// AuthConfig has a pointer to either a CrowConfig or a NaiveAuthConfig.
type AuthConfig struct {
	Crowd  *CrowdConfig      `bson:"crowd" json:"crowd" yaml:"crowd"`
	Naive  *NaiveAuthConfig  `bson:"naive" json:"naive" yaml:"naive"`
	Github *GithubAuthConfig `bson:"github" json:"github" yaml:"github"`
}

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"numnewreporevisionstofetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"maxreporevisionstosearch"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"maxconcurrentrequests"`
}

type ClientBinary struct {
	Arch string `yaml:"arch" json:"arch"`
	OS   string `yaml:"os" json:"os"`
	URL  string `yaml:"url" json:"url"`
}

type ClientConfig struct {
	ClientBinaries []ClientBinary `yaml:"client_binaries" json:"ClientBinaries"`
	LatestRevision string         `yaml:"latest_revision" json:"LatestRevision"`
}

// APIConfig holds relevant log and listener settings for the API server.
type APIConfig struct {
	HttpListenAddr      string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	GithubWebhookSecret string `bson:"github_webhook_secret" json:"github_webhook_secret" yaml:"github_webhook_secret"`
}

// UIConfig holds relevant settings for the UI server.
type UIConfig struct {
	Url            string `bson:"url" json:"url" yaml:"url"`
	HelpUrl        string `bson:"help_url" json:"help_url" yaml:"helpurl"`
	HttpListenAddr string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	// Secret to encrypt session storage
	Secret string `bson:"secret" json:"secret" yaml:"secret"`
	// Default project to assume when none specified, e.g. when using
	// the /waterfall route use this project, while /waterfall/other-project
	// then use `other-project`
	DefaultProject string `bson:"default_project" json:"default_project" yaml:"defaultproject"`
	// Cache results of template compilation, so you don't have to re-read files
	// on every request. Note that if this is true, changes to HTML templates
	// won't take effect until server restart.
	CacheTemplates bool `bson:"cache_templates" json:"cache_templates" yaml:"cachetemplates"`
	// SecureCookies sets the "secure" flag on user tokens. Evergreen
	// does not yet natively support SSL UI connections, but this option
	// is available, for example, for deployments behind HTTPS load balancers.
	SecureCookies bool `bson:"secure_cookies" json:"secure_cookies" yaml:"securecookies"`
	// CsrfKey is a 32-byte key used to generate tokens that validate UI requests
	CsrfKey string `bson:"csrf_key" json:"csrf_key" yaml:"csrfkey"`
}

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	SSHTimeoutSeconds int64 `bson:"ssh_timeout_secs" json:"ssh_timeout_secs" yaml:"sshtimeoutseconds"`
}

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	SMTP *SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

// SMTPConfig holds SMTP email settings.
type SMTPConfig struct {
	Server     string   `bson:"server" json:"server" yaml:"server"`
	Port       int      `bson:"port" json:"port" yaml:"port"`
	UseSSL     bool     `bson:"use_ssl" json:"use_ssl" yaml:"use_ssl"`
	Username   string   `bson:"username" json:"username" yaml:"username"`
	Password   string   `bson:"password" json:"password" yaml:"password"`
	From       string   `bson:"from" json:"from" yaml:"from"`
	AdminEmail []string `bson:"admin_email" json:"admin_email" yaml:"admin_email"`
}

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	MergeToggle int    `bson:"merge_toggle" json:"merge_toggle" yaml:"mergetoggle"`
	TaskFinder  string `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
}

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS       AWSConfig       `bson:"aws" json:"aws" yaml:"aws"`
	Docker    DockerConfig    `bson:"docker" json:"docker" yaml:"docker"`
	GCE       GCEConfig       `bson:"gce" json:"gce" yaml:"gce"`
	OpenStack OpenStackConfig `bson:"openstack" json:"openstack" yaml:"openstack"`
	VSphere   VSphereConfig   `bson:"vsphere" json:"vsphere" yaml:"vsphere"`
}

// AWSConfig stores auth info for Amazon Web Services.
type AWSConfig struct {
	Secret string `bson:"aws_secret" json:"aws_secret" yaml:"aws_secret"`
	Id     string `bson:"aws_id" json:"aws_id" yaml:"aws_id"`
}

// DockerConfig stores auth info for Docker.
type DockerConfig struct {
	APIVersion string `bson:"api_version" json:"api_version" yaml:"api_version"`
}

// OpenStackConfig stores auth info for Linaro using Identity V3. All fields required.
//
// The config is NOT compatible with Identity V2.
type OpenStackConfig struct {
	IdentityEndpoint string `bson:"identity_endpoint" json:"identity_endpoint" yaml:"identity_endpoint"`

	Username   string `bson:"username" json:"username" yaml:"username"`
	Password   string `bson:"password" json:"password" yaml:"password"`
	DomainName string `bson:"domain_name" json:"domain_name" yaml:"domain_name"`

	ProjectName string `bson:"project_name" json:"project_name" yaml:"project_name"`
	ProjectID   string `bson:"project_id" json:"project_id" yaml:"project_id"`

	Region string `bson:"region" json:"region" yaml:"region"`
}

// GCEConfig stores auth info for Google Compute Engine. Can be retrieved from:
// https://developers.google.com/identity/protocols/application-default-credentials
type GCEConfig struct {
	ClientEmail  string `bson:"client_email" json:"client_email" yaml:"client_email"`
	PrivateKey   string `bson:"private_key" json:"private_key" yaml:"private_key"`
	PrivateKeyID string `bson:"private_key_id" json:"private_key_id" yaml:"private_key_id"`
	TokenURI     string `bson:"token_uri" json:"token_uri" yaml:"token_uri"`
}

// VSphereConfig stores auth info for VMware vSphere. The config fields refer
// to your vCenter server, a centralized management tool for the vSphere suite.
type VSphereConfig struct {
	Host     string `bson:"host" json:"host" yaml:"host"`
	Username string `bson:"username" json:"username" yaml:"username"`
	Password string `bson:"password" json:"password" yaml:"password"`
}

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host           string `yaml:"host" bson:"host" json:"host"`
	Username       string `yaml:"username" bson:"username" json:"username"`
	Password       string `yaml:"password" bson:"password" json:"password"`
	DefaultProject string `yaml:"default_project" bson:"default_project" json:"default_project"`
}

func (j JiraConfig) GetHostURL() string {
	if strings.HasPrefix("http", j.Host) {
		return j.Host
	}

	return "https://" + j.Host
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

type AlertsConfig struct {
	SMTP *SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
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

type LoggerConfig struct {
	Buffer         LogBuffering `bson:"buffer" json:"buffer" yaml:"buffer"`
	DefaultLevel   string       `bson:"default_level" json:"default_level" yaml:"default_level"`
	ThresholdLevel string       `bson:"threshold_level" json:"threshold_level" yaml:"threshold_level"`
}

func (c LoggerConfig) Info() send.LevelInfo {
	return send.LevelInfo{
		Default:   level.FromString(c.DefaultLevel),
		Threshold: level.FromString(c.ThresholdLevel),
	}
}

type LogBuffering struct {
	DurationSeconds int `yaml:"duration_seconds"`
	Count           int `yaml:"count"`
}

type AmboyConfig struct {
	Name           string `bson:"name" json:"name" yaml:"name"`
	DB             string `bson:"database" json:"database" yaml:"database"`
	PoolSizeLocal  int    `bson:"pool_size_local" json:"pool_size_local" yaml:"pool_size_local"`
	PoolSizeRemote int    `bson:"pool_size_remote" json:"pool_size_remote" yaml:"pool_size_remote"`
	LocalStorage   int    `bson:"local_storage_size" json:"local_storage_size" yaml:"local_storage_size"`
}

type SlackConfig struct {
	Options *send.SlackOptions `bson:"options" json:"options" yaml:"options"`
	Token   string             `bson:"token" json:"token" yaml:"token"`
	Level   string             `bson:"level" json:"level" yaml:"level"`
}

type NewRelicConfig struct {
	ApplicationName string `bson:"application_name" json:"application_name" yaml:"application_name"`
	LicenseKey      string `bson:"license_key" json:"license_key" yaml:"license_key"`
}

// Settings contains all configuration settings for running Evergreen.
type Settings struct {
	Alerts             AlertsConfig              `yaml:"alerts"`
	Amboy              AmboyConfig               `yaml:"amboy"`
	Api                APIConfig                 `yaml:"api"`
	ApiUrl             string                    `yaml:"api_url"`
	AuthConfig         AuthConfig                `yaml:"auth"`
	ClientBinariesDir  string                    `yaml:"client_binaries_dir"`
	ConfigDir          string                    `yaml:"configdir"`
	Credentials        map[string]string         `yaml:"credentials"`
	Database           DBSettings                `yaml:"database"`
	Expansions         map[string]string         `yaml:"expansions"`
	GithubPRCreatorOrg string                    `yaml:"github_pr_creator_org"`
	HostInit           HostInitConfig            `yaml:"hostinit"`
	IsNonProd          bool                      `yaml:"isnonprod"`
	Jira               JiraConfig                `yaml:"jira"`
	Keys               map[string]string         `yaml:"keys"`
	LoggerConfig       LoggerConfig              `yaml:"logger_config"`
	LogPath            string                    `yaml:"log_path"`
	NewRelic           NewRelicConfig            `yaml:"new_relic"`
	Notify             NotifyConfig              `yaml:"notify"`
	Plugins            PluginConfig              `yaml:"plugins"`
	PprofPort          string                    `yaml:"pprof_port"`
	Providers          CloudProviders            `yaml:"providers"`
	RepoTracker        RepoTrackerConfig         `yaml:"repotracker"`
	Scheduler          SchedulerConfig           `yaml:"scheduler"`
	Slack              SlackConfig               `yaml:"slack"`
	Splunk             send.SplunkConnectionInfo `yaml:"splunk"`
	SuperUsers         []string                  `yaml:"superusers"`
	Ui                 UIConfig                  `yaml:"ui"`
	WriteConcern       WriteConcern              `yaml:"write_concern"`
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

func (s *Settings) GetSender(env Environment) (send.Sender, error) {
	var (
		sender   send.Sender
		fallback send.Sender
		err      error
		senders  []send.Sender
	)

	levelInfo := s.LoggerConfig.Info()

	fallback, err = send.NewErrorLogger("evergreen.err",
		send.LevelInfo{Default: level.Info, Threshold: level.Debug})
	if err != nil {
		return nil, errors.Wrap(err, "problem configuring err fallback logger")
	}

	// setup the base/default logger (generally direct to systemd
	// or standard output)
	switch s.LogPath {
	case LocalLoggingOverride:
		// log directly to systemd if possible, and log to
		// standard output otherwise.
		sender = getSystemLogger()
	case StandardOutputLoggingOverride, "":
		sender = send.MakeNative()
	default:
		sender, err = send.MakeFileLogger(s.LogPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not configure file logger")
		}
	}

	if err = sender.SetLevel(levelInfo); err != nil {
		return nil, errors.Wrap(err, "problem setting level")
	}
	if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
		return nil, errors.Wrap(err, "problem setting error handler")
	}
	senders = append(senders, sender)

	// set up external log aggregation services:
	//
	if endpoint, ok := s.Credentials["sumologic"]; ok {
		sender, err = send.NewSumo("", endpoint)
		if err == nil {
			if err = sender.SetLevel(levelInfo); err != nil {
				return nil, errors.Wrap(err, "problem setting level")
			}
			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LoggerConfig.Buffer.DurationSeconds)*time.Second,
					s.LoggerConfig.Buffer.Count))
		}
	}

	if s.Splunk.Populated() {
		sender, err = send.NewSplunkLogger("", s.Splunk, grip.GetSender().Level())
		if err == nil {
			if err = sender.SetLevel(levelInfo); err != nil {
				return nil, errors.Wrap(err, "problem setting level")
			}
			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LoggerConfig.Buffer.DurationSeconds)*time.Second,
					s.LoggerConfig.Buffer.Count))
		}
	}

	// the slack logging service is only for logging very high level alerts.
	if s.Slack.Token != "" {
		sender, err = send.NewSlackLogger(s.Slack.Options, s.Slack.Token,
			send.LevelInfo{Default: level.Critical, Threshold: level.FromString(s.Slack.Level)})
		if err == nil {
			var slackFallback send.Sender

			switch len(senders) {
			case 0:
				slackFallback = fallback
			case 1:
				slackFallback = senders[0]
			default:
				slackFallback = send.NewConfiguredMultiSender(senders...)
			}

			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(slackFallback)); err != nil {
				return nil, errors.Wrap(err, "problem setting error handler")
			}

			senders = append(senders, logger.MakeQueueSender(env.LocalQueue(), sender))
		}
		grip.Warning(errors.Wrap(err, "problem setting up slack alert logger"))
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

func (s *Settings) GetGithubOauthToken() (string, error) {
	token, ok := s.Credentials["github"]
	if ok && token != "" {
		return token, nil
	}

	return "", errors.New("no github token in settings")
}

func (n *NewRelicConfig) SetUp() (newrelic.Application, error) {
	if n.ApplicationName == "" || n.LicenseKey == "" {
		return nil, nil
	}
	config := newrelic.NewConfig(n.ApplicationName, n.LicenseKey)
	app, err := newrelic.NewApplication(config)
	if err != nil || app == nil {
		return nil, errors.Wrap(err, "error creating New Relic application")
	}
	return app, nil
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
		if settings.LoggerConfig.Buffer.DurationSeconds == 0 {
			settings.LoggerConfig.Buffer.DurationSeconds = defaultLogBufferingDuration
		}

		if settings.LoggerConfig.DefaultLevel == "" {
			settings.LoggerConfig.DefaultLevel = "info"
		}

		if settings.LoggerConfig.ThresholdLevel == "" {
			settings.LoggerConfig.ThresholdLevel = "debug"
		}

		info := settings.LoggerConfig.Info()
		if !info.Valid() {
			return errors.Errorf("logging level configuration is not valid [%+v]", info)
		}

		return nil
	},

	func(settings *Settings) error {
		if settings.Slack.Options == nil {
			settings.Slack.Options = &send.SlackOptions{}
		}

		if settings.Slack.Token != "" {
			if settings.Slack.Options.Channel == "" {
				settings.Slack.Options.Channel = "#evergreen-ops-alerts"
			}

			if settings.Slack.Options.Name == "" {
				settings.Slack.Options.Name = "evergreen"
			}

			if err := settings.Slack.Options.Validate(); err != nil {
				return errors.Wrap(err, "with a non-empty token, you must specify a valid slack configuration")
			}

			if !level.IsValidPriority(level.FromString(settings.Slack.Level)) {
				return errors.Errorf("%s is not a valid priority", settings.Slack.Level)
			}
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
		finders := []string{"legacy", "alternate", "parallel", "pipeline"}

		if settings.Scheduler.TaskFinder == "" {
			// default to alternate
			settings.Scheduler.TaskFinder = finders[0]
			return nil
		}

		if !sliceContains(finders, settings.Scheduler.TaskFinder) {
			return errors.Errorf("supported finders are %s; %s is not supported",
				finders, settings.Scheduler.TaskFinder)

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

	func(settings *Settings) error {
		if settings.Ui.CsrfKey != "" && len(settings.Ui.CsrfKey) != 32 {
			return errors.New("CSRF key must be 32 characters long")
		}
		return nil
	},
}

func sliceContains(slice []string, elem string) bool {
	if slice == nil {
		return false
	}
	for _, i := range slice {
		if i == elem {
			return true
		}
	}

	return false
}
