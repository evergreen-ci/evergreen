package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// APIAdminSettings is the structure of a response to the admin route
type APIAdminSettings struct {
	Alerts             APIAlertsConfig                   `json:"alerts"`
	Amboy              APIAmboyConfig                    `json:"amboy"`
	Api                APIapiConfig                      `json:"api"`
	ApiUrl             APIString                         `json:"api_url"`
	AuthConfig         APIAuthConfig                     `json:"auth"`
	Banner             APIString                         `json:"banner"`
	BannerTheme        APIString                         `json:"banner_theme"`
	ClientBinariesDir  APIString                         `json:"client_binaries_dir"`
	ConfigDir          APIString                         `json:"configdir"`
	Credentials        map[string]APIString              `json:"credentials"`
	Expansions         map[string]APIString              `json:"expansions"`
	GithubPRCreatorOrg APIString                         `json:"github_pr_creator_org"`
	HostInit           APIHostInitConfig                 `json:"hostinit"`
	IsNonProd          bool                              `json:"isnonprod"`
	Jira               APIJiraConfig                     `json:"jira"`
	Keys               map[string]APIString              `json:"keys"`
	LoggerConfig       APILoggerConfig                   `json:"logger_config"`
	LogPath            APIString                         `json:"log_path"`
	NewRelic           APINewRelicConfig                 `json:"new_relic"`
	Notify             APINotifyConfig                   `json:"notify"`
	Plugins            map[string]map[string]interface{} `json:"plugins"`
	PprofPort          APIString                         `json:"pprof_port"`
	Providers          APICloudProviders                 `json:"providers"`
	RepoTracker        APIRepoTrackerConfig              `json:"repotracker"`
	Scheduler          APISchedulerConfig                `json:"scheduler"`
	ServiceFlags       APIServiceFlags                   `json:"service_flags"`
	Slack              APISlackConfig                    `json:"slack"`
	Splunk             APISplunkConnectionInfo           `json:"splunk"`
	SuperUsers         []APIString                       `json:"superusers"`
	Ui                 APIUIConfig                       `json:"ui"`
}

