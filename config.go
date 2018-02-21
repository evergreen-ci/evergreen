package evergreen

import (
	"fmt"
	"io/ioutil"
	"reflect"
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
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2018-02-21"

	errNotFound = "not found"
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
	ClientId     string   `bson:"client_id" json:"client_id" yaml:"client_id"`
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

func (c *AuthConfig) SectionId() string { return "auth" }
func (c *AuthConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *AuthConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"crowd":  c.Crowd,
			"naive":  c.Naive,
			"github": c.Github,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *AuthConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.Crowd == nil && c.Naive == nil && c.Github == nil {
		catcher.Add(errors.New("You must specify one form of authentication"))
	}
	if c.Naive != nil {
		used := map[string]bool{}
		for _, x := range c.Naive.Users {
			if used[x.Username] {
				catcher.Add(fmt.Errorf("Duplicate user %s in list", x.Username))
			}
			used[x.Username] = true
		}
	}
	if c.Github != nil {
		if c.Github.Users == nil && c.Github.Organization == "" {
			catcher.Add(errors.New("Must specify either a set of users or an organization for Github Authentication"))
		}
	}
	return catcher.Resolve()
}

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"numnewreporevisionstofetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"maxreporevisionstosearch"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"maxconcurrentrequests"`
}

func (c *RepoTrackerConfig) SectionId() string { return "repotracker" }
func (c *RepoTrackerConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *RepoTrackerConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"revs_to_fetch":      c.NumNewRepoRevisionsToFetch,
			"max_revs_to_search": c.MaxRepoRevisionsToSearch,
			"max_con_requests":   c.MaxConcurrentRequests,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *RepoTrackerConfig) ValidateAndDefault() error { return nil }

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

func (c *APIConfig) SectionId() string { return "api" }
func (c *APIConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *APIConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"http_listen_addr":      c.HttpListenAddr,
			"github_webhook_secret": c.GithubWebhookSecret,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *APIConfig) ValidateAndDefault() error { return nil }

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

func (c *UIConfig) SectionId() string { return "ui" }
func (c *UIConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *UIConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"url":              c.Url,
			"help_url":         c.HelpUrl,
			"http_listen_addr": c.HttpListenAddr,
			"secret":           c.Secret,
			"default_project":  c.DefaultProject,
			"cache_templates":  c.CacheTemplates,
			"secure_cookies":   c.SecureCookies,
			"csrf_key":         c.CsrfKey,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *UIConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.Secret == "" {
		catcher.Add(errors.New("UI Secret must not be empty"))
	}
	if c.DefaultProject == "" {
		catcher.Add(errors.New("You must specify a default project in UI"))
	}
	if c.Url == "" {
		catcher.Add(errors.New("You must specify a default UI url"))
	}
	if c.CsrfKey != "" && len(c.CsrfKey) != 32 {
		catcher.Add(errors.New("CSRF key must be 32 characters long"))
	}
	return catcher.Resolve()
}

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	SSHTimeoutSeconds int64 `bson:"ssh_timeout_secs" json:"ssh_timeout_secs" yaml:"sshtimeoutseconds"`
}

func (c *HostInitConfig) SectionId() string { return "hostinit" }
func (c *HostInitConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *HostInitConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"ssh_timeout_secs": c.SSHTimeoutSeconds,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *HostInitConfig) ValidateAndDefault() error { return nil }

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	SMTP *SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

func (c *NotifyConfig) SectionId() string { return "notify" }
func (c *NotifyConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *NotifyConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"smtp": c.SMTP,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *NotifyConfig) ValidateAndDefault() error {
	notifyConfig := c.SMTP

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

func (c *SchedulerConfig) SectionId() string { return "scheduler" }
func (c *SchedulerConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *SchedulerConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"merge_toggle": c.MergeToggle,
			"task_finder":  c.TaskFinder,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *SchedulerConfig) ValidateAndDefault() error {
	finders := []string{"legacy", "alternate", "parallel", "pipeline"}

	if c.TaskFinder == "" {
		// default to alternate
		c.TaskFinder = finders[0]
		return nil
	}

	if !sliceContains(finders, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			finders, c.TaskFinder)

	}
	return nil
}

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS       AWSConfig       `bson:"aws" json:"aws" yaml:"aws"`
	Docker    DockerConfig    `bson:"docker" json:"docker" yaml:"docker"`
	GCE       GCEConfig       `bson:"gce" json:"gce" yaml:"gce"`
	OpenStack OpenStackConfig `bson:"openstack" json:"openstack" yaml:"openstack"`
	VSphere   VSphereConfig   `bson:"vsphere" json:"vsphere" yaml:"vsphere"`
}

