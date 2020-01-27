package model

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

func NewConfigModel() *APIAdminSettings {
	return &APIAdminSettings{
		Alerts:            &APIAlertsConfig{},
		Amboy:             &APIAmboyConfig{},
		Api:               &APIapiConfig{},
		AuthConfig:        &APIAuthConfig{},
		Backup:            &APIBackupConfig{},
		CommitQueue:       &APICommitQueueConfig{},
		ContainerPools:    &APIContainerPoolsConfig{},
		Credentials:       map[string]string{},
		Expansions:        map[string]string{},
		HostInit:          &APIHostInitConfig{},
		HostJasper:        &APIHostJasperConfig{},
		Jira:              &APIJiraConfig{},
		JIRANotifications: &APIJIRANotificationsConfig{},
		Keys:              map[string]string{},
		LDAPRoleMap:       &APILDAPRoleMap{},
		LoggerConfig:      &APILoggerConfig{},
		NewRelic:          &APINewRelicConfig{},
		Notify:            &APINotifyConfig{},
		Plugins:           map[string]map[string]interface{}{},
		Providers:         &APICloudProviders{},
		RepoTracker:       &APIRepoTrackerConfig{},
		Scheduler:         &APISchedulerConfig{},
		ServiceFlags:      &APIServiceFlags{},
		Slack:             &APISlackConfig{},
		Splunk:            &APISplunkConnectionInfo{},
		Triggers:          &APITriggerConfig{},
		Ui:                &APIUIConfig{},
	}
}

// APIAdminSettings is the structure of a response to the admin route
type APIAdminSettings struct {
	ACLCheckingEnabled      *bool                             `json:"acl_enabled,omitempty"`
	Alerts                  *APIAlertsConfig                  `json:"alerts,omitempty"`
	Amboy                   *APIAmboyConfig                   `json:"amboy,omitempty"`
	Api                     *APIapiConfig                     `json:"api,omitempty"`
	ApiUrl                  *string                           `json:"api_url,omitempty"`
	AuthConfig              *APIAuthConfig                    `json:"auth,omitempty"`
	Banner                  *string                           `json:"banner,omitempty"`
	BannerTheme             *string                           `json:"banner_theme,omitempty"`
	Backup                  *APIBackupConfig                  `json:"backup,omitempty"`
	ClientBinariesDir       *string                           `json:"client_binaries_dir,omitempty"`
	CommitQueue             *APICommitQueueConfig             `json:"commit_queue,omitempty"`
	ConfigDir               *string                           `json:"configdir,omitempty"`
	ContainerPools          *APIContainerPoolsConfig          `json:"container_pools,omitempty"`
	Credentials             map[string]string                 `json:"credentials,omitempty"`
	DomainName              *string                           `json:"domain_name,omitempty"`
	Expansions              map[string]string                 `json:"expansions,omitempty"`
	Bugsnag                 *string                           `json:"bugsnag,omitempty"`
	GithubPRCreatorOrg      *string                           `json:"github_pr_creator_org,omitempty"`
	GithubOrgs              []string                          `json:"github_orgs,omitempty"`
	HostInit                *APIHostInitConfig                `json:"hostinit,omitempty"`
	HostJasper              *APIHostJasperConfig              `json:"host_jasper,omitempty"`
	Jira                    *APIJiraConfig                    `json:"jira,omitempty"`
	JIRANotifications       *APIJIRANotificationsConfig       `json:"jira_notifications,omitempty"`
	Keys                    map[string]string                 `json:"keys,omitempty"`
	LDAPRoleMap             *APILDAPRoleMap                   `json:"ldap_role_map,omitempty"`
	LoggerConfig            *APILoggerConfig                  `json:"logger_config,omitempty"`
	LogPath                 *string                           `json:"log_path,omitempty"`
	NewRelic                *APINewRelicConfig                `json:"newrelic,omitempty"`
	Notify                  *APINotifyConfig                  `json:"notify,omitempty"`
	Plugins                 map[string]map[string]interface{} `json:"plugins,omitempty"`
	PprofPort               *string                           `json:"pprof_port,omitempty"`
	Providers               *APICloudProviders                `json:"providers,omitempty"`
	RepoTracker             *APIRepoTrackerConfig             `json:"repotracker,omitempty"`
	Scheduler               *APISchedulerConfig               `json:"scheduler,omitempty"`
	ServiceFlags            *APIServiceFlags                  `json:"service_flags,omitempty"`
	Slack                   *APISlackConfig                   `json:"slack,omitempty"`
	SpawnHostsPerUser       *int                              `json:"spawn_hosts_per_user"`
	Splunk                  *APISplunkConnectionInfo          `json:"splunk,omitempty"`
	SuperUsers              []string                          `json:"superusers,omitempty"`
	Triggers                *APITriggerConfig                 `json:"triggers,omitempty"`
	Ui                      *APIUIConfig                      `json:"ui,omitempty"`
	UnexpirableHostsPerUser *int                              `json:"unexpirable_hosts_per_user"`
}

