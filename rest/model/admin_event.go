package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
)

type APIAdminEvent struct {
	Timestamp time.Time `json:"ts"`
	User      string    `json:"user"`
	Section   string    `json:"section"`
	Before    Model     `json:"before"`
	After     Model     `json:"after"`
	Guid      string    `json:"guid"`
}

func (e *APIAdminEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.EventLogEntry:
		e.Timestamp = v.Timestamp
		data, ok := v.Data.(*event.AdminEventData)
		if !ok {
			return errors.New("unable to convert event type to admin event")
		}
		e.User = data.User
		e.Section = data.Section
		e.Guid = data.GUID
		before, err := AdminDbToRestModel(data.Changes.Before)
		if err != nil {
			return errors.Wrap(err, "unable to convert 'before' changes")
		}
		after, err := AdminDbToRestModel(data.Changes.After)
		if err != nil {
			return errors.Wrap(err, "unable to convert 'after' changes")
		}
		e.Before = before
		e.After = after
	default:
		return fmt.Errorf("%T is not the correct event type", h)
	}

	return nil
}

func (e *APIAdminEvent) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for APIAdminEvent")
}