func (c *CloudProviders) SectionId() string { return "providers" }
func (c *CloudProviders) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *CloudProviders) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"aws":       c.AWS,
			"docker":    c.Docker,
			"gce":       c.GCE,
			"openstack": c.OpenStack,
			"vsphere":   c.VSphere,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *CloudProviders) ValidateAndDefault() error { return nil }

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

func (c *JiraConfig) SectionId() string { return "jira" }
func (c *JiraConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *JiraConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"host":            c.Host,
			"username":        c.Username,
			"password":        c.Password,
			"default_project": c.DefaultProject,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *JiraConfig) ValidateAndDefault() error { return nil }
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

func (c *AlertsConfig) SectionId() string { return "alerts" }
func (c *AlertsConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *AlertsConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"smtp": c.SMTP,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *AlertsConfig) ValidateAndDefault() error { return nil }

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
func (c *LoggerConfig) SectionId() string { return "logger_config" }
func (c *LoggerConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *LoggerConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"buffer":          c.Buffer,
			"default_level":   c.DefaultLevel,
			"threshold_level": c.ThresholdLevel,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *LoggerConfig) ValidateAndDefault() error {
	if c.Buffer.DurationSeconds == 0 {
		c.Buffer.DurationSeconds = defaultLogBufferingDuration
	}

	if c.DefaultLevel == "" {
		c.DefaultLevel = "info"
	}

	if c.ThresholdLevel == "" {
		c.ThresholdLevel = "debug"
	}

	info := c.Info()
	if !info.Valid() {
		return errors.Errorf("logging level configuration is not valid [%+v]", info)
	}

	return nil
}

type LogBuffering struct {
	DurationSeconds int `bson:"duration_seconds" json:"duration_seconds" yaml:"duration_seconds"`
	Count           int `bson:"count" json:"count" yaml:"count"`
}

type AmboyConfig struct {
	Name           string `bson:"name" json:"name" yaml:"name"`
	DB             string `bson:"database" json:"database" yaml:"database"`
	PoolSizeLocal  int    `bson:"pool_size_local" json:"pool_size_local" yaml:"pool_size_local"`
	PoolSizeRemote int    `bson:"pool_size_remote" json:"pool_size_remote" yaml:"pool_size_remote"`
	LocalStorage   int    `bson:"local_storage_size" json:"local_storage_size" yaml:"local_storage_size"`
}

func (c *AmboyConfig) SectionId() string { return "amboy" }
func (c *AmboyConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *AmboyConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"name":               c.Name,
			"database":           c.DB,
			"pool_size_local":    c.PoolSizeLocal,
			"pool_size_remote":   c.PoolSizeRemote,
			"local_storage_size": c.LocalStorage,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *AmboyConfig) ValidateAndDefault() error {
	if c.Name == "" {
		c.Name = defaultAmboyQueueName
	}

	if c.DB == "" {
		c.DB = defaultAmboyDBName
	}

	if c.PoolSizeLocal == 0 {
		c.PoolSizeLocal = defaultAmboyPoolSize
	}

	if c.PoolSizeRemote == 0 {
		c.PoolSizeRemote = defaultAmboyPoolSize
	}

	if c.LocalStorage == 0 {
		c.LocalStorage = defaultAmboyLocalStorageSize
	}

	return nil
}

type SlackConfig struct {
	Options *send.SlackOptions `bson:"options" json:"options" yaml:"options"`
	Token   string             `bson:"token" json:"token" yaml:"token"`
	Level   string             `bson:"level" json:"level" yaml:"level"`
}