// BuildFromService builds a model from the service layer
func (as *APIAdminSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.Settings:
		if v == nil {
			return errors.New("evergreen settings object is nil")
		}
		apiModelReflect := reflect.ValueOf(*as)
		dbModelReflect := reflect.ValueOf(*v)
		for i := 0; i < apiModelReflect.NumField(); i++ {
			propName := apiModelReflect.Type().Field(i).Name
			val := apiModelReflect.FieldByName(propName)
			if val.IsNil() {
				continue
			}

			// check to see if this property is an API model itself
			interfaceVal := val.Interface()
			model, ok := interfaceVal.(Model)
			if !ok {
				continue
			}
			// build the sub-model from the DB model. assumes that the 2 fields are named the same thing
			if err := model.BuildFromService(dbModelReflect.FieldByName(propName).Interface()); err != nil {
				return errors.Wrapf(err, "error converting model section %s", propName)
			}
		}
		as.ACLCheckingEnabled = &v.ACLCheckingEnabled
		as.ApiUrl = &v.ApiUrl
		as.Banner = &v.Banner
		tmp := string(v.BannerTheme)
		as.BannerTheme = &tmp
		as.ClientBinariesDir = &v.ClientBinariesDir
		as.ConfigDir = &v.ConfigDir
		as.DomainName = ToStringPtr(v.DomainName)
		as.Bugsnag = ToStringPtr(v.Bugsnag)
		as.GithubPRCreatorOrg = &v.GithubPRCreatorOrg
		as.LogPath = &v.LogPath
		as.Plugins = v.Plugins
		as.PprofPort = &v.PprofPort
		as.Credentials = v.Credentials
		as.Expansions = v.Expansions
		as.Keys = v.Keys
		as.SuperUsers = v.SuperUsers
		as.GithubOrgs = v.GithubOrgs
		as.UnexpirableHostsPerUser = &v.UnexpirableHostsPerUser
		as.SpawnHostsPerUser = &v.SpawnHostsPerUser
	default:
		return errors.Errorf("%T is not a supported admin settings type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIAdminSettings) ToService() (interface{}, error) {
	settings := evergreen.Settings{
		Credentials:             map[string]string{},
		Expansions:              map[string]string{},
		Keys:                    map[string]string{},
		Plugins:                 evergreen.PluginConfig{},
		SuperUsers:              as.SuperUsers,
		GithubOrgs:              as.GithubOrgs,
		SpawnHostsPerUser:       cloud.DefaultMaxSpawnHostsPerUser,
		UnexpirableHostsPerUser: host.DefaultUnexpirableHostsPerUser,
	}
	if as.ACLCheckingEnabled != nil {
		settings.ACLCheckingEnabled = *as.ACLCheckingEnabled
	}
	if as.ApiUrl != nil {
		settings.ApiUrl = *as.ApiUrl
	}
	if as.Banner != nil {
		settings.Banner = *as.Banner
	}
	if as.BannerTheme != nil {
		settings.BannerTheme = evergreen.BannerTheme(*as.BannerTheme)
	}
	if as.ClientBinariesDir != nil {
		settings.ClientBinariesDir = *as.ClientBinariesDir
	}
	if as.ConfigDir != nil {
		settings.ConfigDir = *as.ConfigDir
	}
	settings.DomainName = FromStringPtr(as.DomainName)
	settings.Bugsnag = FromStringPtr(as.Bugsnag)

	if as.GithubPRCreatorOrg != nil {
		settings.GithubPRCreatorOrg = *as.GithubPRCreatorOrg
	}
	if as.LogPath != nil {
		settings.LogPath = *as.LogPath
	}
	if as.PprofPort != nil {
		settings.PprofPort = *as.PprofPort
	}
	if as.SpawnHostsPerUser != nil {
		settings.SpawnHostsPerUser = *as.SpawnHostsPerUser
	}
	if as.UnexpirableHostsPerUser != nil {
		settings.UnexpirableHostsPerUser = *as.UnexpirableHostsPerUser
	}

	apiModelReflect := reflect.ValueOf(*as)
	dbModelReflect := reflect.ValueOf(&settings).Elem()
	for i := 0; i < apiModelReflect.NumField(); i++ {
		propName := apiModelReflect.Type().Field(i).Name
		val := apiModelReflect.FieldByName(propName)
		if val.IsNil() {
			continue
		}

		// check to see if this property is an API model itself
		interfaceVal := val.Interface()
		model, ok := interfaceVal.(Model)
		if !ok {
			continue
		}
		// set the corresponding DB model field. assumes that the 2 fields are named the same thing
		i, err := model.ToService()
		if err != nil {
			return nil, errors.Wrapf(err, "error converting model section %s", propName)
		}
		valToSet := reflect.ValueOf(i)
		dbModelReflect.FieldByName(propName).Set(valToSet)
	}
	for k, v := range as.Credentials {
		settings.Credentials[k] = v
	}
	for k, v := range as.Expansions {
		settings.Expansions[k] = v
	}
	for k, v := range as.Keys {
		settings.Keys[k] = v
	}
	for k, v := range as.Plugins {
		settings.Plugins[k] = map[string]interface{}{}
		for k2, v2 := range v {
			settings.Plugins[k][k2] = v2
		}
	}
	return settings, nil
}

type APIAlertsConfig struct {
	SMTP APISMTPConfig `json:"smtp"`
}

func (a *APIAlertsConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AlertsConfig:
		if err := a.SMTP.BuildFromService(v.SMTP); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAlertsConfig) ToService() (interface{}, error) {
	smtp, err := a.SMTP.ToService()
	if err != nil {
		return nil, err
	}
	return evergreen.AlertsConfig{
		SMTP: smtp.(evergreen.SMTPConfig),
	}, nil
}

type APISMTPConfig struct {
	Server     *string   `json:"server"`
	Port       int       `json:"port"`
	UseSSL     bool      `json:"use_ssl"`
	Username   *string   `json:"username"`
	Password   *string   `json:"password"`
	From       *string   `json:"from"`
	AdminEmail []*string `json:"admin_email"`
}

func (a *APISMTPConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.SMTPConfig:
		a.Server = ToStringPtr(v.Server)
		a.Port = v.Port
		a.UseSSL = v.UseSSL
		a.Username = ToStringPtr(v.Username)
		a.Password = ToStringPtr(v.Password)
		a.From = ToStringPtr(v.From)
		for _, s := range v.AdminEmail {
			a.AdminEmail = append(a.AdminEmail, ToStringPtr(s))
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISMTPConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.SMTPConfig{
		Server:   FromStringPtr(a.Server),
		Port:     a.Port,
		UseSSL:   a.UseSSL,
		Username: FromStringPtr(a.Username),
		Password: FromStringPtr(a.Password),
		From:     FromStringPtr(a.From),
	}
	for _, s := range a.AdminEmail {
		config.AdminEmail = append(config.AdminEmail, FromStringPtr(s))
	}
	return config, nil
}

type APIAmboyConfig struct {
	Name                                  *string `json:"name"`
	SingleName                            *string `json:"single_name"`
	DB                                    *string `json:"database"`
	PoolSizeLocal                         int     `json:"pool_size_local"`
	PoolSizeRemote                        int     `json:"pool_size_remote"`
	LocalStorage                          int     `json:"local_storage_size"`
	GroupDefaultWorkers                   int     `json:"group_default_workers"`
	GroupBackgroundCreateFrequencyMinutes int     `json:"group_background_create_frequency"`
	GroupPruneFrequencyMinutes            int     `json:"group_prune_frequency"`
	GroupTTLMinutes                       int     `json:"group_ttl"`
}

func (a *APIAmboyConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AmboyConfig:
		a.Name = ToStringPtr(v.Name)
		a.SingleName = ToStringPtr(v.SingleName)
		a.DB = ToStringPtr(v.DB)
		a.PoolSizeLocal = v.PoolSizeLocal
		a.PoolSizeRemote = v.PoolSizeRemote
		a.LocalStorage = v.LocalStorage
		a.GroupDefaultWorkers = v.GroupDefaultWorkers
		a.GroupBackgroundCreateFrequencyMinutes = v.GroupBackgroundCreateFrequencyMinutes
		a.GroupPruneFrequencyMinutes = v.GroupPruneFrequencyMinutes
		a.GroupTTLMinutes = v.GroupTTLMinutes
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAmboyConfig) ToService() (interface{}, error) {
	return evergreen.AmboyConfig{
		Name:                                  FromStringPtr(a.Name),
		SingleName:                            FromStringPtr(a.SingleName),
		DB:                                    FromStringPtr(a.DB),
		PoolSizeLocal:                         a.PoolSizeLocal,
		PoolSizeRemote:                        a.PoolSizeRemote,
		LocalStorage:                          a.LocalStorage,
		GroupDefaultWorkers:                   a.GroupDefaultWorkers,
		GroupBackgroundCreateFrequencyMinutes: a.GroupBackgroundCreateFrequencyMinutes,
		GroupPruneFrequencyMinutes:            a.GroupPruneFrequencyMinutes,
		GroupTTLMinutes:                       a.GroupTTLMinutes,
	}, nil
}

type APIapiConfig struct {
	HttpListenAddr      *string `json:"http_listen_addr"`
	GithubWebhookSecret *string `json:"github_webhook_secret"`
}

func (a *APIapiConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.APIConfig:
		a.HttpListenAddr = ToStringPtr(v.HttpListenAddr)
		a.GithubWebhookSecret = ToStringPtr(v.GithubWebhookSecret)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIapiConfig) ToService() (interface{}, error) {
	return evergreen.APIConfig{
		HttpListenAddr:      FromStringPtr(a.HttpListenAddr),
		GithubWebhookSecret: FromStringPtr(a.GithubWebhookSecret),
	}, nil
}

type APIAuthConfig struct {
	LDAP          *APILDAPConfig       `json:"ldap"`
	Okta          *APIOktaConfig       `json:"okta"`
	Naive         *APINaiveAuthConfig  `json:"naive"`
	Github        *APIGithubAuthConfig `json:"github"`
	PreferredType *string              `json:"preferred_type"`
}

func (a *APIAuthConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AuthConfig:
		if v.LDAP != nil {
			a.LDAP = &APILDAPConfig{}
			if err := a.LDAP.BuildFromService(v.LDAP); err != nil {
				return errors.Wrap(err, "could not build API LDAP auth settings from service")
			}
		}
		if v.Okta != nil {
			a.Okta = &APIOktaConfig{}
			if err := a.Okta.BuildFromService(v.Okta); err != nil {
				return errors.Wrap(err, "could not build API Okta auth settings from service")
			}
		}
		if v.Okta != nil {
			a.Okta = &APIOktaConfig{}
			if err := a.Okta.BuildFromService(v.Okta); err != nil {
				return err
			}
		}
		if v.Github != nil {
			a.Github = &APIGithubAuthConfig{}
			if err := a.Github.BuildFromService(v.Github); err != nil {
				return errors.Wrap(err, "could not build API GitHub auth settings from service")
			}
		}
		if v.Naive != nil {
			a.Naive = &APINaiveAuthConfig{}
			if err := a.Naive.BuildFromService(v.Naive); err != nil {
				return errors.Wrap(err, "could not build API naive auth settings from service")
			}
		}
		a.PreferredType = ToStringPtr(v.PreferredType)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAuthConfig) ToService() (interface{}, error) {
	var ldap *evergreen.LDAPConfig
	var okta *evergreen.OktaConfig
	var naive *evergreen.NaiveAuthConfig
	var github *evergreen.GithubAuthConfig
	var ok bool
	i, err := a.LDAP.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		ldap, ok = i.(*evergreen.LDAPConfig)
		if !ok {
			return nil, errors.Errorf("expecting LDAPConfig but got %T", i)
		}
	}
	i, err = a.Okta.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		okta, ok = i.(*evergreen.OktaConfig)
		if !ok {
			return nil, errors.Errorf("expecting OktaConfig but got %T", i)
		}
	}
	i, err = a.Okta.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		okta = i.(*evergreen.OktaConfig)
	}
	i, err = a.Naive.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		naive, ok = i.(*evergreen.NaiveAuthConfig)
		if !ok {
			return nil, errors.Errorf("expecting NaiveAuthConfig but got %T", i)
		}
	}
	i, err = a.Github.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		github, ok = i.(*evergreen.GithubAuthConfig)
		if !ok {
			return nil, errors.Errorf("expecting GithubAuthConfig but got %T", i)
		}
	}
	return evergreen.AuthConfig{
		LDAP:          ldap,
		Okta:          okta,
		Naive:         naive,
		Github:        github,
		PreferredType: FromStringPtr(a.PreferredType),
	}, nil
}

