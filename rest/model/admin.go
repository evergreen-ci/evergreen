package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/pkg/errors"
)

// APIAdminSettings is the structure of a response to the admin route
type APIAdminSettings struct {
	Banner       APIString       `json:"banner"`
	BannerTheme  APIString       `json:"banner_theme"`
	ServiceFlags APIServiceFlags `json:"service_flags"`
}

// APIBanner is a public structure representing the banner part of the admin settings
type APIBanner struct {
	Text  APIString `json:"banner"`
	Theme APIString `json:"theme"`
}

// APIServiceFlags is a public structure representing the admin service flags
type APIServiceFlags struct {
	TaskDispatchDisabled  bool `json:"task_dispatch_disabled"`
	HostinitDisabled      bool `json:"hostinit_disabled"`
	MonitorDisabled       bool `json:"monitor_disabled"`
	NotificationsDisabled bool `json:"notifications_disabled"`
	AlertsDisabled        bool `json:"alerts_disabled"`
	TaskrunnerDisabled    bool `json:"taskrunner_disabled"`
	RepotrackerDisabled   bool `json:"repotracker_disabled"`
	SchedulerDisabled     bool `json:"scheduler_disabled"`
}

// RestartTasksResponse is the response model returned from the /admin/restart route
type RestartTasksResponse struct {
	TasksRestarted []string `json:"tasks_restarted"`
	TasksErrored   []string `json:"tasks_errored"`
}

// BuildFromService builds a model from the service layer
func (as *APIAdminSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *admin.AdminSettings:
		as.Banner = APIString(v.Banner)
		as.BannerTheme = APIString(v.BannerTheme)
		err := as.ServiceFlags.BuildFromService(v.ServiceFlags)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("%T is not a supported admin settings type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIAdminSettings) ToService() (interface{}, error) {
	flags, err := as.ServiceFlags.ToService()
	if err != nil {
		return nil, err
	}
	valid, theme := admin.IsValidBannerTheme(string(as.BannerTheme))
	if !valid {
		return nil, fmt.Errorf("%s is not a valid banner theme type", as.BannerTheme)
	}
	settings := admin.AdminSettings{
		Banner:       string(as.Banner),
		BannerTheme:  theme,
		ServiceFlags: flags.(admin.ServiceFlags),
	}
	return settings, nil
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
	case admin.ServiceFlags:
		as.TaskDispatchDisabled = v.TaskDispatchDisabled
		as.HostinitDisabled = v.HostinitDisabled
		as.MonitorDisabled = v.MonitorDisabled
		as.NotificationsDisabled = v.NotificationsDisabled
		as.AlertsDisabled = v.AlertsDisabled
		as.TaskrunnerDisabled = v.TaskrunnerDisabled
		as.RepotrackerDisabled = v.RepotrackerDisabled
		as.SchedulerDisabled = v.SchedulerDisabled
	default:
		return errors.Errorf("%T is not a supported service flags type", h)
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIServiceFlags) ToService() (interface{}, error) {
	serviceFlags := admin.ServiceFlags{
		TaskDispatchDisabled:  as.TaskDispatchDisabled,
		HostinitDisabled:      as.HostinitDisabled,
		MonitorDisabled:       as.MonitorDisabled,
		NotificationsDisabled: as.NotificationsDisabled,
		AlertsDisabled:        as.AlertsDisabled,
		TaskrunnerDisabled:    as.TaskrunnerDisabled,
		RepotrackerDisabled:   as.RepotrackerDisabled,
		SchedulerDisabled:     as.SchedulerDisabled,
	}
	return serviceFlags, nil
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