// BuildFromService builds a model from the service layer
func (as *APIAdminSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.Settings:
		if v == nil {
			return errors.New("evergreen settings object is nil")
		}
		if err := as.Alerts.BuildFromService(v.Alerts); err != nil {
			return err
		}
		if err := as.Amboy.BuildFromService(v.Amboy); err != nil {
			return err
		}
		if err := as.Api.BuildFromService(v.Api); err != nil {
			return err
		}
		if err := as.AuthConfig.BuildFromService(v.AuthConfig); err != nil {
			return err
		}
		if err := as.HostInit.BuildFromService(v.HostInit); err != nil {
			return err
		}
		if err := as.Jira.BuildFromService(v.Jira); err != nil {
			return err
		}
		if err := as.LoggerConfig.BuildFromService(v.LoggerConfig); err != nil {
			return err
		}
		if err := as.NewRelic.BuildFromService(v.NewRelic); err != nil {
			return err
		}
		if err := as.Notify.BuildFromService(v.Notify); err != nil {
			return err
		}
		if err := as.Providers.BuildFromService(v.Providers); err != nil {
			return err
		}
		if err := as.RepoTracker.BuildFromService(v.RepoTracker); err != nil {
			return err
		}
		if err := as.Scheduler.BuildFromService(v.Scheduler); err != nil {
			return err
		}
		if err := as.ServiceFlags.BuildFromService(v.ServiceFlags); err != nil {
			return err
		}
		if err := as.Slack.BuildFromService(v.Slack); err != nil {
			return err
		}
		if err := as.Splunk.BuildFromService(v.Splunk); err != nil {
			return err
		}
		if err := as.Ui.BuildFromService(v.Ui); err != nil {
			return err
		}
		as.ApiUrl = APIString(v.ApiUrl)
		as.Banner = APIString(v.Banner)
		as.BannerTheme = APIString(v.BannerTheme)
		as.ClientBinariesDir = APIString(v.ClientBinariesDir)
		as.ConfigDir = APIString(v.ConfigDir)
		as.GithubPRCreatorOrg = APIString(v.GithubPRCreatorOrg)
		as.IsNonProd = v.IsNonProd
		as.LogPath = APIString(v.LogPath)
		as.Plugins = v.Plugins
		as.PprofPort = APIString(v.PprofPort)
		as.Credentials = map[string]APIString{}
		for k, v := range v.Credentials {
			as.Credentials[k] = APIString(v)
		}
		as.Expansions = map[string]APIString{}
		for k, v := range v.Expansions {
			as.Expansions[k] = APIString(v)
		}
		as.Keys = map[string]APIString{}
		for k, v := range v.Keys {
			as.Keys[k] = APIString(v)
		}
		for _, user := range v.SuperUsers {
			as.SuperUsers = append(as.SuperUsers, APIString(user))
		}
	default:
		return errors.Errorf("%T is not a supported admin settings type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIAdminSettings) ToService() (interface{}, error) {
	alerts, err := as.Alerts.ToService()
	if err != nil {
		return nil, err
	}
	amboy, err := as.Amboy.ToService()
	if err != nil {
		return nil, err
	}
	api, err := as.Api.ToService()
	if err != nil {
		return nil, err
	}
	auth, err := as.AuthConfig.ToService()
	if err != nil {
		return nil, err
	}
	hostinit, err := as.HostInit.ToService()
	if err != nil {
		return nil, err
	}
	jira, err := as.Jira.ToService()
	if err != nil {
		return nil, err
	}
	logger, err := as.LoggerConfig.ToService()
	if err != nil {
		return nil, err
	}
	newrelic, err := as.NewRelic.ToService()
	if err != nil {
		return nil, err
	}
	notify, err := as.Notify.ToService()
	if err != nil {
		return nil, err
	}
	cloud, err := as.Providers.ToService()
	if err != nil {
		return nil, err
	}
	repotracker, err := as.RepoTracker.ToService()
	if err != nil {
		return nil, err
	}
	scheduler, err := as.Scheduler.ToService()
	if err != nil {
		return nil, err
	}
	flags, err := as.ServiceFlags.ToService()
	if err != nil {
		return nil, err
	}
	slack, err := as.Slack.ToService()
	if err != nil {
		return nil, err
	}
	splunk, err := as.Splunk.ToService()
	if err != nil {
		return nil, err
	}
	ui, err := as.Ui.ToService()
	if err != nil {
		return nil, err
	}
	settings := evergreen.Settings{
		Alerts:             alerts.(evergreen.AlertsConfig),
		Amboy:              amboy.(evergreen.AmboyConfig),
		Api:                api.(evergreen.APIConfig),
		ApiUrl:             string(as.ApiUrl),
		AuthConfig:         auth.(evergreen.AuthConfig),
		Banner:             string(as.Banner),
		BannerTheme:        evergreen.BannerTheme(string(as.BannerTheme)),
		ClientBinariesDir:  string(as.ClientBinariesDir),
		ConfigDir:          string(as.ConfigDir),
		Credentials:        map[string]string{},
		Expansions:         map[string]string{},
		GithubPRCreatorOrg: string(as.GithubPRCreatorOrg),
		HostInit:           hostinit.(evergreen.HostInitConfig),
		IsNonProd:          as.IsNonProd,
		Jira:               jira.(evergreen.JiraConfig),
		Keys:               map[string]string{},
		LoggerConfig:       logger.(evergreen.LoggerConfig),
		LogPath:            string(as.LogPath),
		NewRelic:           newrelic.(evergreen.NewRelicConfig),
		Notify:             notify.(evergreen.NotifyConfig),
		Plugins:            evergreen.PluginConfig{},
		PprofPort:          string(as.PprofPort),
		Providers:          cloud.(evergreen.CloudProviders),
		RepoTracker:        repotracker.(evergreen.RepoTrackerConfig),
		Scheduler:          scheduler.(evergreen.SchedulerConfig),
		ServiceFlags:       flags.(evergreen.ServiceFlags),
		Slack:              slack.(evergreen.SlackConfig),
		Splunk:             splunk.(send.SplunkConnectionInfo),
		Ui:                 ui.(evergreen.UIConfig),
	}
	for k, v := range as.Credentials {
		settings.Credentials[k] = string(v)
	}
	for k, v := range as.Expansions {
		settings.Expansions[k] = string(v)
	}
	for k, v := range as.Keys {
		settings.Keys[k] = string(v)
	}
	for k, v := range as.Plugins {
		settings.Plugins[k] = map[string]interface{}{}
		for k2, v2 := range v {
			settings.Plugins[k][k2] = v2
		}
	}
	for _, user := range as.SuperUsers {
		settings.SuperUsers = append(settings.SuperUsers, string(user))
	}
	return settings, nil
}

type APIAlertsConfig struct {
	SMTP *APISMTPConfig `json:"smtp"`
}

func (a *APIAlertsConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AlertsConfig:
		a.SMTP = &APISMTPConfig{}
		if err := a.SMTP.BuildFromService(v.SMTP); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAlertsConfig) ToService() (interface{}, error) {
	var config *evergreen.SMTPConfig
	smtp, err := a.SMTP.ToService()
	if err != nil {
		return nil, err
	}
	if smtp != nil {
		config = smtp.(*evergreen.SMTPConfig)
	}
	return evergreen.AlertsConfig{
		SMTP: config,
	}, nil
}

type APISMTPConfig struct {
	Server     APIString   `json:"server"`
	Port       int         `json:"port"`
	UseSSL     bool        `json:"use_ssl"`
	Username   APIString   `json:"username"`
	Password   APIString   `json:"password"`
	From       APIString   `json:"from"`
	AdminEmail []APIString `json:"admin_email"`
}

func (a *APISMTPConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.SMTPConfig:
		if v == nil {
			return nil
		}
		a.Server = APIString(v.Server)
		a.Port = v.Port
		a.UseSSL = v.UseSSL
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
		a.From = APIString(v.From)
		for _, s := range v.AdminEmail {
			a.AdminEmail = append(a.AdminEmail, APIString(s))
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
		Server:   string(a.Server),
		Port:     a.Port,
		UseSSL:   a.UseSSL,
		Username: string(a.Username),
		Password: string(a.Password),
		From:     string(a.From),
	}
	for _, s := range a.AdminEmail {
		config.AdminEmail = append(config.AdminEmail, string(s))
	}
	return &config, nil
}

type APIAmboyConfig struct {
	Name           APIString `json:"name"`
	DB             APIString `json:"database"`
	PoolSizeLocal  int       `json:"pool_size_local"`
	PoolSizeRemote int       `json:"pool_size_remote"`
	LocalStorage   int       `json:"local_storage_size"`
}

func (a *APIAmboyConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AmboyConfig:
		a.Name = APIString(v.Name)
		a.DB = APIString(v.DB)
		a.PoolSizeLocal = v.PoolSizeLocal
		a.PoolSizeRemote = v.PoolSizeRemote
		a.LocalStorage = v.LocalStorage
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAmboyConfig) ToService() (interface{}, error) {
	return evergreen.AmboyConfig{
		Name:           string(a.Name),
		DB:             string(a.DB),
		PoolSizeLocal:  a.PoolSizeLocal,
		PoolSizeRemote: a.PoolSizeRemote,
		LocalStorage:   a.LocalStorage,
	}, nil
}

type APIapiConfig struct {
	HttpListenAddr      APIString `json:"http_listen_addr"`
	GithubWebhookSecret APIString `json:"github_webhook_secret"`
}

func (a *APIapiConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.APIConfig:
		a.HttpListenAddr = APIString(v.HttpListenAddr)
		a.GithubWebhookSecret = APIString(v.GithubWebhookSecret)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIapiConfig) ToService() (interface{}, error) {
	return evergreen.APIConfig{
		HttpListenAddr:      string(a.HttpListenAddr),
		GithubWebhookSecret: string(a.GithubWebhookSecret),
	}, nil
}

type APIAuthConfig struct {
	Crowd  *APICrowdConfig      `json:"crowd"`
	Naive  *APINaiveAuthConfig  `json:"naive"`
	Github *APIGithubAuthConfig `json:"github"`
}

func (a *APIAuthConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AuthConfig:
		a.Crowd = &APICrowdConfig{}
		a.Naive = &APINaiveAuthConfig{}
		a.Github = &APIGithubAuthConfig{}
		if err := a.Crowd.BuildFromService(v.Crowd); err != nil {
			return err
		}
		if err := a.Naive.BuildFromService(v.Naive); err != nil {
			return err
		}
		if err := a.Github.BuildFromService(v.Github); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAuthConfig) ToService() (interface{}, error) {
	var crowd *evergreen.CrowdConfig
	var naive *evergreen.NaiveAuthConfig
	var github *evergreen.GithubAuthConfig
	i, err := a.Crowd.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		crowd = i.(*evergreen.CrowdConfig)
	}
	i, err = a.Naive.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		naive = i.(*evergreen.NaiveAuthConfig)
	}
	i, err = a.Github.ToService()
	if err != nil {
		return nil, err
	}
	if i != nil {
		github = i.(*evergreen.GithubAuthConfig)
	}
	return evergreen.AuthConfig{
		Crowd:  crowd,
		Naive:  naive,
		Github: github,
	}, nil
}

