package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/model/admin"
)

// APIAdminSettings is the structure of a response to the admin route
type APIAdminSettings struct {
	Banner       APIString       `json:"banner"`
	ServiceFlags APIServiceFlags `json:"service_flags"`
}

// APIBanner is a public structure representing the banner part of the admin settings
type APIBanner struct {
	Text APIString `json:"banner"`
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

// BuildFromService builds a model from the service layer
func (as *APIAdminSettings) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *admin.AdminSettings:
		as.Banner = APIString(v.Banner)
		as.ServiceFlags = APIServiceFlags{
			TaskDispatchDisabled:  v.ServiceFlags.TaskDispatchDisabled,
			HostinitDisabled:      v.ServiceFlags.HostinitDisabled,
			MonitorDisabled:       v.ServiceFlags.MonitorDisabled,
			NotificationsDisabled: v.ServiceFlags.NotificationsDisabled,
			AlertsDisabled:        v.ServiceFlags.AlertsDisabled,
			TaskrunnerDisabled:    v.ServiceFlags.TaskrunnerDisabled,
			RepotrackerDisabled:   v.ServiceFlags.RepotrackerDisabled,
			SchedulerDisabled:     v.ServiceFlags.SchedulerDisabled,
		}
	default:
		return errors.New("Incorrect type when unmarshalling admin settings")
	}
	return nil
}

// ToService returns a service model from an API model
func (as *APIAdminSettings) ToService() (interface{}, error) {
	settings := admin.AdminSettings{
		Banner: string(as.Banner),
		ServiceFlags: admin.ServiceFlags{
			TaskDispatchDisabled:  as.ServiceFlags.TaskDispatchDisabled,
			HostinitDisabled:      as.ServiceFlags.HostinitDisabled,
			MonitorDisabled:       as.ServiceFlags.MonitorDisabled,
			NotificationsDisabled: as.ServiceFlags.NotificationsDisabled,
			AlertsDisabled:        as.ServiceFlags.AlertsDisabled,
			TaskrunnerDisabled:    as.ServiceFlags.TaskrunnerDisabled,
			RepotrackerDisabled:   as.ServiceFlags.RepotrackerDisabled,
			SchedulerDisabled:     as.ServiceFlags.SchedulerDisabled,
		},
	}
	return settings, nil
}

// BuildFromService builds a model from the service layer
func (ab *APIBanner) BuildFromService(h interface{}) error {
	switch h.(type) {
	case string:
		ab.Text = APIString(h.(string))
	default:
		return errors.New("Incorrect type when unmarshalling banner")
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
		return errors.New("Incorrect type when unmarshalling service flags")
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
