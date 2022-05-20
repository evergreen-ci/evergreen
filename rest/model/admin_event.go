package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
)

type APIAdminEvent struct {
	Timestamp *time.Time `json:"ts"`
	User      string     `json:"user"`
	Section   string     `json:"section"`
	Before    Model      `json:"before"`
	After     Model      `json:"after"`
	Guid      string     `json:"guid"`
}

func (e *APIAdminEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.EventLogEntry:
		e.Timestamp = ToTimePtr(v.Timestamp)
		data, ok := v.Data.(*event.AdminEventData)
		if !ok {
			return errors.Errorf("programmatic error: expected admin event but got type %T", v.Data)
		}
		e.User = data.User
		e.Section = data.Section
		e.Guid = data.GUID
		before, err := AdminDbToRestModel(data.Changes.Before)
		if err != nil {
			return errors.Wrap(err, "converting 'before' changes for admin event")
		}
		after, err := AdminDbToRestModel(data.Changes.After)
		if err != nil {
			return errors.Wrap(err, "converting 'after' changes for admin event")
		}
		e.Before = before
		e.After = after
	default:
		return errors.Errorf("programmatic error: expected event log entry but got type %T", h)
	}

	return nil
}

func (e *APIAdminEvent) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for APIAdminEvent")
}