type APICrowdConfig struct {
	Username APIString `json:"username"`
	Password APIString `json:"password"`
	Urlroot  APIString `json:"url_root"`
}

func (a *APICrowdConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.CrowdConfig:
		if v == nil {
			return nil
		}
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
		a.Urlroot = APIString(v.Urlroot)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APICrowdConfig) ToService() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	return &evergreen.CrowdConfig{
		Username: string(a.Username),
		Password: string(a.Password),
		Urlroot:  string(a.Urlroot),
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
	Username    APIString `json:"username"`
	DisplayName APIString `json:"display_name"`
	Password    APIString `json:"password"`
	Email       APIString `json:"email"`
}

func (a *APIAuthUser) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.AuthUser:
		if v == nil {
			return nil
		}
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
		a.DisplayName = APIString(v.DisplayName)
		a.Email = APIString(v.Email)
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
		Username:    string(a.Username),
		Password:    string(a.Password),
		DisplayName: string(a.DisplayName),
		Email:       string(a.Email),
	}, nil
}

type APIGithubAuthConfig struct {
	ClientId     APIString   `json:"client_id"`
	ClientSecret APIString   `json:"client_secret"`
	Users        []APIString `json:"users"`
	Organization APIString   `json:"organization"`
}

func (a *APIGithubAuthConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *evergreen.GithubAuthConfig:
		if v == nil {
			return nil
		}
		a.ClientId = APIString(v.ClientId)
		a.ClientSecret = APIString(v.ClientSecret)
		a.Organization = APIString(v.Organization)
		for _, u := range v.Users {
			a.Users = append(a.Users, APIString(u))
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
		ClientId:     string(a.ClientId),
		ClientSecret: string(a.ClientSecret),
		Organization: string(a.Organization),
	}
	for _, u := range a.Users {
		config.Users = append(config.Users, string(u))
	}
	return &config, nil
}