type APIBackupConfig struct {
	BucketName *string `bson:"bucket_name" json:"bucket_name" yaml:"bucket_name"`
	Key        *string `bson:"key" json:"key" yaml:"key"`
	Secret     *string `bson:"secret" json:"secret" yaml:"secret"`
	Prefix     *string `bson:"prefix" json:"prefix" yaml:"prefix"`
	Compress   bool    `bson:"compress" json:"compress" yaml:"compress"`
}

func (a *APIBackupConfig) BuildFromService(c interface{}) error {
	switch conf := c.(type) {
	case evergreen.BackupConfig:
		a.BucketName = ToStringPtr(conf.BucketName)
		a.Key = ToStringPtr(conf.Key)
		a.Secret = ToStringPtr(conf.Secret)
		a.Compress = conf.Compress
		a.Prefix = ToStringPtr(conf.Prefix)

		return nil
	default:
		return errors.Errorf("%T is not a supported type", c)
	}
}
func (a *APIBackupConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}

	return evergreen.BackupConfig{
		BucketName: FromStringPtr(a.BucketName),
		Key:        FromStringPtr(a.Key),
		Secret:     FromStringPtr(a.Secret),
		Prefix:     FromStringPtr(a.Prefix),
		Compress:   a.Compress,
	}, nil
}

type APILDAPConfig struct {
	URL                *string `json:"url"`
	Port               *string `json:"port"`
	UserPath           *string `json:"path"`
	ServicePath        *string `json:"service_path"`
	Group              *string `json:"group"`
	ServiceGroup       *string `json:"service_group"`
	ExpireAfterMinutes *string `json:"expire_after_minutes"`
	GroupOU            *string `json:"group_ou"`
}

func (a *APILDAPConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.LDAPConfig:
		if v == nil {
			return nil
		}
		a.URL = ToStringPtr(v.URL)
		a.Port = ToStringPtr(v.Port)
		a.UserPath = ToStringPtr(v.UserPath)
		a.ServicePath = ToStringPtr(v.ServicePath)
		a.Group = ToStringPtr(v.Group)
		a.ServiceGroup = ToStringPtr(v.ServiceGroup)
		a.ExpireAfterMinutes = ToStringPtr(v.ExpireAfterMinutes)
		a.GroupOU = ToStringPtr(v.GroupOU)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APILDAPConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.LDAPConfig{
		URL:                FromStringPtr(a.URL),
		Port:               FromStringPtr(a.Port),
		UserPath:           FromStringPtr(a.UserPath),
		ServicePath:        FromStringPtr(a.ServicePath),
		Group:              FromStringPtr(a.Group),
		ServiceGroup:       FromStringPtr(a.ServiceGroup),
		ExpireAfterMinutes: FromStringPtr(a.ExpireAfterMinutes),
		GroupOU:            FromStringPtr(a.Group),
	}, nil
}

type APIOktaConfig struct {
	ClientID           *string `json:"client_id"`
	ClientSecret       *string `json:"client_secret"`
	Issuer             *string `json:"issuer"`
	UserGroup          *string `json:"user_group"`
	ExpireAfterMinutes int     `json:"expire_after_minutes"`
}

func (a *APIOktaConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.OktaConfig:
		if v == nil {
			return nil
		}
		a.ClientID = ToStringPtr(v.ClientID)
		a.ClientSecret = ToStringPtr(v.ClientSecret)
		a.Issuer = ToStringPtr(v.Issuer)
		a.UserGroup = ToStringPtr(v.UserGroup)
		a.ExpireAfterMinutes = v.ExpireAfterMinutes
		return nil
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
}

func (a *APIOktaConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.OktaConfig{
		ClientID:           FromStringPtr(a.ClientID),
		ClientSecret:       FromStringPtr(a.ClientSecret),
		Issuer:             FromStringPtr(a.Issuer),
		UserGroup:          FromStringPtr(a.UserGroup),
		ExpireAfterMinutes: a.ExpireAfterMinutes,
	}, nil
}

type APINaiveAuthConfig struct {
	Users []*APIAuthUser `json:"users"`
}

func (a *APINaiveAuthConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.NaiveAuthConfig:
		if v == nil {
			return nil
		}
		for _, u := range v.Users {
			APIuser := &APIAuthUser{}
			if err := APIuser.BuildFromService(u); err != nil {
				return err
			}
			a.Users = append(a.Users, APIuser)
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APINaiveAuthConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.NaiveAuthConfig{}
	for _, u := range a.Users {
		i, err := u.ToService()
		if err != nil {
			return nil, err
		}
		user := i.(*evergreen.AuthUser)
		config.Users = append(config.Users, user)
	}
	return &config, nil
}

type APIAuthUser struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	Password    *string `json:"password"`
	Email       *string `json:"email"`
}

func (a *APIAuthUser) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.AuthUser:
		if v == nil {
			return nil
		}
		a.Username = ToStringPtr(v.Username)
		a.Password = ToStringPtr(v.Password)
		a.DisplayName = ToStringPtr(v.DisplayName)
		a.Email = ToStringPtr(v.Email)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAuthUser) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.AuthUser{
		Username:    FromStringPtr(a.Username),
		Password:    FromStringPtr(a.Password),
		DisplayName: FromStringPtr(a.DisplayName),
		Email:       FromStringPtr(a.Email),
	}, nil
}

type APIGithubAuthConfig struct {
	ClientId     *string   `json:"client_id"`
	ClientSecret *string   `json:"client_secret"`
	Users        []*string `json:"users"`
	Organization *string   `json:"organization"`
}

