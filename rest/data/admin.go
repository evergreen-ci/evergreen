package data

import (
	"context"
	"fmt"
	"reflect"
	"sync"
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
		restarted, notRestarted, err := model.RetryCommitQueueItems(pRef.Identifier, pRef.CommitQueue.PatchType, opts)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"project":    pRef.Identifier,
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

type MockAdminConnector struct {
	mu           sync.RWMutex
	MockSettings *evergreen.Settings
}

// GetEvergreenSettings retrieves the admin settings document from the mock connector
func (ac *MockAdminConnector) GetEvergreenSettings() (*evergreen.Settings, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.MockSettings, nil
}

func (ac *MockAdminConnector) GetBanner() (string, string, error) {
	return ac.MockSettings.Banner, string(ac.MockSettings.BannerTheme), nil
}

// SetEvergreenSettings sets the admin settings document in the mock connector
func (ac *MockAdminConnector) SetEvergreenSettings(changes *restModel.APIAdminSettings,
	oldSettings *evergreen.Settings, u *user.DBUser, persist bool) (*evergreen.Settings, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	i, err := changes.ToService()
	if err != nil {
		return nil, err
	}
	settingsModel := i.(evergreen.Settings)
	ac.MockSettings = &settingsModel
	return ac.MockSettings, nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.Banner = text
	return nil
}

func (ac *MockAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := evergreen.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.BannerTheme = theme
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags evergreen.ServiceFlags, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.ServiceFlags = flags
	return nil
}

// RestartFailedTasks mocks a response to restarting failed tasks
func (ac *MockAdminConnector) RestartFailedTasks(queue amboy.Queue, opts model.RestartOptions) (*restModel.RestartResponse, error) {
	return &restModel.RestartResponse{
		ItemsRestarted: []string{"task1", "task2", "task3"},
		ItemsErrored:   nil,
	}, nil
}

func (ac *MockAdminConnector) RevertConfigTo(guid string, user string) error {
	return nil
}

func (ac *MockAdminConnector) GetAdminEventLog(before time.Time, n int) ([]restModel.APIAdminEvent, error) {
	return nil, nil
}

func (ac *MockAdminConnector) RestartFailedCommitQueueVersions(opts model.RestartOptions) (*restModel.RestartResponse, error) {
	return nil, errors.New("not implemented")
}

func (ac *MockAdminConnector) MapLDAPGroupToRole(group, roleID string) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}

	ac.MockSettings.LDAPRoleMap = append(
		ac.MockSettings.LDAPRoleMap,
		evergreen.LDAPRoleMapping{group, roleID},
	)

	return nil
}

func (ac *MockAdminConnector) UnmapLDAPGroupToRole(group string) error {
	if ac.MockSettings == nil {
		return nil
	}

	for i, mapping := range ac.MockSettings.LDAPRoleMap {
		if mapping.LDAPGroup == group {
			ac.MockSettings.LDAPRoleMap[i] = ac.MockSettings.LDAPRoleMap[len(ac.MockSettings.LDAPRoleMap)-1]
			ac.MockSettings.LDAPRoleMap = ac.MockSettings.LDAPRoleMap[:len(ac.MockSettings.LDAPRoleMap)-1]
			return nil
		}
	}

	return nil
}