// APIBanner is a public structure representing the banner part of the admin settings
type APIBanner struct {
	Text  APIString `json:"banner"`
	Theme APIString `json:"theme"`
}

type APIHostInitConfig struct {
	SSHTimeoutSeconds int64 `json:"ssh_timeout_secs"`
}

func (a *APIHostInitConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.HostInitConfig:
		a.SSHTimeoutSeconds = v.SSHTimeoutSeconds
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIHostInitConfig) ToService() (interface{}, error) {
	return evergreen.HostInitConfig{
		SSHTimeoutSeconds: a.SSHTimeoutSeconds,
	}, nil
}

type APIJiraConfig struct {
	Host           APIString `json:"host"`
	Username       APIString `json:"username"`
	Password       APIString `json:"password"`
	DefaultProject APIString `json:"default_project"`
}

func (a *APIJiraConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.JiraConfig:
		a.Host = APIString(v.Host)
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
		a.DefaultProject = APIString(v.DefaultProject)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIJiraConfig) ToService() (interface{}, error) {
	return evergreen.JiraConfig{
		Host:           string(a.Host),
		Username:       string(a.Username),
		Password:       string(a.Password),
		DefaultProject: string(a.DefaultProject),
	}, nil
}

type APILoggerConfig struct {
	Buffer         APILogBuffering `json:"buffer"`
	DefaultLevel   APIString       `json:"default_level"`
	ThresholdLevel APIString       `json:"threshold_level"`
}