func (a *APIGithubAuthConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.GithubAuthConfig:
		if v == nil {
			return nil
		}
		a.ClientId = ToStringPtr(v.ClientId)
		a.ClientSecret = ToStringPtr(v.ClientSecret)
		a.Organization = ToStringPtr(v.Organization)
		for _, u := range v.Users {
			a.Users = append(a.Users, ToStringPtr(u))
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIGithubAuthConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.GithubAuthConfig{
		ClientId:     FromStringPtr(a.ClientId),
		ClientSecret: FromStringPtr(a.ClientSecret),
		Organization: FromStringPtr(a.Organization),
	}
	for _, u := range a.Users {
		config.Users = append(config.Users, FromStringPtr(u))
	}
	return &config, nil
}

// APIBanner is a public structure representing the banner part of the admin settings
type APIBanner struct {
	Text  *string `json:"banner"`
	Theme *string `json:"theme"`
}

type APIHostInitConfig struct {
	SSHTimeoutSeconds int64 `json:"ssh_timeout_secs"`
	HostThrottle      int   `json:"host_throttle"`
}

func (a *APIHostInitConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.HostInitConfig:
		a.SSHTimeoutSeconds = v.SSHTimeoutSeconds
		a.HostThrottle = v.HostThrottle
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIHostInitConfig) ToService() (interface{}, error) {
	return evergreen.HostInitConfig{
		SSHTimeoutSeconds: a.SSHTimeoutSeconds,
		HostThrottle:      a.HostThrottle,
	}, nil
}

type APIJiraConfig struct {
	Host           *string `json:"host"`
	Username       *string `json:"username"`
	Password       *string `json:"password"`
	DefaultProject *string `json:"default_project"`
}

func (a *APIJiraConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.JiraConfig:
		a.Host = ToStringPtr(v.Host)
		a.Username = ToStringPtr(v.Username)
		a.Password = ToStringPtr(v.Password)
		a.DefaultProject = ToStringPtr(v.DefaultProject)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIJiraConfig) ToService() (interface{}, error) {
	return evergreen.JiraConfig{
		Host:           FromStringPtr(a.Host),
		Username:       FromStringPtr(a.Username),
		Password:       FromStringPtr(a.Password),
		DefaultProject: FromStringPtr(a.DefaultProject),
	}, nil
}

type APILDAPRoleMapping struct {
	LDAPGroup *string `json:"ldap_group"`
	RoleID    *string ` json:"role_id"`
}

func (a *APILDAPRoleMapping) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.LDAPRoleMapping:
		a.LDAPGroup = ToStringPtr(v.LDAPGroup)
		a.RoleID = ToStringPtr(v.RoleID)
	}

	return nil
}

func (a *APILDAPRoleMapping) ToService() (interface{}, error) {
	mapping := evergreen.LDAPRoleMapping{
		LDAPGroup: FromStringPtr(a.LDAPGroup),
		RoleID:    FromStringPtr(a.RoleID),
	}

	return mapping, nil
}

type APILDAPRoleMap []APILDAPRoleMapping

func (a *APILDAPRoleMap) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.LDAPRoleMap:
		m := make(APILDAPRoleMap, len(v))
		for i := range v {
			if err := m[i].BuildFromService(v[i]); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *APILDAPRoleMap) ToService() (interface{}, error) {
	serviceMap := make(evergreen.LDAPRoleMap, len(*a))
	for i := range *a {
		v, err := (*a)[i].ToService()
		if err != nil {
			return nil, err
		}
		serviceMap[i] = v.(evergreen.LDAPRoleMapping)
	}

	return serviceMap, nil
}

type APILoggerConfig struct {
	Buffer              *APILogBuffering `json:"buffer"`
	DefaultLevel        *string          `json:"default_level"`
	ThresholdLevel      *string          `json:"threshold_level"`
	LogkeeperURL        *string          `json:"logkeeper_url"`
	BuildloggerBaseURL  *string          `json:"buildlogger_base_url"`
	BuildloggerRPCPort  *string          `json:"buildlogger_rpc_port"`
	BuildloggerUser     *string          `json:"buildlogger_user"`
	BuildloggerPassword *string          `json:"buildlogger_password"`
}

func (a *APILoggerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.LoggerConfig:
		a.DefaultLevel = ToStringPtr(v.DefaultLevel)
		a.ThresholdLevel = ToStringPtr(v.ThresholdLevel)
		a.LogkeeperURL = ToStringPtr(v.LogkeeperURL)
		a.BuildloggerBaseURL = ToStringPtr(v.BuildloggerBaseURL)
		a.BuildloggerRPCPort = ToStringPtr(v.BuildloggerRPCPort)
		a.BuildloggerUser = ToStringPtr(v.BuildloggerUser)
		a.BuildloggerPassword = ToStringPtr(v.BuildloggerPassword)
		a.Buffer = &APILogBuffering{}
		if err := a.Buffer.BuildFromService(v.Buffer); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APILoggerConfig) ToService() (interface{}, error) {
	config := evergreen.LoggerConfig{
		DefaultLevel:        FromStringPtr(a.DefaultLevel),
		ThresholdLevel:      FromStringPtr(a.ThresholdLevel),
		LogkeeperURL:        FromStringPtr(a.LogkeeperURL),
		BuildloggerBaseURL:  FromStringPtr(a.BuildloggerBaseURL),
		BuildloggerRPCPort:  FromStringPtr(a.BuildloggerRPCPort),
		BuildloggerUser:     FromStringPtr(a.BuildloggerUser),
		BuildloggerPassword: FromStringPtr(a.BuildloggerPassword),
	}
	i, err := a.Buffer.ToService()
	if err != nil {
		return nil, err
	}
	buffer := i.(evergreen.LogBuffering)
	config.Buffer = buffer
	return config, nil
}

type APILogBuffering struct {
	DurationSeconds int `json:"duration_seconds"`
	Count           int `json:"count"`
}

func (a *APILogBuffering) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.LogBuffering:
		a.DurationSeconds = v.DurationSeconds
		a.Count = v.Count
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APILogBuffering) ToService() (interface{}, error) {
	return evergreen.LogBuffering{
		DurationSeconds: a.DurationSeconds,
		Count:           a.Count,
	}, nil
}

type APINotifyConfig struct {
	BufferTargetPerInterval int           `json:"buffer_target_per_interval"`
	BufferIntervalSeconds   int           `json:"buffer_interval_seconds"`
	SMTP                    APISMTPConfig `json:"smtp"`
}

func (a *APINotifyConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.NotifyConfig:
		a.SMTP = APISMTPConfig{}
		if err := a.SMTP.BuildFromService(v.SMTP); err != nil {
			return err
		}
		a.BufferTargetPerInterval = v.BufferTargetPerInterval
		a.BufferIntervalSeconds = v.BufferIntervalSeconds
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APINotifyConfig) ToService() (interface{}, error) {
	smtp, err := a.SMTP.ToService()
	if err != nil {
		return nil, err
	}
	return evergreen.NotifyConfig{
		BufferTargetPerInterval: a.BufferTargetPerInterval,
		BufferIntervalSeconds:   a.BufferIntervalSeconds,
		SMTP:                    smtp.(evergreen.SMTPConfig),
	}, nil
}

type APICloudProviders struct {
	AWS       *APIAWSConfig       `json:"aws"`
	Docker    *APIDockerConfig    `json:"docker"`
	GCE       *APIGCEConfig       `json:"gce"`
	OpenStack *APIOpenStackConfig `json:"openstack"`
	VSphere   *APIVSphereConfig   `json:"vsphere"`
}

func (a *APICloudProviders) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.CloudProviders:
		a.AWS = &APIAWSConfig{}
		a.Docker = &APIDockerConfig{}
		a.GCE = &APIGCEConfig{}
		a.OpenStack = &APIOpenStackConfig{}
		a.VSphere = &APIVSphereConfig{}
		if err := a.AWS.BuildFromService(v.AWS); err != nil {
			return err
		}
		if err := a.Docker.BuildFromService(v.Docker); err != nil {
			return err
		}
		if err := a.GCE.BuildFromService(v.GCE); err != nil {
			return err
		}
		if err := a.OpenStack.BuildFromService(v.OpenStack); err != nil {
			return err
		}
		if err := a.VSphere.BuildFromService(v.VSphere); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APICloudProviders) ToService() (interface{}, error) {
	aws, err := a.AWS.ToService()
	if err != nil {
		return nil, err
	}
	docker, err := a.Docker.ToService()
	if err != nil {
		return nil, err
	}
	gce, err := a.GCE.ToService()
	if err != nil {
		return nil, err
	}
	openstack, err := a.OpenStack.ToService()
	if err != nil {
		return nil, err
	}
	vsphere, err := a.VSphere.ToService()
	if err != nil {
		return nil, err
	}
	return evergreen.CloudProviders{
		AWS:       aws.(evergreen.AWSConfig),
		Docker:    docker.(evergreen.DockerConfig),
		GCE:       gce.(evergreen.GCEConfig),
		OpenStack: openstack.(evergreen.OpenStackConfig),
		VSphere:   vsphere.(evergreen.VSphereConfig),
	}, nil
}

