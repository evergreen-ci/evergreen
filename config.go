package evergreen

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2019-03-22"

	errNotFound = "not found"
)

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
	ContainerPools     ContainerPoolsConfig      `yaml:"container_pools" bson:"container_pools" json:"container_pools" id:"container_pools"`
	Credentials        map[string]string         `yaml:"credentials" bson:"credentials" json:"credentials"`
	CredentialsNew     util.KeyValuePairSlice    `yaml:"credentials_new" bson:"credentials_new" json:"credentials_new"`
	Database           DBSettings                `yaml:"database"`
	Expansions         map[string]string         `yaml:"expansions" bson:"expansions" json:"expansions"`
	ExpansionsNew      util.KeyValuePairSlice    `yaml:"expansions_new" bson:"expansions_new" json:"expansions_new"`
	GithubPRCreatorOrg string                    `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	GoogleAnalyticsID  string                    `yaml:"google_analytics" bson:"google_analytics" json:"google_analytics"`
	HostInit           HostInitConfig            `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	Jira               JiraConfig                `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	JIRANotifications  JIRANotificationsConfig   `yaml:"jira_notifications" json:"jira_notifications" bson:"jira_notifications" id:"jira_notifications"`
	Keys               map[string]string         `yaml:"keys" bson:"keys" json:"keys"`
	KeysNew            util.KeyValuePairSlice    `yaml:"keys_new" bson:"keys_new" json:"keys_new"`
	LoggerConfig       LoggerConfig              `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath            string                    `yaml:"log_path" bson:"log_path" json:"log_path"`
	Notify             NotifyConfig              `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	Plugins            PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PluginsNew         util.KeyValuePairSlice    `yaml:"plugins_new" bson:"plugins_new" json:"plugins_new"`
	PprofPort          string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	Providers          CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	RepoTracker        RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	Scheduler          SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags       ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags"`
	Slack              SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	Splunk             send.SplunkConnectionInfo `yaml:"splunk" bson:"splunk" json:"splunk"`
	SuperUsers         []string                  `yaml:"superusers" bson:"superusers" json:"superusers"`
	Triggers           TriggerConfig             `yaml:"triggers" bson:"triggers" json:"triggers" id:"triggers"`
	Ui                 UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
}

func (c *Settings) SectionId() string { return ConfigDocID }

func (c *Settings) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = Settings{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *Settings) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			apiUrlKey:             c.ApiUrl,
			bannerKey:             c.Banner,
			bannerThemeKey:        c.BannerTheme,
			clientBinariesDirKey:  c.ClientBinariesDir,
			configDirKey:          c.ConfigDir,
			containerPoolsKey:     c.ContainerPools,
			credentialsKey:        c.Credentials,
			credentialsNewKey:     c.CredentialsNew,
			expansionsKey:         c.Expansions,
			expansionsNewKey:      c.ExpansionsNew,
			googleAnalyticsKey:    c.GoogleAnalyticsID,
			githubPRCreatorOrgKey: c.GithubPRCreatorOrg,
			keysKey:               c.Keys,
			keysNewKey:            c.KeysNew,
			logPathKey:            c.LogPath,
			pprofPortKey:          c.PprofPort,
			pluginsKey:            c.Plugins,
			pluginsNewKey:         c.PluginsNew,
			splunkKey:             c.Splunk,
			superUsersKey:         c.SuperUsers,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *Settings) ValidateAndDefault() error {
	var err error
	catcher := grip.NewSimpleCatcher()
	if c.ApiUrl == "" {
		catcher.Add(errors.New("API hostname must not be empty"))
	}
	if c.ConfigDir == "" {
		catcher.Add(errors.New("Config directory must not be empty"))
	}
	if len(c.CredentialsNew) > 0 {
		if c.Credentials, err = c.CredentialsNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing credentials"))
		}
	}
	if len(c.ExpansionsNew) > 0 {
		if c.Expansions, err = c.ExpansionsNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing expansions"))
		}
	}
	if len(c.KeysNew) > 0 {
		if c.Keys, err = c.KeysNew.Map(); err != nil {
			catcher.Add(errors.Wrap(err, "error parsing keys"))
		}
	}
	if len(c.PluginsNew) > 0 {
		tempPlugins, err := c.PluginsNew.NestedMap()
		if err != nil {
			catcher.Add(errors.Wrap(err, "error parsing plugins"))
		}
		c.Plugins = map[string]map[string]interface{}{}
		for k1, v1 := range tempPlugins {
			c.Plugins[k1] = map[string]interface{}{}
			for k2, v2 := range v1 {
				c.Plugins[k1][k2] = v2
			}
		}
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
	sections := ConfigRegistry.GetSections()
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
func (settings *Settings) SessionFactory() *db.SessionFactory {
	return CreateSession(settings.Database)
}

func CreateSession(settings DBSettings) *db.SessionFactory {
	safety := mgo.Safe{}
	safety.W = settings.WriteConcernSettings.W
	safety.WMode = settings.WriteConcernSettings.WMode
	safety.WTimeout = settings.WriteConcernSettings.WTimeout
	safety.FSync = settings.WriteConcernSettings.FSync
	safety.J = settings.WriteConcernSettings.J
	return db.NewSessionFactory(settings.Url, settings.DB, settings.SSL, safety, defaultMgoDialTimeout)
}

func (s *Settings) GetGithubOauthString() (string, error) {
	token, ok := s.Credentials["github"]
	if ok && token != "" {
		return token, nil
	}

	return "", errors.New("no github token in settings")
}

func (s *Settings) GetGithubOauthToken() (string, error) {
	oauthString, err := s.GetGithubOauthString()
	if err != nil {
		return "", err
	}
	splitToken := strings.Split(oauthString, " ")
	if len(splitToken) != 2 || splitToken[0] != "token" {
		return "", errors.New("token format was invalid, expected 'token [token]'")
	}
	return splitToken[1], nil
}

func GetServiceFlags() (*ServiceFlags, error) {
	section := ConfigRegistry.GetSection("service_flags")
	if section == nil {
		return nil, errors.New("unable to retrieve config section")
	}
	if err := section.Get(); err != nil {
		return nil, errors.Wrap(err, "error retrieving section from DB")
	}
	flags, ok := section.(*ServiceFlags)
	if !ok {
		return nil, errors.New("unable to convert config section to service flags")
	}
	return flags, nil
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

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