func (c *SlackConfig) SectionId() string { return "slack" }
func (c *SlackConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *SlackConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"options": c.Options,
			"token":   c.Token,
			"level":   c.Level,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *SlackConfig) ValidateAndDefault() error {
	if c.Options == nil {
		c.Options = &send.SlackOptions{}
	}

	if c.Token != "" {
		if c.Options.Channel == "" {
			c.Options.Channel = "#evergreen-ops-alerts"
		}

		if c.Options.Name == "" {
			c.Options.Name = "evergreen"
		}

		if err := c.Options.Validate(); err != nil {
			return errors.Wrap(err, "with a non-empty token, you must specify a valid slack configuration")
		}

		if !level.IsValidPriority(level.FromString(c.Level)) {
			return errors.Errorf("%s is not a valid priority", c.Level)
		}
	}

	return nil
}

type NewRelicConfig struct {
	ApplicationName string `bson:"application_name" json:"application_name" yaml:"application_name"`
	LicenseKey      string `bson:"license_key" json:"license_key" yaml:"license_key"`
}

func (c *NewRelicConfig) SectionId() string { return "new_relic" }
func (c *NewRelicConfig) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *NewRelicConfig) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"application_name": c.ApplicationName,
			"license_key":      c.LicenseKey,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *NewRelicConfig) ValidateAndDefault() error { return nil }

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled         bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostinitDisabled             bool `bson:"hostinit_disabled" json:"hostinit_disabled"`
	MonitorDisabled              bool `bson:"monitor_disabled" json:"monitor_disabled"`
	NotificationsDisabled        bool `bson:"notifications_disabled" json:"notifications_disabled"`
	AlertsDisabled               bool `bson:"alerts_disabled" json:"alerts_disabled"`
	TaskrunnerDisabled           bool `bson:"taskrunner_disabled" json:"taskrunner_disabled"`
	RepotrackerDisabled          bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled            bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
	GithubPRTestingDisabled      bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	RepotrackerPushEventDisabled bool `bson:"repotracker_push_event_disabled" json:"repotracker_push_event_disabled"`
	CLIUpdatesDisabled           bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled" json:"github_status_api_disabled"`
}

func (c *ServiceFlags) SectionId() string { return "service_flags" }
func (c *ServiceFlags) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *ServiceFlags) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			taskDispatchKey:                 c.TaskDispatchDisabled,
			hostinitKey:                     c.HostinitDisabled,
			monitorKey:                      c.MonitorDisabled,
			notificationsKey:                c.NotificationsDisabled,
			alertsKey:                       c.AlertsDisabled,
			taskrunnerKey:                   c.TaskrunnerDisabled,
			repotrackerKey:                  c.RepotrackerDisabled,
			schedulerKey:                    c.SchedulerDisabled,
			githubPRTestingDisabledKey:      c.GithubPRTestingDisabled,
			repotrackerPushEventDisabledKey: c.RepotrackerPushEventDisabled,
			cliUpdatesDisabledKey:           c.CLIUpdatesDisabled,
			githubStatusAPIDisabled:         c.GithubStatusAPIDisabled,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *ServiceFlags) ValidateAndDefault() error { return nil }

// supported banner themes in Evergreen
type BannerTheme string

const (
	Announcement BannerTheme = "announcement"
	Information              = "information"
	Warning                  = "warning"
	Important                = "important"
)

func IsValidBannerTheme(input string) (bool, BannerTheme) {
	switch input {
	case "":
		return true, ""
	case "announcement":
		return true, Announcement
	case "information":
		return true, Information
	case "warning":
		return true, Warning
	case "important":
		return true, Important
	default:
		return false, ""
	}
}