type APICommitQueueConfig struct {
	MergeTaskDistro *string `json:"merge_task_distro"`
	CommitterName   *string `json:"committer_name"`
	CommitterEmail  *string `json:"committer_email"`
}

func (a *APICommitQueueConfig) BuildFromService(h interface{}) error {
	if v, ok := h.(evergreen.CommitQueueConfig); ok {
		a.MergeTaskDistro = ToStringPtr(v.MergeTaskDistro)
		a.CommitterName = ToStringPtr(v.CommitterName)
		a.CommitterEmail = ToStringPtr(v.CommitterEmail)

		return nil
	}

	return errors.Errorf("Received CommitQueueConfig of type %T", h)
}

func (a *APICommitQueueConfig) ToService() (interface{}, error) {
	return evergreen.CommitQueueConfig{
		MergeTaskDistro: FromStringPtr(a.MergeTaskDistro),
		CommitterName:   FromStringPtr(a.CommitterName),
		CommitterEmail:  FromStringPtr(a.CommitterEmail),
	}, nil
}

type APIContainerPoolsConfig struct {
	Pools []APIContainerPool `json:"pools"`
}

func (a *APIContainerPoolsConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.ContainerPoolsConfig:
		for _, pool := range v.Pools {
			APIpool := APIContainerPool{}
			if err := APIpool.BuildFromService(pool); err != nil {
				return err
			}
			a.Pools = append(a.Pools, APIpool)
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIContainerPoolsConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.ContainerPoolsConfig{}
	for _, p := range a.Pools {
		i, err := p.ToService()
		if err != nil {
			return nil, err
		}
		pool := i.(evergreen.ContainerPool)
		config.Pools = append(config.Pools, pool)
	}
	return config, nil
}

type APIContainerPool struct {
	Distro        *string `json:"distro"`
	Id            *string `json:"id"`
	MaxContainers int     `json:"max_containers"`
	Port          uint16  `json:"port"`
}

func (a *APIContainerPool) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.ContainerPool:
		a.Distro = ToStringPtr(v.Distro)
		a.Id = ToStringPtr(v.Id)
		a.MaxContainers = v.MaxContainers
		a.Port = v.Port
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIContainerPool) ToService() (interface{}, error) {
	return evergreen.ContainerPool{
		Distro:        FromStringPtr(a.Distro),
		Id:            FromStringPtr(a.Id),
		MaxContainers: a.MaxContainers,
		Port:          a.Port,
	}, nil
}

type APIEC2Key struct {
	Name   *string `json:"name"`
	Region *string `json:"region"`
	Key    *string `json:"key"`
	Secret *string `json:"secret"`
}

func (a *APIEC2Key) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.EC2Key:
		a.Name = ToStringPtr(v.Name)
		a.Region = ToStringPtr(v.Region)
		a.Key = ToStringPtr(v.Key)
		a.Secret = ToStringPtr(v.Secret)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIEC2Key) ToService() (interface{}, error) {
	res := evergreen.EC2Key{}
	res.Name = FromStringPtr(a.Name)
	res.Region = FromStringPtr(a.Region)
	res.Key = FromStringPtr(a.Key)
	res.Secret = FromStringPtr(a.Secret)
	return res, nil
}

type APIAWSConfig struct {
	EC2Keys              []APIEC2Key `json:"ec2_keys"`
	S3Key                *string     `json:"s3_key"`
	S3Secret             *string     `json:"s3_secret"`
	Bucket               *string     `json:"bucket"`
	S3BaseURL            *string     `json:"s3_base_url"`
	DefaultSecurityGroup *string     `json:"default_security_group"`
	AllowedInstanceTypes []*string   `json:"allowed_instance_types"`
	MaxVolumeSizePerUser *int        `json:"max_volume_size"`
}