func (a *APILoggerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.LoggerConfig:
		a.DefaultLevel = APIString(v.DefaultLevel)
		a.ThresholdLevel = APIString(v.ThresholdLevel)
		a.Buffer = APILogBuffering{}
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
		DefaultLevel:   string(a.DefaultLevel),
		ThresholdLevel: string(a.ThresholdLevel),
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

type APINewRelicConfig struct {
	ApplicationName APIString `json:"application_name"`
	LicenseKey      APIString `json:"license_key"`
}

func (a *APINewRelicConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.NewRelicConfig:
		a.ApplicationName = APIString(v.ApplicationName)
		a.LicenseKey = APIString(v.LicenseKey)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APINewRelicConfig) ToService() (interface{}, error) {
	return evergreen.NewRelicConfig{
		ApplicationName: string(a.ApplicationName),
		LicenseKey:      string(a.LicenseKey),
	}, nil
}

type APINotifyConfig struct {
	SMTP *APISMTPConfig `json:"smtp"`
}

func (a *APINotifyConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.NotifyConfig:
		a.SMTP = &APISMTPConfig{}
		if err := a.SMTP.BuildFromService(v.SMTP); err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APINotifyConfig) ToService() (interface{}, error) {
	var config *evergreen.SMTPConfig
	smtp, err := a.SMTP.ToService()
	if err != nil {
		return nil, err
	}
	if smtp != nil {
		config = smtp.(*evergreen.SMTPConfig)
	}
	return evergreen.NotifyConfig{
		SMTP: config,
	}, nil
}

type APICloudProviders struct {
	AWS       APIAWSConfig       `json:"aws"`
	Docker    APIDockerConfig    `json:"docker"`
	GCE       APIGCEConfig       `json:"gce"`
	OpenStack APIOpenStackConfig `json:"openstack"`
	VSphere   APIVSphereConfig   `json:"vsphere"`
}

func (a *APICloudProviders) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.CloudProviders:
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

type APIAWSConfig struct {
	Secret APIString `json:"aws_secret"`
	Id     APIString `json:"aws_id"`
}

func (a *APIAWSConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.AWSConfig:
		a.Secret = APIString(v.Secret)
		a.Id = APIString(v.Id)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIAWSConfig) ToService() (interface{}, error) {
	return evergreen.AWSConfig{
		Id:     string(a.Id),
		Secret: string(a.Secret),
	}, nil
}

type APIDockerConfig struct {
	APIVersion APIString `json:"api_version"`
}

func (a *APIDockerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.DockerConfig:
		a.APIVersion = APIString(v.APIVersion)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIDockerConfig) ToService() (interface{}, error) {
	return evergreen.DockerConfig{
		APIVersion: string(a.APIVersion),
	}, nil
}

type APIGCEConfig struct {
	ClientEmail  APIString `json:"client_email"`
	PrivateKey   APIString `json:"private_key"`
	PrivateKeyID APIString `json:"private_key_id"`
	TokenURI     APIString `json:"token_uri"`
}

func (a *APIGCEConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.GCEConfig:
		a.ClientEmail = APIString(v.ClientEmail)
		a.PrivateKey = APIString(v.PrivateKey)
		a.PrivateKeyID = APIString(v.PrivateKeyID)
		a.TokenURI = APIString(v.TokenURI)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIGCEConfig) ToService() (interface{}, error) {
	return evergreen.GCEConfig{
		ClientEmail:  string(a.ClientEmail),
		PrivateKey:   string(a.PrivateKey),
		PrivateKeyID: string(a.PrivateKeyID),
		TokenURI:     string(a.TokenURI),
	}, nil
}

type APIOpenStackConfig struct {
	IdentityEndpoint APIString `json:"identity_endpoint"`

	Username   APIString `json:"username"`
	Password   APIString `json:"password"`
	DomainName APIString `json:"domain_name"`

	ProjectName APIString `json:"project_name"`
	ProjectID   APIString `json:"project_id"`

	Region APIString `json:"region"`
}

func (a *APIOpenStackConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.OpenStackConfig:
		a.IdentityEndpoint = APIString(v.IdentityEndpoint)
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
		a.DomainName = APIString(v.DomainName)
		a.ProjectName = APIString(v.ProjectName)
		a.ProjectID = APIString(v.ProjectID)
		a.Region = APIString(v.Region)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIOpenStackConfig) ToService() (interface{}, error) {
	return evergreen.OpenStackConfig{
		IdentityEndpoint: string(a.IdentityEndpoint),
		Username:         string(a.Username),
		Password:         string(a.Password),
		DomainName:       string(a.DomainName),
		ProjectID:        string(a.ProjectID),
		ProjectName:      string(a.ProjectName),
		Region:           string(a.Region),
	}, nil
}

type APIVSphereConfig struct {
	Host     APIString `json:"host"`
	Username APIString `json:"username"`
	Password APIString `json:"password"`
}

func (a *APIVSphereConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.VSphereConfig:
		a.Host = APIString(v.Host)
		a.Username = APIString(v.Username)
		a.Password = APIString(v.Password)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIVSphereConfig) ToService() (interface{}, error) {
	return evergreen.VSphereConfig{
		Host:     string(a.Host),
		Username: string(a.Username),
		Password: string(a.Password),
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
	MergeToggle int       `json:"merge_toggle"`
	TaskFinder  APIString `json:"task_finder"`
}

func (a *APISchedulerConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.SchedulerConfig:
		a.MergeToggle = v.MergeToggle
		a.TaskFinder = APIString(v.TaskFinder)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISchedulerConfig) ToService() (interface{}, error) {
	return evergreen.SchedulerConfig{
		MergeToggle: a.MergeToggle,
		TaskFinder:  string(a.TaskFinder),
	}, nil
}

// APIServiceFlags is a public structure representing the admin service flags
type APIServiceFlags struct {
	TaskDispatchDisabled         bool `json:"task_dispatch_disabled"`
	HostinitDisabled             bool `json:"hostinit_disabled"`
	MonitorDisabled              bool `json:"monitor_disabled"`
	NotificationsDisabled        bool `json:"notifications_disabled"`
	AlertsDisabled               bool `json:"alerts_disabled"`
	TaskrunnerDisabled           bool `json:"taskrunner_disabled"`
	RepotrackerDisabled          bool `json:"repotracker_disabled"`
	SchedulerDisabled            bool `json:"scheduler_disabled"`
	GithubPRTestingDisabled      bool `json:"github_pr_testing_disabled"`
	RepotrackerPushEventDisabled bool `json:"repotracker_push_event_disabled"`
	CLIUpdatesDisabled           bool `json:"cli_updates_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled" json:"github_status_api_disabled"`
}

type APISlackConfig struct {
	Options APISlackOptions `json:"options"`
	Token   APIString       `json:"token"`
	Level   APIString       `json:"level"`
}

func (a *APISlackConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.SlackConfig:
		a.Token = APIString(v.Token)
		a.Level = APIString(v.Level)
		a.Options = APISlackOptions{}
		if err := a.Options.BuildFromService(*v.Options); err != nil {
			return err
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
	options := i.(send.SlackOptions)
	return evergreen.SlackConfig{
		Token:   string(a.Token),
		Level:   string(a.Level),
		Options: &options,
	}, nil
}

type APISlackOptions struct {
	Channel       APIString       `json:"channel"`
	Hostname      APIString       `json:"hostname"`
	Name          APIString       `json:"name"`
	BasicMetadata bool            `json:"add_basic_metadata"`
	Fields        bool            `json:"use_fields"`
	AllFields     bool            `json:"all_fields"`
	FieldsSet     map[string]bool `json:"fields"`
}

func (a *APISlackOptions) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case send.SlackOptions:
		a.Channel = APIString(v.Channel)
		a.Hostname = APIString(v.Hostname)
		a.Name = APIString(v.Name)
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
	return send.SlackOptions{
		Channel:       string(a.Channel),
		Hostname:      string(a.Hostname),
		Name:          string(a.Name),
		BasicMetadata: a.BasicMetadata,
		Fields:        a.Fields,
		AllFields:     a.AllFields,
		FieldsSet:     a.FieldsSet,
	}, nil
}

type APISplunkConnectionInfo struct {
	ServerURL APIString `json:"url"`
	Token     APIString `json:"token"`
	Channel   APIString `json:"channel"`
}

func (a *APISplunkConnectionInfo) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case send.SplunkConnectionInfo:
		a.ServerURL = APIString(v.ServerURL)
		a.Token = APIString(v.Token)
		a.Channel = APIString(v.Channel)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APISplunkConnectionInfo) ToService() (interface{}, error) {
	return send.SplunkConnectionInfo{
		ServerURL: string(a.ServerURL),
		Token:     string(a.Token),
		Channel:   string(a.Channel),
	}, nil
}

type APIUIConfig struct {
	Url            APIString `json:"url"`
	HelpUrl        APIString `json:"help_url"`
	HttpListenAddr APIString `json:"http_listen_addr"`
	Secret         APIString `json:"secret"`
	DefaultProject APIString `json:"default_project"`
	CacheTemplates bool      `json:"cache_templates"`
	SecureCookies  bool      `json:"secure_cookies"`
	CsrfKey        APIString `json:"csrf_key"`
}

func (a *APIUIConfig) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.UIConfig:
		a.Url = APIString(v.Url)
		a.HelpUrl = APIString(v.HelpUrl)
		a.HttpListenAddr = APIString(v.HttpListenAddr)
		a.Secret = APIString(v.Secret)
		a.DefaultProject = APIString(v.DefaultProject)
		a.CacheTemplates = v.CacheTemplates
		a.SecureCookies = v.SecureCookies
		a.CsrfKey = APIString(v.CsrfKey)
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (a *APIUIConfig) ToService() (interface{}, error) {
	return evergreen.UIConfig{
		Url:            string(a.Url),
		HelpUrl:        string(a.HelpUrl),
		HttpListenAddr: string(a.HttpListenAddr),
		Secret:         string(a.Secret),
		DefaultProject: string(a.DefaultProject),
		CacheTemplates: a.CacheTemplates,
		SecureCookies:  a.SecureCookies,
		CsrfKey:        string(a.CsrfKey),
	}, nil
}

// RestartTasksResponse is the response model returned from the /admin/restart route
type RestartTasksResponse struct {
	TasksRestarted []string `json:"tasks_restarted"`
	TasksErrored   []string `json:"tasks_errored"`
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
	return nil, errors.New("ToService not implemented for banner")
}

// BuildFromService builds a model from the service layer
func (as *APIServiceFlags) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case evergreen.ServiceFlags:
		as.TaskDispatchDisabled = v.TaskDispatchDisabled
		as.HostinitDisabled = v.HostinitDisabled
		as.MonitorDisabled = v.MonitorDisabled
		as.NotificationsDisabled = v.NotificationsDisabled
		as.AlertsDisabled = v.AlertsDisabled
		as.TaskrunnerDisabled = v.TaskrunnerDisabled
		as.RepotrackerDisabled = v.RepotrackerDisabled
		as.SchedulerDisabled = v.SchedulerDisabled
		as.GithubPRTestingDisabled = v.GithubPRTestingDisabled
		as.RepotrackerPushEventDisabled = v.RepotrackerPushEventDisabled
		as.CLIUpdatesDisabled = v.CLIUpdatesDisabled
		as.GithubStatusAPIDisabled = v.GithubStatusAPIDisabled
	default:
		return errors.Errorf("%T is not a supported service flags type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIServiceFlags) ToService() (interface{}, error) {
	return evergreen.ServiceFlags{
		TaskDispatchDisabled:         as.TaskDispatchDisabled,
		HostinitDisabled:             as.HostinitDisabled,
		MonitorDisabled:              as.MonitorDisabled,
		NotificationsDisabled:        as.NotificationsDisabled,
		AlertsDisabled:               as.AlertsDisabled,
		TaskrunnerDisabled:           as.TaskrunnerDisabled,
		RepotrackerDisabled:          as.RepotrackerDisabled,
		SchedulerDisabled:            as.SchedulerDisabled,
		GithubPRTestingDisabled:      as.GithubPRTestingDisabled,
		RepotrackerPushEventDisabled: as.RepotrackerPushEventDisabled,
		CLIUpdatesDisabled:           as.CLIUpdatesDisabled,
		GithubStatusAPIDisabled:      as.GithubStatusAPIDisabled,
	}, nil
}

// BuildFromService builds a model from the service layer
func (rtr *RestartTasksResponse) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *RestartTasksResponse:
		rtr.TasksRestarted = v.TasksRestarted
		rtr.TasksErrored = v.TasksErrored
	default:
		return errors.Errorf("%T is the incorrect type for a restart task response", h)
	}
	return nil
}

// ToService is not implemented for /admin/restart
func (rtr *RestartTasksResponse) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for RestartTasksResponse")
}
