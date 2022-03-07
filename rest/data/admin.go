package data

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type DBAdminConnector struct{}

// GetEvergreenSettings retrieves the admin settings document from the DB
func (ac *DBAdminConnector) GetEvergreenSettings() (*evergreen.Settings, error) {
	return evergreen.GetConfig()
}

func (ac *DBAdminConnector) GetBanner() (string, string, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return "", "", errors.Wrap(err, "error retrieving settings from DB")
	}
	return settings.Banner, string(settings.BannerTheme), nil
}

// SetEvergreenSettings sets the admin settings document in the DB and event logs it
func (ac *DBAdminConnector) SetEvergreenSettings(changes *restModel.APIAdminSettings,
	oldSettings *evergreen.Settings, u *user.DBUser, persist bool) (*evergreen.Settings, error) {

	settingsAPI := restModel.NewConfigModel()
	err := settingsAPI.BuildFromService(oldSettings)
	if err != nil {
		return nil, errors.Wrap(err, "error converting existing settings")
	}
	changesReflect := reflect.ValueOf(*changes)
	settingsReflect := reflect.ValueOf(settingsAPI)

	//iterate over each field in the changes struct and apply any changes to the existing settings
	for i := 0; i < changesReflect.NumField(); i++ {
		// get the property name and find its value within the settings struct
		propName := changesReflect.Type().Field(i).Name
		changedVal := changesReflect.FieldByName(propName)
		if changedVal.IsNil() {
			continue
		}

		settingsReflect.Elem().FieldByName(propName).Set(changedVal)
	}

	i, err := settingsAPI.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "error converting to DB model")
	}
	newSettings := i.(evergreen.Settings)
	if persist {
		// We have to call Validate before we attempt to persist it because the
		// evergreen.Settings internally calls ValidateAndDefault to set the
		// default values.
		if err = newSettings.Validate(); err != nil {
			return nil, errors.Wrap(err, "validation failed")
		}
		err = evergreen.UpdateConfig(&newSettings)
		if err != nil {
			return nil, errors.Wrap(err, "error saving new settings")
		}
		newSettings.Id = evergreen.ConfigDocID
		return &newSettings, LogConfigChanges(&newSettings, oldSettings, u)
	}

	return &newSettings, nil
}

func LogConfigChanges(newSettings *evergreen.Settings, oldSettings *evergreen.Settings, u *user.DBUser) error {

	catcher := grip.NewSimpleCatcher()
	// log the other config sub-documents
	valConfig := reflect.ValueOf(*newSettings)
	var oldStruct reflect.Value
	if oldSettings != nil {
		oldStruct = reflect.ValueOf(*oldSettings)
	}

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
		section, ok := propInterface.(evergreen.ConfigSection)
		if !ok {
			catcher.Add(fmt.Errorf("unable to convert config section %s", propName))
			continue
		}

		// save changes in the event log
		if oldSettings != nil {
			field := oldStruct.FieldByName(propName)
			oldPointer := reflect.Indirect(reflect.New(field.Type()))
			oldPointer.Set(field)
			oldInterface := oldPointer.Addr().Interface()
			catcher.Add(event.LogAdminEvent(section.SectionId(), oldInterface.(evergreen.ConfigSection), section, u.Username()))
		}
	}

	// log the root config document
	if err := event.LogAdminEvent(newSettings.SectionId(), oldSettings, newSettings, u.Username()); err != nil {
		catcher.Add(errors.Wrap(err, "error saving event log for root document"))
	}

	return errors.WithStack(catcher.Resolve())
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	return evergreen.SetBanner(text)
}

// SetBannerTheme sets the banner theme in the DB and event logs it
func (ac *DBAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := evergreen.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}

	return evergreen.SetBannerTheme(theme)
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags evergreen.ServiceFlags, u *user.DBUser) error {

	return evergreen.SetServiceFlags(flags)
}

// RestartFailedTasks attempts to restart failed tasks that started between 2 times
func (ac *DBAdminConnector) RestartFailedTasks(queue amboy.Queue, opts model.RestartOptions) (*restModel.RestartResponse, error) {
	var results model.RestartResults
	var err error

	if opts.DryRun {
		results, err = model.RestartFailedTasks(opts)
		if err != nil {
			return nil, err
		}
	} else {
		if err = queue.Put(context.TODO(), units.NewTasksRestartJob(opts)); err != nil {
			return nil, errors.Wrap(err, "error starting background job for task restart")
		}
	}

	return &restModel.RestartResponse{
		ItemsRestarted: results.ItemsRestarted,
		ItemsErrored:   results.ItemsErrored,
	}, nil
}

func (ac *DBAdminConnector) RestartFailedCommitQueueVersions(opts model.RestartOptions) (*restModel.RestartResponse, error) {
	totalRestarted := []string{}
	totalNotRestarted := []string{}
	pRefs, err := model.FindProjectRefsWithCommitQueueEnabled()
	if err != nil {
		return nil, errors.Wrapf(err, "error finding projects with commit queue enabled")
	}
	for _, pRef := range pRefs {
		restarted, notRestarted, err := model.RetryCommitQueueItems(pRef.Id, opts)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"project":    pRef.Id,
				"start_time": opts.StartTime,
				"end_time":   opts.EndTime,
				"message":    "unable to restart failed commit queue versions for project",
			}))
			continue
		}
		totalRestarted = append(totalRestarted, restarted...)
		totalNotRestarted = append(totalNotRestarted, notRestarted...)
	}
	return &restModel.RestartResponse{
		ItemsRestarted: totalRestarted,
		ItemsErrored:   totalNotRestarted,
	}, nil
}

func (ac *DBAdminConnector) RevertConfigTo(guid string, user string) error {
	return event.RevertConfig(guid, user)
}

func (ac *DBAdminConnector) GetAdminEventLog(before time.Time, n int) ([]restModel.APIAdminEvent, error) {
	events, err := event.FindAdmin(event.AdminEventsBefore(before, n))
	if err != nil {
		return nil, err
	}
	out := []restModel.APIAdminEvent{}
	catcher := grip.NewBasicCatcher()
	for _, evt := range events {
		apiEvent := restModel.APIAdminEvent{}
		err = apiEvent.BuildFromService(evt)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, apiEvent)
	}

	return out, catcher.Resolve()
}

func (ac *DBAdminConnector) MapLDAPGroupToRole(group, roleID string) error {
	mapping := &evergreen.LDAPRoleMap{}
	return errors.Wrapf(mapping.Add(group, roleID), "error mapping %s to %s", group, roleID)
}

func (ac *DBAdminConnector) UnmapLDAPGroupToRole(group string) error {
	mapping := &evergreen.LDAPRoleMap{}
	return errors.Wrapf(mapping.Remove(group), "error unmapping %s", group)
}