func (a *APIAWSConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AWSConfig:
		for _, key := range v.EC2Keys {
			apiKey := APIEC2Key{}
			if err := apiKey.BuildFromService(key); err != nil {
				return err
			}
			a.EC2Keys = append(a.EC2Keys, apiKey)
		}
		a.S3Key = ToStringPtr(v.S3Key)
		a.S3Secret = ToStringPtr(v.S3Secret)
		a.Bucket = ToStringPtr(v.Bucket)
		a.S3BaseURL = ToStringPtr(v.S3BaseURL)
		a.DefaultSecurityGroup = ToStringPtr(v.DefaultSecurityGroup)
		a.MaxVolumeSizePerUser = &v.MaxVolumeSizePerUser

		for _, t := range v.AllowedInstanceTypes {
			a.AllowedInstanceTypes = append(a.AllowedInstanceTypes, ToStringPtr(t))
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAWSConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	config := evergreen.AWSConfig{
		S3Key:                FromStringPtr(a.S3Key),
		S3Secret:             FromStringPtr(a.S3Secret),
		Bucket:               FromStringPtr(a.Bucket),
		S3BaseURL:            FromStringPtr(a.S3BaseURL),
		DefaultSecurityGroup: FromStringPtr(a.DefaultSecurityGroup),
		MaxVolumeSizePerUser: host.DefaultMaxVolumeSizePerUser,
	}
	if a.MaxVolumeSizePerUser != nil {
		config.MaxVolumeSizePerUser = *a.MaxVolumeSizePerUser
	}

	for _, k := range a.EC2Keys {
		i, err := k.ToService()
		if err != nil {
			return nil, err
		}
		key, ok := i.(evergreen.EC2Key)
		if !ok {
			return nil, errors.New("Unable to convert key to EC2Key")
		}
		config.EC2Keys = append(config.EC2Keys, key)
	}

	for _, t := range a.AllowedInstanceTypes {
		config.AllowedInstanceTypes = append(config.AllowedInstanceTypes, FromStringPtr(t))
	}

	return config, nil
}

type APIDockerConfig struct {
	APIVersion *string `json:"api_version"`
}

func (a *APIDockerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.DockerConfig:
		a.APIVersion = ToStringPtr(v.APIVersion)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIDockerConfig) ToService() (interface{}, error) {
	return evergreen.DockerConfig{
		APIVersion: FromStringPtr(a.APIVersion),
	}, nil
}

type APIGCEConfig struct {
	ClientEmail  *string `json:"client_email"`
	PrivateKey   *string `json:"private_key"`
	PrivateKeyID *string `json:"private_key_id"`
	TokenURI     *string `json:"token_uri"`
}

func (a *APIGCEConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.GCEConfig:
		a.ClientEmail = ToStringPtr(v.ClientEmail)
		a.PrivateKey = ToStringPtr(v.PrivateKey)
		a.PrivateKeyID = ToStringPtr(v.PrivateKeyID)
		a.TokenURI = ToStringPtr(v.TokenURI)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIGCEConfig) ToService() (interface{}, error) {
	return evergreen.GCEConfig{
		ClientEmail:  FromStringPtr(a.ClientEmail),
		PrivateKey:   FromStringPtr(a.PrivateKey),
		PrivateKeyID: FromStringPtr(a.PrivateKeyID),
		TokenURI:     FromStringPtr(a.TokenURI),
	}, nil
}

type APIOpenStackConfig struct {
	IdentityEndpoint *string `json:"identity_endpoint"`

	Username   *string `json:"username"`
	Password   *string `json:"password"`
	DomainName *string `json:"domain_name"`

	ProjectName *string `json:"project_name"`
	ProjectID   *string `json:"project_id"`

	Region *string `json:"region"`
}

func (a *APIOpenStackConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.OpenStackConfig:
		a.IdentityEndpoint = ToStringPtr(v.IdentityEndpoint)
		a.Username = ToStringPtr(v.Username)
		a.Password = ToStringPtr(v.Password)
		a.DomainName = ToStringPtr(v.DomainName)
		a.ProjectName = ToStringPtr(v.ProjectName)
		a.ProjectID = ToStringPtr(v.ProjectID)
		a.Region = ToStringPtr(v.Region)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIOpenStackConfig) ToService() (interface{}, error) {
	return evergreen.OpenStackConfig{
		IdentityEndpoint: FromStringPtr(a.IdentityEndpoint),
		Username:         FromStringPtr(a.Username),
		Password:         FromStringPtr(a.Password),
		DomainName:       FromStringPtr(a.DomainName),
		ProjectID:        FromStringPtr(a.ProjectID),
		ProjectName:      FromStringPtr(a.ProjectName),
		Region:           FromStringPtr(a.Region),
	}, nil
}

type APIVSphereConfig struct {
	Host     *string `json:"host"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

func (a *APIVSphereConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.VSphereConfig:
		a.Host = ToStringPtr(v.Host)
		a.Username = ToStringPtr(v.Username)
		a.Password = ToStringPtr(v.Password)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIVSphereConfig) ToService() (interface{}, error) {
	return evergreen.VSphereConfig{
		Host:     FromStringPtr(a.Host),
		Username: FromStringPtr(a.Username),
		Password: FromStringPtr(a.Password),
	}, nil
}

type APIRepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `json:"revs_to_fetch"`
	MaxRepoRevisionsToSearch   int `json:"max_revs_to_search"`
	MaxConcurrentRequests      int `json:"max_con_requests"`
}

func (a *APIRepoTrackerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.RepoTrackerConfig:
		a.NumNewRepoRevisionsToFetch = v.NumNewRepoRevisionsToFetch
		a.MaxConcurrentRequests = v.MaxConcurrentRequests
		a.MaxRepoRevisionsToSearch = v.MaxRepoRevisionsToSearch
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIRepoTrackerConfig) ToService() (interface{}, error) {
	return evergreen.RepoTrackerConfig{
		NumNewRepoRevisionsToFetch: a.NumNewRepoRevisionsToFetch,
		MaxConcurrentRequests:      a.MaxConcurrentRequests,
		MaxRepoRevisionsToSearch:   a.MaxRepoRevisionsToSearch,
	}, nil
}

type APISchedulerConfig struct {
	TaskFinder                    *string `json:"task_finder"`
	HostAllocator                 *string `json:"host_allocator"`
	FreeHostFraction              float64 `json:"free_host_fraction"`
	CacheDurationSeconds          int     `json:"cache_duration_seconds"`
	Planner                       *string `json:"planner"`
	TargetTimeSeconds             int     `json:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `json:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `json:"group_versions"`
	PatchFactor                   int64   `json:"patch_factor"`
	PatchTimeInQueueFactor        int64   `json:"patch_time_in_queue_factor"`
	MainlineTimeInQueueFactor     int64   `json:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor         int64   `json:"expected_runtime_factor"`
}

func (a *APISchedulerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.SchedulerConfig:
		a.TaskFinder = ToStringPtr(v.TaskFinder)
		a.HostAllocator = ToStringPtr(v.HostAllocator)
		a.FreeHostFraction = v.FreeHostFraction
		a.CacheDurationSeconds = v.CacheDurationSeconds
		a.Planner = ToStringPtr(v.Planner)
		a.TargetTimeSeconds = v.TargetTimeSeconds
		a.AcceptableHostIdleTimeSeconds = v.AcceptableHostIdleTimeSeconds
		a.GroupVersions = v.GroupVersions
		a.PatchFactor = v.PatchFactor
		a.PatchTimeInQueueFactor = v.PatchTimeInQueueFactor
		a.MainlineTimeInQueueFactor = v.MainlineTimeInQueueFactor
		a.ExpectedRuntimeFactor = v.ExpectedRuntimeFactor
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISchedulerConfig) ToService() (interface{}, error) {
	return evergreen.SchedulerConfig{
		TaskFinder:                    FromStringPtr(a.TaskFinder),
		HostAllocator:                 FromStringPtr(a.HostAllocator),
		FreeHostFraction:              a.FreeHostFraction,
		CacheDurationSeconds:          a.CacheDurationSeconds,
		Planner:                       FromStringPtr(a.Planner),
		TargetTimeSeconds:             a.TargetTimeSeconds,
		AcceptableHostIdleTimeSeconds: a.AcceptableHostIdleTimeSeconds,
		GroupVersions:                 a.GroupVersions,
		PatchFactor:                   a.PatchFactor,
		ExpectedRuntimeFactor:         a.ExpectedRuntimeFactor,
		PatchTimeInQueueFactor:        a.PatchTimeInQueueFactor,
		MainlineTimeInQueueFactor:     a.MainlineTimeInQueueFactor,
	}, nil
}

// APIServiceFlags is a public structure representing the admin service flags
type APIServiceFlags struct {
	TaskDispatchDisabled       bool `json:"task_dispatch_disabled"`
	HostInitDisabled           bool `json:"host_init_disabled"`
	MonitorDisabled            bool `json:"monitor_disabled"`
	AlertsDisabled             bool `json:"alerts_disabled"`
	AgentStartDisabled         bool `json:"agent_start_disabled"`
	RepotrackerDisabled        bool `json:"repotracker_disabled"`
	SchedulerDisabled          bool `json:"scheduler_disabled"`
	GithubPRTestingDisabled    bool `json:"github_pr_testing_disabled"`
	CLIUpdatesDisabled         bool `json:"cli_updates_disabled"`
	BackgroundStatsDisabled    bool `json:"background_stats_disabled"`
	TaskLoggingDisabled        bool `json:"task_logging_disabled"`
	CacheStatsJobDisabled      bool `json:"cache_stats_job_disabled"`
	CacheStatsEndpointDisabled bool `json:"cache_stats_endpoint_disabled"`
	CacheStatsOldTasksDisabled bool `json:"cache_stats_old_tasks_disabled"`
	TaskReliabilityDisabled    bool `json:"task_reliability_disabled"`
	CommitQueueDisabled        bool `json:"commit_queue_disabled"`
	PlannerDisabled            bool `json:"planner_disabled"`
	HostAllocatorDisabled      bool `json:"host_allocator_disabled"`
	DRBackupDisabled           bool `json:"dr_backup_disabled"`

	// Notifications Flags
	EventProcessingDisabled      bool `json:"event_processing_disabled"`
	JIRANotificationsDisabled    bool `json:"jira_notifications_disabled"`
	SlackNotificationsDisabled   bool `json:"slack_notifications_disabled"`
	EmailNotificationsDisabled   bool `json:"email_notifications_disabled"`
	WebhookNotificationsDisabled bool `json:"webhook_notifications_disabled"`
	GithubStatusAPIDisabled      bool `json:"github_status_api_disabled"`
}

type APISlackConfig struct {
	Options *APISlackOptions `json:"options"`
	Token   *string          `json:"token"`
	Level   *string          `json:"level"`
}

func (a *APISlackConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.SlackConfig:
		a.Token = ToStringPtr(v.Token)
		a.Level = ToStringPtr(v.Level)
		if v.Options != nil {
			a.Options = &APISlackOptions{}
			if err := a.Options.BuildFromService(*v.Options); err != nil { //nolint: vet
				return err
			}
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISlackConfig) ToService() (interface{}, error) {
	i, err := a.Options.ToService()
	if err != nil {
		return nil, err
	}
	options := i.(send.SlackOptions) //nolint: vet
	return evergreen.SlackConfig{
		Token:   FromStringPtr(a.Token),
		Level:   FromStringPtr(a.Level),
		Options: &options,
	}, nil
}

type APISlackOptions struct {
	Channel       *string         `json:"channel"`
	Hostname      *string         `json:"hostname"`
	Name          *string         `json:"name"`
	Username      *string         `json:"username"`
	IconURL       *string         `json:"icon_url"`
	BasicMetadata bool            `json:"add_basic_metadata"`
	Fields        bool            `json:"use_fields"`
	AllFields     bool            `json:"all_fields"`
	FieldsSet     map[string]bool `json:"fields"`
}

func (a *APISlackOptions) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case send.SlackOptions:
		a.Channel = ToStringPtr(v.Channel)
		a.Hostname = ToStringPtr(v.Hostname)
		a.Name = ToStringPtr(v.Name)
		a.Username = ToStringPtr(v.Username)
		a.IconURL = ToStringPtr(v.IconURL)
		a.BasicMetadata = v.BasicMetadata
		a.Fields = v.Fields
		a.AllFields = v.AllFields
		a.FieldsSet = v.FieldsSet
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISlackOptions) ToService() (interface{}, error) {
	if a == nil {
		return send.SlackOptions{}, nil
	}
	return send.SlackOptions{
		Channel:       FromStringPtr(a.Channel),
		Hostname:      FromStringPtr(a.Hostname),
		Name:          FromStringPtr(a.Name),
		Username:      FromStringPtr(a.Username),
		IconURL:       FromStringPtr(a.IconURL),
		BasicMetadata: a.BasicMetadata,
		Fields:        a.Fields,
		AllFields:     a.AllFields,
		FieldsSet:     a.FieldsSet,
	}, nil
}

type APISplunkConnectionInfo struct {
	ServerURL *string `json:"url"`
	Token     *string `json:"token"`
	Channel   *string `json:"channel"`
}

func (a *APISplunkConnectionInfo) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case send.SplunkConnectionInfo:
		a.ServerURL = ToStringPtr(v.ServerURL)
		a.Token = ToStringPtr(v.Token)
		a.Channel = ToStringPtr(v.Channel)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISplunkConnectionInfo) ToService() (interface{}, error) {
	return send.SplunkConnectionInfo{
		ServerURL: FromStringPtr(a.ServerURL),
		Token:     FromStringPtr(a.Token),
		Channel:   FromStringPtr(a.Channel),
	}, nil
}

