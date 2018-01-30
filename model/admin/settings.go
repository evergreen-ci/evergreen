package admin

import (
	"fmt"
	"reflect"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// Config is a struct that holds Evergreen-wide configuration settings
type Config struct {
	Id                 string                      `bson:"_id" json:"id"`
	Alerts             evergreen.AlertsConfig      `bson:"alerts" json:"alerts"`
	Amboy              evergreen.AmboyConfig       `bson:"amboy" json:"amboy"`
	Api                evergreen.APIConfig         `bson:"api" json:"api"`
	ApiUrl             string                      `bson:"api_url" json:"api_url"`
	AuthConfig         evergreen.AuthConfig        `bson:"auth" json:"auth"`
	Banner             string                      `bson:"banner" json:"banner"`
	BannerTheme        BannerTheme                 `bson:"banner_theme" json:"banner_theme"`
	ClientBinariesDir  string                      `bson:"client_binaries_dir" json:"client_binaries_dir"`
	ConfigDir          string                      `bson:"configdir" json:"configdir"`
	Credentials        map[string]string           `bson:"credentials" json:"credentials"`
	Expansions         map[string]string           `bson:"expansions" json:"expansions"`
	GithubPRCreatorOrg string                      `bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	HostInit           evergreen.HostInitConfig    `bson:"hostinit" json:"hostinit"`
	IsNonProd          bool                        `bson:"isnonprod" json:"isnonprod"`
	Jira               evergreen.JiraConfig        `bson:"jira" json:"jira"`
	Keys               map[string]string           `bson:"keys" json:"keys"`
	LoggerConfig       evergreen.LoggerConfig      `bson:"logger_config" json:"logger_config"`
	LogPath            string                      `bson:"log_path" json:"log_path"`
	NewRelic           evergreen.NewRelicConfig    `bson:"new_relic" json:"new_relic"`
	Notify             evergreen.NotifyConfig      `bson:"notify" json:"notify"`
	Plugins            evergreen.PluginConfig      `bson:"plugins" json:"plugins"`
	PprofPort          string                      `bson:"pprof_port" json:"pprof_port"`
	Providers          evergreen.CloudProviders    `bson:"providers" json:"providers"`
	RepoTracker        evergreen.RepoTrackerConfig `bson:"repotracker" json:"repotracker"`
	Scheduler          evergreen.SchedulerConfig   `bson:"scheduler" json:"scheduler"`
	ServiceFlags       ServiceFlags                `bson:"service_flags" json:"service_flags" id:"service_flags"`
	Slack              evergreen.SlackConfig       `bson:"slack" json:"slack"`
	Splunk             send.SplunkConnectionInfo   `bson:"splunk" json:"splunk"`
	SuperUsers         []string                    `bson:"superusers" json:"superusers"`
	Ui                 evergreen.UIConfig          `bson:"ui" json:"ui"`
}

func (c *Config) id() string { return configDocID }
func (c *Config) get() error {
	err := db.FindOneQ(Collection, db.Query(byId(c.id())), c)
	if err != nil && err.Error() == "not found" {
		return nil
	}
	return err
}
func (c *Config) set() error {
	return nil
}

// configSection defines a sub-document in the evegreen configSection
// any config sections must also be added to registry.go
type configSection interface {
	// id() returns the ID of the section to be used in the database document and struct tag
	id() string
	// get() populates the section from the DB
	get() error
	// set() upserts the section document into the DB
	set() error
}

// GetSettings retrieves the Evergreen config document. If no document is
// present in the DB, it will return the defaults
func GetConfig() (*Config, error) {
	config := &Config{}

	// retrieve the root config document
	if err := config.get(); err != nil {
		return nil, err
	}
	grip.Info(config)

	// retrieve the other config sub-documents and form the whole struct
	catcher := grip.NewSimpleCatcher()
	sections := registry.getSections()
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
		if err := section.get(); err != nil {
			catcher.Add(errors.Wrapf(err, "error populating section %s", sectionId))
			continue
		}

		// set the value of the section struct to the value of the corresponding field in the config
		sectionVal := reflect.ValueOf(section).Elem()
		propVal := reflect.ValueOf(config).Elem().FieldByName(propName)
		if !propVal.CanSet() {
			catcher.Add(fmt.Errorf("unable to set field %s in %s", propName, sectionId))
		}
		propVal.Set(sectionVal)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return config, nil
}