// Settings contains all configuration settings for running Evergreen.
type Settings struct {
	Id                 string                    `bson:"_id" json:"id"`
	Alerts             AlertsConfig              `yaml:"alerts" bson:"alerts" json:"alerts" id:"alerts"`
	Amboy              AmboyConfig               `yaml:"amboy" bson:"amboy" json:"amboy" id:"amboy"`
	Api                APIConfig                 `yaml:"api" bson:"api" json:"api" id:"api"`
	ApiUrl             string                    `yaml:"api_url" bson:"api_url" json:"api_url"`
	AuthConfig         AuthConfig                `yaml:"auth" bson:"auth" json:"auth" id:"auth"`
	Banner             string                    `bson:"banner" json:"banner"`
	BannerTheme        BannerTheme               `bson:"banner_theme" json:"banner_theme"`
	ClientBinariesDir  string                    `yaml:"client_binaries_dir" bson:"client_binaries_dir" json:"client_binaries_dir"`
	ConfigDir          string                    `yaml:"configdir" bson:"configdir" json:"configdir"`
	Credentials        map[string]string         `yaml:"credentials" bson:"credentials" json:"credentials"`
	Database           DBSettings                `yaml:"database"`
	Expansions         map[string]string         `yaml:"expansions" bson:"expansions" json:"expansions"`
	GithubPRCreatorOrg string                    `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	HostInit           HostInitConfig            `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	IsNonProd          bool                      `yaml:"isnonprod" bson:"isnonprod" json:"isnonprod"`
	Jira               JiraConfig                `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	Keys               map[string]string         `yaml:"keys" bson:"keys" json:"keys"`
	LoggerConfig       LoggerConfig              `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath            string                    `yaml:"log_path" bson:"log_path" json:"log_path"`
	NewRelic           NewRelicConfig            `yaml:"new_relic" bson:"new_relic" json:"new_relic" id:"new_relic"`
	Notify             NotifyConfig              `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	Plugins            PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PprofPort          string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	Providers          CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	RepoTracker        RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	Scheduler          SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags       ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags"`
	Slack              SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	Splunk             send.SplunkConnectionInfo `yaml:"splunk" bson:"splunk" json:"splunk"`
	SuperUsers         []string                  `yaml:"superusers" bson:"superusers" json:"superusers"`
	Ui                 UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
	WriteConcern       WriteConcern              `yaml:"write_concern"`
}

func (c *Settings) SectionId() string { return configDocID }
func (c *Settings) Get() error {
	err := legacyDB.FindOneQ(ConfigCollection, legacyDB.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *Settings) Set() error {
	_, err := legacyDB.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			apiUrlKey:             c.ApiUrl,
			bannerKey:             c.Banner,
			bannerThemeKey:        c.BannerTheme,
			clientBinariesDirKey:  c.ClientBinariesDir,
			configDirKey:          c.ConfigDir,
			credentialsKey:        c.Credentials,
			expansionsKey:         c.Expansions,
			githubPRCreatorOrgKey: c.GithubPRCreatorOrg,
			isNonProdKey:          c.IsNonProd,
			keysKey:               c.Keys,
			logPathKey:            c.LogPath,
			pprofPortKey:          c.PprofPort,
			pluginsKey:            c.Plugins,
			splunkKey:             c.Splunk,
			superUsersKey:         c.SuperUsers,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *Settings) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.ApiUrl == "" {
		catcher.Add(errors.New("API hostname must not be empty"))
	}
	if c.ConfigDir == "" {
		catcher.Add(errors.New("Config directory must not be empty"))
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if c.ClientBinariesDir == "" {
		c.ClientBinariesDir = ClientDirectory
	}
	if c.LogPath == "" {
		c.LogPath = LocalLoggingOverride
	}
	return nil
}

// ConfigSection defines a sub-document in the evegreen config
// any config sections must also be added to registry.go
type ConfigSection interface {
	// SectionId() returns the ID of the section to be used in the database document and struct tag
	SectionId() string
	// Get() populates the section from the DB
	Get() error
	// Set() upserts the section document into the DB
	Set() error
	// ValidateAndDefault() validates input and sets defaults
	ValidateAndDefault() error
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

// GetConfig retrieves the Evergreen config document. If no document is
// present in the DB, it will return the defaults
func GetConfig() (*Settings, error) {
	config := &Settings{}

	// retrieve the root config document
	if err := config.Get(); err != nil {
		return nil, err
	}

	// retrieve the other config sub-documents and form the whole struct
	catcher := grip.NewSimpleCatcher()
	sections := configRegistry.getSections()
	valConfig := reflect.ValueOf(*config)
	//iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its corresponding section in the registry
		propName := valConfig.Type().Field(i).Name
		section, ok := sections[sectionId]
		if !ok {
			catcher.Add(fmt.Errorf("config section %s not found in registry", sectionId))
			continue
		}

		// retrieve the section's document from the db
		if err := section.Get(); err != nil {
			catcher.Add(errors.Wrapf(err, "error populating section %s", sectionId))
			continue
		}

		// set the value of the section struct to the value of the corresponding field in the config
		sectionVal := reflect.ValueOf(section).Elem()
		propVal := reflect.ValueOf(config).Elem().FieldByName(propName)
		if !propVal.CanSet() {
			catcher.Add(fmt.Errorf("unable to set field %s in %s", propName, sectionId))
			continue
		}
		propVal.Set(sectionVal)
	}

	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())
	}
	return config, nil
}