type APIUIConfig struct {
	Url                     *string  `json:"url"`
	HelpUrl                 *string  `json:"help_url"`
	UIv2Url                 *string  `json:"uiv2_url"`
	HttpListenAddr          *string  `json:"http_listen_addr"`
	Secret                  *string  `json:"secret"`
	DefaultProject          *string  `json:"default_project"`
	CacheTemplates          bool     `json:"cache_templates"`
	CsrfKey                 *string  `json:"csrf_key"`
	CORSOrigins             []string `json:"cors_origins"`
	LoginDomain             *string  `json:"login_domain"`
	ExpireLoginCookieDomain *string  `json:"expire_domain"`
}

func (a *APIUIConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.UIConfig:
		a.Url = ToStringPtr(v.Url)
		a.HelpUrl = ToStringPtr(v.HelpUrl)
		a.UIv2Url = ToStringPtr(v.UIv2Url)
		a.HttpListenAddr = ToStringPtr(v.HttpListenAddr)
		a.Secret = ToStringPtr(v.Secret)
		a.DefaultProject = ToStringPtr(v.DefaultProject)
		a.CacheTemplates = v.CacheTemplates
		a.CsrfKey = ToStringPtr(v.CsrfKey)
		a.CORSOrigins = v.CORSOrigins
		a.LoginDomain = ToStringPtr(v.LoginDomain)
		a.ExpireLoginCookieDomain = ToStringPtr(v.ExpireLoginCookieDomain)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIUIConfig) ToService() (interface{}, error) {
	return evergreen.UIConfig{
		Url:                     FromStringPtr(a.Url),
		HelpUrl:                 FromStringPtr(a.HelpUrl),
		UIv2Url:                 FromStringPtr(a.UIv2Url),
		HttpListenAddr:          FromStringPtr(a.HttpListenAddr),
		Secret:                  FromStringPtr(a.Secret),
		DefaultProject:          FromStringPtr(a.DefaultProject),
		CacheTemplates:          a.CacheTemplates,
		CsrfKey:                 FromStringPtr(a.CsrfKey),
		CORSOrigins:             a.CORSOrigins,
		LoginDomain:             FromStringPtr(a.LoginDomain),
		ExpireLoginCookieDomain: FromStringPtr(a.ExpireLoginCookieDomain),
	}, nil
}

type APINewRelicConfig struct {
	AccountID     *string `json:"accountId"`
	TrustKey      *string `json:"trustKey"`
	AgentID       *string `json:"agentId"`
	LicenseKey    *string `json:"licenseKey"`
	ApplicationID *string `json:"applicationId"`
}

// BuildFromService builds a model from the service layer
func (a *APINewRelicConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.NewRelicConfig:
		a.AccountID = ToStringPtr(v.AccountID)
		a.TrustKey = ToStringPtr(v.TrustKey)
		a.AgentID = ToStringPtr(v.AgentID)
		a.LicenseKey = ToStringPtr(v.LicenseKey)
		a.ApplicationID = ToStringPtr(v.ApplicationID)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (a *APINewRelicConfig) ToService() (interface{}, error) {
	return evergreen.NewRelicConfig{
		AccountID:     FromStringPtr(a.AccountID),
		TrustKey:      FromStringPtr(a.TrustKey),
		AgentID:       FromStringPtr(a.AgentID),
		LicenseKey:    FromStringPtr(a.LicenseKey),
		ApplicationID: FromStringPtr(a.ApplicationID),
	}, nil
}

// RestartTasksResponse is the response model returned from the /admin/restart route
type RestartResponse struct {
	ItemsRestarted []string `json:"items_restarted"`
	ItemsErrored   []string `json:"items_errored"`
}

// BuildFromService builds a model from the service layer
func (ab *APIBanner) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case APIBanner:
		ab.Text = v.Text
		ab.Theme = v.Theme
	default:
		return errors.Errorf("%T is not a supported admin banner type", h)
	}
	return nil
}

// ToService is not yet implemented
func (ab *APIBanner) ToService() (interface{}, error) {
	return ab, nil
}

