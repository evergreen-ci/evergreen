package evergreen

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	yaml "gopkg.in/yaml.v3"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "2021-11-01"

	// Agent version to control agent rollover.
	AgentVersion = "2021-10-20"
)

// ConfigSection defines a sub-document in the evergreen config
// any config sections must also be added to registry.go
type ConfigSection interface {
	// SectionId() returns the ID of the section to be used in the database document and struct tag
	SectionId() string
	// Get() populates the section from the DB
	Get(Environment) error
	// Set() upserts the section document into the DB
	Set() error
	// ValidateAndDefault() validates input and sets defaults
	ValidateAndDefault() error
}

// Settings contains all configuration settings for running Evergreen. Settings
// with the "id" struct tag should implement the ConfigSection interface.
type Settings struct {
	Id                  string                    `bson:"_id" json:"id" yaml:"id"`
	Alerts              AlertsConfig              `yaml:"alerts" bson:"alerts" json:"alerts" id:"alerts"`
	Amboy               AmboyConfig               `yaml:"amboy" bson:"amboy" json:"amboy" id:"amboy"`
	Api                 APIConfig                 `yaml:"api" bson:"api" json:"api" id:"api"`
	ApiUrl              string                    `yaml:"api_url" bson:"api_url" json:"api_url"`
	AuthConfig          AuthConfig                `yaml:"auth" bson:"auth" json:"auth" id:"auth"`
	Banner              string                    `bson:"banner" json:"banner" yaml:"banner"`
	BannerTheme         BannerTheme               `bson:"banner_theme" json:"banner_theme" yaml:"banner_theme"`
	Cedar               CedarConfig               `bson:"cedar" json:"cedar" yaml:"cedar" id:"cedar"`
	ClientBinariesDir   string                    `yaml:"client_binaries_dir" bson:"client_binaries_dir" json:"client_binaries_dir"`
	CommitQueue         CommitQueueConfig         `yaml:"commit_queue" bson:"commit_queue" json:"commit_queue" id:"commit_queue"`
	ConfigDir           string                    `yaml:"configdir" bson:"configdir" json:"configdir"`
	ContainerPools      ContainerPoolsConfig      `yaml:"container_pools" bson:"container_pools" json:"container_pools" id:"container_pools"`
	Credentials         map[string]string         `yaml:"credentials" bson:"credentials" json:"credentials"`
	CredentialsNew      util.KeyValuePairSlice    `yaml:"credentials_new" bson:"credentials_new" json:"credentials_new"`
	Database            DBSettings                `yaml:"database" json:"database" bson:"database"`
	DomainName          string                    `yaml:"domain_name" bson:"domain_name" json:"domain_name"`
	Expansions          map[string]string         `yaml:"expansions" bson:"expansions" json:"expansions"`
	ExpansionsNew       util.KeyValuePairSlice    `yaml:"expansions_new" bson:"expansions_new" json:"expansions_new"`
	GithubPRCreatorOrg  string                    `yaml:"github_pr_creator_org" bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	GithubOrgs          []string                  `yaml:"github_orgs" bson:"github_orgs" json:"github_orgs"`
	HostInit            HostInitConfig            `yaml:"hostinit" bson:"hostinit" json:"hostinit" id:"hostinit"`
	HostJasper          HostJasperConfig          `yaml:"host_jasper" bson:"host_jasper" json:"host_jasper" id:"host_jasper"`
	Jira                JiraConfig                `yaml:"jira" bson:"jira" json:"jira" id:"jira"`
	JIRANotifications   JIRANotificationsConfig   `yaml:"jira_notifications" json:"jira_notifications" bson:"jira_notifications" id:"jira_notifications"`
	Keys                map[string]string         `yaml:"keys" bson:"keys" json:"keys"`
	KeysNew             util.KeyValuePairSlice    `yaml:"keys_new" bson:"keys_new" json:"keys_new"`
	LDAPRoleMap         LDAPRoleMap               `yaml:"ldap_role_map" bson:"ldap_role_map" json:"ldap_role_map"`
	LoggerConfig        LoggerConfig              `yaml:"logger_config" bson:"logger_config" json:"logger_config" id:"logger_config"`
	LogPath             string                    `yaml:"log_path" bson:"log_path" json:"log_path"`
	NewRelic            NewRelicConfig            `yaml:"newrelic" bson:"newrelic" json:"newrelic" id:"newrelic"`
	Notify              NotifyConfig              `yaml:"notify" bson:"notify" json:"notify" id:"notify"`
	Plugins             PluginConfig              `yaml:"plugins" bson:"plugins" json:"plugins"`
	PluginsNew          util.KeyValuePairSlice    `yaml:"plugins_new" bson:"plugins_new" json:"plugins_new"`
	PodInit             PodInitConfig             `yaml:"pod_init" bson:"pod_init" json:"pod_init" id:"pod_init"`
	PprofPort           string                    `yaml:"pprof_port" bson:"pprof_port" json:"pprof_port"`
	Providers           CloudProviders            `yaml:"providers" bson:"providers" json:"providers" id:"providers"`
	RepoTracker         RepoTrackerConfig         `yaml:"repotracker" bson:"repotracker" json:"repotracker" id:"repotracker"`
	Scheduler           SchedulerConfig           `yaml:"scheduler" bson:"scheduler" json:"scheduler" id:"scheduler"`
	ServiceFlags        ServiceFlags              `bson:"service_flags" json:"service_flags" id:"service_flags" yaml:"service_flags"`
	SSHKeyDirectory     string                    `yaml:"ssh_key_directory" bson:"ssh_key_directory" json:"ssh_key_directory"`
	SSHKeyPairs         []SSHKeyPair              `yaml:"ssh_key_pairs" bson:"ssh_key_pairs" json:"ssh_key_pairs"`
	Slack               SlackConfig               `yaml:"slack" bson:"slack" json:"slack" id:"slack"`
	Splunk              send.SplunkConnectionInfo `yaml:"splunk" bson:"splunk" json:"splunk"`
	Triggers            TriggerConfig             `yaml:"triggers" bson:"triggers" json:"triggers" id:"triggers"`
	Ui                  UIConfig                  `yaml:"ui" bson:"ui" json:"ui" id:"ui"`
	Spawnhost           SpawnHostConfig           `yaml:"spawnhost" bson:"spawnhost" json:"spawnhost" id:"spawnhost"`
	ShutdownWaitSeconds int                       `yaml:"shutdown_wait_seconds" bson:"shutdown_wait_seconds" json:"shutdown_wait_seconds"`
}

func (c *Settings) SectionId() string { return ConfigDocID }

func (c *Settings) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = Settings{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

// Set saves the global fields in the configuration (i.e. those that are not
// ConfigSections).
func (c *Settings) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			apiUrlKey:             c.ApiUrl,
			bannerKey:             c.Banner,
			bannerThemeKey:        c.BannerTheme,
			clientBinariesDirKey:  c.ClientBinariesDir,
			commitQueueKey:        c.CommitQueue,
			configDirKey:          c.ConfigDir,
			credentialsKey:        c.Credentials,
			credentialsNewKey:     c.CredentialsNew,
			domainNameKey:         c.DomainName,
			expansionsKey:         c.Expansions,
			expansionsNewKey:      c.ExpansionsNew,
			githubPRCreatorOrgKey: c.GithubPRCreatorOrg,
			githubOrgsKey:         c.GithubOrgs,
			keysKey:               c.Keys,
			keysNewKey:            c.KeysNew,
			ldapRoleMapKey:        c.LDAPRoleMap,
			logPathKey:            c.LogPath,
			pprofPortKey:          c.PprofPort,
			pluginsKey:            c.Plugins,
			pluginsNewKey:         c.PluginsNew,
			splunkKey:             c.Splunk,
			sshKeyDirectoryKey:    c.SSHKeyDirectory,
			sshKeyPairsKey:        c.SSHKeyPairs,
			spawnhostKey:          c.Spawnhost,
			shutdownWaitKey:       c.ShutdownWaitSeconds,
		},
	}, options.Update().SetUpsert(true))

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

	keys := map[string]bool{}
	for _, mapping := range c.LDAPRoleMap {
		if keys[mapping.LDAPGroup] == true {
			catcher.Add(errors.Errorf("duplicate LDAP group value %s found in LDAP-role mappings", mapping.LDAPGroup))
		}
		keys[mapping.LDAPGroup] = true
	}

	if len(c.SSHKeyPairs) != 0 && c.SSHKeyDirectory == "" {
		catcher.New("cannot use SSH key pairs without setting a directory for them")
	}

	for i := 0; i < len(c.SSHKeyPairs); i++ {
		catcher.NewWhen(c.SSHKeyPairs[i].Name == "", "must specify a name for SSH key pairs")
		catcher.ErrorfWhen(c.SSHKeyPairs[i].Public == "", "must specify a public key for SSH key pair '%s'", c.SSHKeyPairs[i].Name)
		catcher.ErrorfWhen(c.SSHKeyPairs[i].Private == "", "must specify a private key for SSH key pair '%s'", c.SSHKeyPairs[i].Name)
		// Avoid overwriting the filepath stored in Keys, which is a special
		// case for the path to the legacy SSH identity file.
		for _, key := range c.Keys {
			catcher.ErrorfWhen(c.SSHKeyPairs[i].PrivatePath(c) == key, "cannot overwrite the legacy SSH key '%s'", key)
		}

		// ValidateAndDefault can be called before the environment has been
		// initialized.
		if env := GetEnvironment(); env != nil {
			// Ensure we are not modify any existing keys.
			if settings := env.Settings(); settings != nil {
				for _, key := range env.Settings().SSHKeyPairs {
					if key.Name == c.SSHKeyPairs[i].Name {
						catcher.ErrorfWhen(c.SSHKeyPairs[i].Public != key.Public, "cannot modify public key for existing SSH key pair '%s'", key.Name)
						catcher.ErrorfWhen(c.SSHKeyPairs[i].Private != key.Private, "cannot modify private key for existing SSH key pair '%s'", key.Name)
					}
				}
			}
		}
		if c.SSHKeyPairs[i].EC2Regions == nil {
			c.SSHKeyPairs[i].EC2Regions = []string{}
		}
	}
	// ValidateAndDefault can be called before the environment has been
	// initialized.
	if env := GetEnvironment(); env != nil {
		if settings := env.Settings(); settings != nil {
			// Ensure we are not deleting any existing keys.
			for _, key := range GetEnvironment().Settings().SSHKeyPairs {
				var found bool
				for _, newKey := range c.SSHKeyPairs {
					if newKey.Name == key.Name {
						found = true
						break
					}
				}
				catcher.ErrorfWhen(!found, "cannot find existing SSH key '%s'", key.Name)
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
	if c.ShutdownWaitSeconds < 0 {
		c.ShutdownWaitSeconds = DefaultShutdownWaitSeconds
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
// present in the DB, it will return the defaults.
func GetConfig() (*Settings, error) { return BootstrapConfig(GetEnvironment()) }

// Bootstrap config gets a config from the database defined in the environment.
func BootstrapConfig(env Environment) (*Settings, error) {
	config := &Settings{}

	// retrieve the root config document
	if err := config.Get(env); err != nil {
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
		if err := section.Get(env); err != nil {
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

func (s *Settings) GetSender(ctx context.Context, env Environment) (send.Sender, error) {
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
	if s.Splunk.Populated() {
		retryConf := utility.NewDefaultHTTPRetryConf()
		retryConf.MaxDelay = time.Second
		retryConf.BaseDelay = 10 * time.Millisecond
		retryConf.MaxRetries = 10
		client := utility.GetHTTPRetryableClient(retryConf)

		sender, err = send.NewSplunkLoggerWithClient("", s.Splunk, grip.GetSender().Level(), client)
		if err == nil {
			if err = sender.SetLevel(levelInfo); err != nil {
				utility.PutHTTPClient(client)
				return nil, errors.Wrap(err, "problem setting level")
			}
			if err = sender.SetErrorHandler(send.ErrorHandlerFromSender(fallback)); err != nil {
				utility.PutHTTPClient(client)
				return nil, errors.Wrap(err, "problem setting error handler")
			}
			senders = append(senders,
				send.NewBufferedSender(sender,
					time.Duration(s.LoggerConfig.Buffer.DurationSeconds)*time.Second,
					s.LoggerConfig.Buffer.Count))

			env.RegisterCloser("splunk-http-client", false, func(_ context.Context) error {
				utility.PutHTTPClient(client)
				return nil
			})
		} else {
			utility.PutHTTPClient(client)
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

			senders = append(senders, logger.MakeQueueSender(ctx, env.LocalQueue(), sender))
		}
		grip.Warning(errors.Wrap(err, "problem setting up slack alert logger"))
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (s *Settings) GetGithubOauthStrings() ([]string, error) {
	var tokens []string
	var token_name string

	token, ok := s.Credentials["github"]
	if ok && token != "" {
		// we want to make sure tokens[0] is always the default token
		tokens = append(tokens, token)
	} else {
		return nil, errors.New("no 'github' token in settings")
	}

	for i := 1; i < 10; i++ {
		token_name = fmt.Sprintf("github_alt%d", i)
		token, ok := s.Credentials[token_name]
		if ok && token != "" {
			tokens = append(tokens, token)
		} else {
			break
		}
	}
	return tokens, nil
}

func (s *Settings) GetGithubOauthToken() (string, error) {
	if s == nil {
		return "", errors.New("not defined")
	}

	oauthStrings, err := s.GetGithubOauthStrings()
	if err != nil {
		return "", err
	}
	timeSeed := time.Now().Nanosecond()
	randomStartIdx := timeSeed % len(oauthStrings)
	var oauthString string
	for i := range oauthStrings {
		oauthString = oauthStrings[randomStartIdx+i]
		splitToken, err := splitToken(oauthString)
		if err != nil {
			grip.Error(message.Fields{
				"error":   err,
				"message": fmt.Sprintf("problem with github_alt%d", i)})
		} else {
			return splitToken, nil
		}
	}
	return "", errors.New("all github tokens are malformatted. Proper format is github:token <token> or github_alt#:token <token>")
}

func splitToken(oauthString string) (string, error) {
	splitToken := strings.Split(oauthString, " ")
	if len(splitToken) != 2 || splitToken[0] != "token" {
		return "", errors.New("token format was invalid, expected 'token [token]'")
	}
	return splitToken[1], nil
}
func GetServiceFlags() (*ServiceFlags, error) {
	flags := &ServiceFlags{}
	if err := flags.Get(GetEnvironment()); err != nil {
		return nil, errors.Wrap(err, "error retrieving section from DB")
	}
	return flags, nil
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

// SSHKeyPair represents a public and private SSH key pair.
type SSHKeyPair struct {
	Name    string `bson:"name" json:"name" yaml:"name"`
	Public  string `bson:"public" json:"public" yaml:"public"`
	Private string `bson:"private" json:"private" yaml:"private"`
	// EC2Regions contains all EC2 regions that have stored this SSH key.
	EC2Regions []string `bson:"ec2_regions" json:"ec2_regions" yaml:"ec2_regions"`
}

// AddEC2Region adds the given EC2 region to the set of regions containing the
// SSH key.
func (p *SSHKeyPair) AddEC2Region(region string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	query := bson.M{
		idKey: ConfigDocID,
		sshKeyPairsKey: bson.M{
			"$elemMatch": bson.M{
				sshKeyPairNameKey: p.Name,
			},
		},
	}
	var update bson.M
	if len(p.EC2Regions) == 0 {
		// In case this is the first element, we have to push to create the
		// array first.
		update = bson.M{
			"$push": bson.M{bsonutil.GetDottedKeyName(sshKeyPairsKey, "$", sshKeyPairEC2RegionsKey): region},
		}
	} else {
		update = bson.M{
			"$addToSet": bson.M{bsonutil.GetDottedKeyName(sshKeyPairsKey, "$", sshKeyPairEC2RegionsKey): region},
		}
	}
	if _, err := coll.UpdateOne(ctx, query, update); err != nil {
		return errors.WithStack(err)
	}

	if !utility.StringSliceContains(p.EC2Regions, region) {
		p.EC2Regions = append(p.EC2Regions, region)
	}

	return nil
}

func (p *SSHKeyPair) PrivatePath(settings *Settings) string {
	return filepath.Join(settings.SSHKeyDirectory, p.Name)
}

type WriteConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	J        bool   `yaml:"j"`
}

func (wc WriteConcern) Resolve() *writeconcern.WriteConcern {
	opts := []writeconcern.Option{}

	if wc.J {
		opts = append(opts, writeconcern.J(true))
	}
	if wc.WMode == "majority" {
		opts = append(opts, writeconcern.WMajority())
	} else if wc.W > 0 {
		opts = append(opts, writeconcern.W(wc.W))
	}

	if wc.WTimeout > 0 {
		opts = append(opts, writeconcern.WTimeout(time.Duration(wc.WTimeout)*time.Millisecond))
	}

	return writeconcern.New().WithOptions(opts...)
}

type ReadConcern struct {
	Level string `yaml:"level"`
}

func (rc ReadConcern) Resolve() *readconcern.ReadConcern {

	if rc.Level == "majority" {
		return readconcern.Majority()
	} else if rc.Level == "local" {
		return readconcern.Local()
	} else if rc.Level == "" {
		return readconcern.Majority()
	} else {
		grip.Error(message.Fields{
			"error":   "ReadConcern Level is not majority or local, setting to majority",
			"rcLevel": rc.Level})
		return readconcern.Majority()
	}
}

type DBSettings struct {
	Url                  string       `yaml:"url"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings WriteConcern `yaml:"write_concern"`
	ReadConcernSettings  ReadConcern  `yaml:"read_concern"`
	AuthFile             string       `yaml:"auth_file"`
}

func (dbs *DBSettings) HasAuth() bool {
	return dbs.AuthFile != ""
}

type dbCreds struct {
	DBUser string `yaml:"mdb_database_username"`
	DBPwd  string `yaml:"mdb_database_password"`
}

func (dbs *DBSettings) GetAuth() (string, string, error) {
	return GetAuthFromYAML(dbs.AuthFile)
}

func GetAuthFromYAML(authFile string) (string, string, error) {
	creds := &dbCreds{}

	file, err := os.Open(authFile)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)

	if err := decoder.Decode(&creds); err != nil {
		return "", "", err
	}

	return creds.DBUser, creds.DBPwd, nil
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