// UpdateConfig updates all evergreen settings documents in DB
func UpdateConfig(config *Settings) error {
	// update the root config document
	if err := config.Set(); err != nil {
		return err
	}

	// update the other config sub-documents
	catcher := grip.NewSimpleCatcher()
	valConfig := reflect.ValueOf(*config)

	//iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its value within the settings struct
		propName := valConfig.Type().Field(i).Name
		propVal := valConfig.FieldByName(propName)

		// create a reflective copy of the struct
		valPointer := reflect.Indirect(reflect.New(propVal.Type()))
		valPointer.Set(propVal)

		// convert the pointer to that struct to an empty interface
		propInterface := valPointer.Addr().Interface()

		// type assert to the ConfigSection interface
		section, ok := propInterface.(ConfigSection)
		if !ok {
			catcher.Add(fmt.Errorf("unable to convert config section %s", propName))
			continue
		}
		catcher.Add(section.Set())
	}

	return errors.WithStack(catcher.Resolve())
}

// Validate checks the settings and returns nil if the config is valid,
// or an error with a message explaining why otherwise.
func (settings *Settings) Validate() error {
	catcher := grip.NewSimpleCatcher()

	// validate the root-level settings struct
	catcher.Add(settings.ValidateAndDefault())

	// validate each sub-document
	valConfig := reflect.ValueOf(*settings)
	// iterate over each field in the config struct
	for i := 0; i < valConfig.NumField(); i++ {
		// retrieve the 'id' struct tag
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" { // no 'id' tag means this is a simple field that we can skip
			continue
		}

		// get the property name and find its value within the settings struct
		propName := valConfig.Type().Field(i).Name
		propVal := valConfig.FieldByName(propName)

		// the goal is to convert this struct which we know implements ConfigSection
		// from a reflection data structure back to the interface
		// the below creates a copy and takes the address of it as a workaround because
		// you can't take the address of it via reflection for some reason
		// (and all interface methods on the struct have pointer receivers)

		// create a reflective copy of the struct
		valPointer := reflect.Indirect(reflect.New(propVal.Type()))
		valPointer.Set(propVal)

		// convert the pointer to that struct to an empty interface
		propInterface := valPointer.Addr().Interface()

		// type assert to the ConfigSection interface
		section, ok := propInterface.(ConfigSection)
		if !ok {
			catcher.Add(fmt.Errorf("unable to convert config section %s", propName))
			continue
		}
		err := section.ValidateAndDefault()
		if err != nil {
			catcher.Add(err)
			continue
		}

		// set the value of the section struct in case there was any defaulting done
		sectionVal := reflect.ValueOf(section).Elem()
		propAddr := reflect.ValueOf(settings).Elem().FieldByName(propName)
		if !propAddr.CanSet() {
			catcher.Add(fmt.Errorf("unable to set field %s in %s", propName, sectionId))
			continue
		}
		propAddr.Set(sectionVal)
	}
	return errors.WithStack(catcher.Resolve())
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