// BuildFromService builds a model from the service layer
func (as *APIServiceFlags) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.ServiceFlags:
		as.TaskDispatchDisabled = v.TaskDispatchDisabled
		as.HostInitDisabled = v.HostInitDisabled
		as.MonitorDisabled = v.MonitorDisabled
		as.AlertsDisabled = v.AlertsDisabled
		as.AgentStartDisabled = v.AgentStartDisabled
		as.RepotrackerDisabled = v.RepotrackerDisabled
		as.SchedulerDisabled = v.SchedulerDisabled
		as.GithubPRTestingDisabled = v.GithubPRTestingDisabled
		as.CLIUpdatesDisabled = v.CLIUpdatesDisabled
		as.EventProcessingDisabled = v.EventProcessingDisabled
		as.JIRANotificationsDisabled = v.JIRANotificationsDisabled
		as.SlackNotificationsDisabled = v.SlackNotificationsDisabled
		as.EmailNotificationsDisabled = v.EmailNotificationsDisabled
		as.WebhookNotificationsDisabled = v.WebhookNotificationsDisabled
		as.GithubStatusAPIDisabled = v.GithubStatusAPIDisabled
		as.BackgroundStatsDisabled = v.BackgroundStatsDisabled
		as.TaskLoggingDisabled = v.TaskLoggingDisabled
		as.CacheStatsJobDisabled = v.CacheStatsJobDisabled
		as.CacheStatsEndpointDisabled = v.CacheStatsEndpointDisabled
		as.CacheStatsOldTasksDisabled = v.CacheStatsOldTasksDisabled
		as.TaskReliabilityDisabled = v.TaskReliabilityDisabled
		as.CommitQueueDisabled = v.CommitQueueDisabled
		as.PlannerDisabled = v.PlannerDisabled
		as.HostAllocatorDisabled = v.HostAllocatorDisabled
		as.DRBackupDisabled = v.DRBackupDisabled
	default:
		return errors.Errorf("%T is not a supported service flags type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIServiceFlags) ToService() (interface{}, error) {
	return evergreen.ServiceFlags{
		TaskDispatchDisabled:         as.TaskDispatchDisabled,
		HostInitDisabled:             as.HostInitDisabled,
		MonitorDisabled:              as.MonitorDisabled,
		AlertsDisabled:               as.AlertsDisabled,
		AgentStartDisabled:           as.AgentStartDisabled,
		RepotrackerDisabled:          as.RepotrackerDisabled,
		SchedulerDisabled:            as.SchedulerDisabled,
		GithubPRTestingDisabled:      as.GithubPRTestingDisabled,
		CLIUpdatesDisabled:           as.CLIUpdatesDisabled,
		EventProcessingDisabled:      as.EventProcessingDisabled,
		JIRANotificationsDisabled:    as.JIRANotificationsDisabled,
		SlackNotificationsDisabled:   as.SlackNotificationsDisabled,
		EmailNotificationsDisabled:   as.EmailNotificationsDisabled,
		WebhookNotificationsDisabled: as.WebhookNotificationsDisabled,
		GithubStatusAPIDisabled:      as.GithubStatusAPIDisabled,
		BackgroundStatsDisabled:      as.BackgroundStatsDisabled,
		TaskLoggingDisabled:          as.TaskLoggingDisabled,
		CacheStatsJobDisabled:        as.CacheStatsJobDisabled,
		CacheStatsEndpointDisabled:   as.CacheStatsEndpointDisabled,
		CacheStatsOldTasksDisabled:   as.CacheStatsOldTasksDisabled,
		TaskReliabilityDisabled:      as.TaskReliabilityDisabled,
		CommitQueueDisabled:          as.CommitQueueDisabled,
		PlannerDisabled:              as.PlannerDisabled,
		HostAllocatorDisabled:        as.HostAllocatorDisabled,
		DRBackupDisabled:             as.DRBackupDisabled,
	}, nil
}

// BuildFromService builds a model from the service layer
func (rtr *RestartResponse) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *RestartResponse:
		rtr.ItemsRestarted = v.ItemsRestarted
		rtr.ItemsErrored = v.ItemsErrored
	default:
		return errors.Errorf("%T is the incorrect type for a restart task response", h)
	}
	return nil
}

// ToService is not implemented for /admin/restart
func (rtr *RestartResponse) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for RestartTasksResponse")
}

func AdminDbToRestModel(in evergreen.ConfigSection) (Model, error) {
	id := in.SectionId()
	var out Model
	if id == evergreen.ConfigDocID {
		out = &APIAdminSettings{}
		err := out.BuildFromService(reflect.ValueOf(in).Interface())
		if err != nil {
			return nil, err
		}
	} else {
		structVal := reflect.ValueOf(*NewConfigModel())
		for i := 0; i < structVal.NumField(); i++ {
			// this assumes that the json tag is the same as the section ID
			tag := strings.Split(structVal.Type().Field(i).Tag.Get("json"), ",")[0]
			if tag != id {
				continue
			}

			propName := structVal.Type().Field(i).Name
			propVal := structVal.FieldByName(propName)
			propInterface := propVal.Interface()
			apiModel, ok := propInterface.(Model)
			if !ok {
				return nil, fmt.Errorf("unable to convert section %s to a Model interface", id)
			}
			out = apiModel
		}
		if out == nil {
			return nil, fmt.Errorf("section %s is not defined in the APIAdminSettings struct", id)
		}
		err := out.BuildFromService(reflect.Indirect(reflect.ValueOf(in)).Interface())
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

type APIJIRANotificationsConfig struct {
	CustomFields map[string]APIJIRANotificationsProject `json:"custom_fields,omitempty"`
}

type APIJIRANotificationsProject struct {
	Fields     map[string]string `json:"fields,omitempty"`
	Components []string          `json:"components,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
}

func (j *APIJIRANotificationsConfig) BuildFromService(h interface{}) error {
	var config *evergreen.JIRANotificationsConfig
	switch v := h.(type) {
	case *evergreen.JIRANotificationsConfig:
		config = v
	case evergreen.JIRANotificationsConfig:
		config = &v
	default:
		return errors.Errorf("expected *evergreen.JIRANotificationsConfig, but got %T instead", h)
	}

	j.CustomFields = make(map[string]APIJIRANotificationsProject)
	for _, project := range config.CustomFields {
		apiProject := APIJIRANotificationsProject{}
		if err := apiProject.BuildFromService(project); err != nil {
			return errors.Wrapf(err, "can't build project '%s' from service", project.Project)
		}

		j.CustomFields[project.Project] = apiProject
	}

	return nil
}

func (j *APIJIRANotificationsConfig) ToService() (interface{}, error) {
	service := evergreen.JIRANotificationsConfig{}
	if j.CustomFields == nil || len(j.CustomFields) == 0 {
		return service, nil
	}

	for projectName, fields := range j.CustomFields {
		projectIface, err := fields.ToService()
		if err != nil {
			return nil, errors.Errorf("can't convert project '%s' to service", projectName)
		}
		project := projectIface.(evergreen.JIRANotificationsProject)

		project.Project = projectName
		service.CustomFields = append(service.CustomFields, project)
	}

	return service, nil
}

func (j *APIJIRANotificationsProject) BuildFromService(h interface{}) error {
	serviceProject, ok := h.(evergreen.JIRANotificationsProject)
	if !ok {
		return errors.Errorf("Expecting JIRANotificationsProject but got %T", h)
	}

	apiFields := make(map[string]string)
	for _, field := range serviceProject.Fields {
		apiFields[field.Field] = field.Template
	}
	j.Fields = apiFields
	j.Components = serviceProject.Components
	j.Labels = serviceProject.Labels

	return nil
}

func (j *APIJIRANotificationsProject) ToService() (interface{}, error) {
	service := evergreen.JIRANotificationsProject{}
	for field, template := range j.Fields {
		service.Fields = append(service.Fields, evergreen.JIRANotificationsCustomField{Field: field, Template: template})
	}
	service.Components = j.Components
	service.Labels = j.Labels

	return service, nil
}

type APITriggerConfig struct {
	GenerateTaskDistro *string `json:"generate_distro"`
}

func (c *APITriggerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.TriggerConfig:
		c.GenerateTaskDistro = ToStringPtr(v.GenerateTaskDistro)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}
func (c *APITriggerConfig) ToService() (interface{}, error) {
	return evergreen.TriggerConfig{
		GenerateTaskDistro: FromStringPtr(c.GenerateTaskDistro),
	}, nil
}

type APIHostJasperConfig struct {
	BinaryName       *string `json:"binary_name,omitempty"`
	DownloadFileName *string `json:"download_file_name,omitempty"`
	Port             int     `json:"port,omitempty"`
	URL              *string `json:"url,omitempty"`
	Version          *string `json:"version,omitempty"`
}

func (c *APIHostJasperConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.HostJasperConfig:
		c.BinaryName = ToStringPtr(v.BinaryName)
		c.DownloadFileName = ToStringPtr(v.DownloadFileName)
		c.Port = v.Port
		c.URL = ToStringPtr(v.URL)
		c.Version = ToStringPtr(v.Version)
	default:
		return errors.Errorf("expected evergreen.HostJasperConfig but got %T instead", h)
	}
	return nil
}

func (c *APIHostJasperConfig) ToService() (interface{}, error) {
	return evergreen.HostJasperConfig{
		BinaryName:       FromStringPtr(c.BinaryName),
		DownloadFileName: FromStringPtr(c.DownloadFileName),
		Port:             c.Port,
		URL:              FromStringPtr(c.URL),
		Version:          FromStringPtr(c.Version),
	}, nil
}
