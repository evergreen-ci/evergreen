package model

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type APISelector struct {
	Type *string `json:"type"`
	Data *string `json:"data"`
}

type APISubscription struct {
	ID             *string           `json:"id"`
	ResourceType   *string           `json:"resource_type"`
	Trigger        *string           `json:"trigger"`
	Selectors      []APISelector     `json:"selectors"`
	RegexSelectors []APISelector     `json:"regex_selectors"`
	Subscriber     APISubscriber     `json:"subscriber"`
	OwnerType      *string           `json:"owner_type"`
	Owner          *string           `json:"owner"`
	TriggerData    map[string]string `json:"trigger_data,omitempty"`
}

func (s *APISelector) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.Selector:
		s.Data = utility.ToStringPtr(v.Data)
		s.Type = utility.ToStringPtr(v.Type)
	default:
		return errors.New("unrecognized type for APISelector")
	}

	return nil
}

func (s *APISelector) ToService() (interface{}, error) {
	return event.Selector{
		Data: utility.FromStringPtr(s.Data),
		Type: utility.FromStringPtr(s.Type),
	}, nil
}

func (s *APISubscription) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.Subscription:
		s.ID = utility.ToStringPtr(v.ID)
		s.ResourceType = utility.ToStringPtr(v.ResourceType)
		s.Trigger = utility.ToStringPtr(v.Trigger)
		s.Owner = utility.ToStringPtr(v.Owner)
		s.OwnerType = utility.ToStringPtr(string(v.OwnerType))
		s.TriggerData = v.TriggerData
		err := s.Subscriber.BuildFromService(v.Subscriber)
		if err != nil {
			return err
		}
		s.Selectors = []APISelector{}
		s.RegexSelectors = []APISelector{}
		for _, selector := range v.Selectors {
			newSelector := APISelector{}
			err = newSelector.BuildFromService(selector)
			if err != nil {
				return err
			}
			s.Selectors = append(s.Selectors, newSelector)
		}
		for _, selector := range v.RegexSelectors {
			newSelector := APISelector{}
			err = newSelector.BuildFromService(selector)
			if err != nil {
				return err
			}
			s.RegexSelectors = append(s.RegexSelectors, newSelector)
		}
	default:
		return errors.New("unrecognized type for APISubscription")
	}

	return nil
}

func (s *APISubscription) ToService() (interface{}, error) {
	out := event.Subscription{
		ID:             utility.FromStringPtr(s.ID),
		ResourceType:   utility.FromStringPtr(s.ResourceType),
		Trigger:        utility.FromStringPtr(s.Trigger),
		Owner:          utility.FromStringPtr(s.Owner),
		OwnerType:      event.OwnerType(utility.FromStringPtr(s.OwnerType)),
		Selectors:      []event.Selector{},
		RegexSelectors: []event.Selector{},
		TriggerData:    s.TriggerData,
	}
	subscriberInterface, err := s.Subscriber.ToService()
	if err != nil {
		return nil, err
	}
	subscriber, ok := subscriberInterface.(event.Subscriber)
	if !ok {
		return nil, errors.New("unable to convert subscriber")
	}
	out.Subscriber = subscriber
	for _, selector := range s.Selectors {
		selectorInterface, err := selector.ToService()
		if err != nil {
			return nil, err
		}
		newSelector, ok := selectorInterface.(event.Selector)
		if !ok {
			return nil, errors.New("unable to convert selector")
		}
		out.Selectors = append(out.Selectors, newSelector)
	}
	if err = out.Filter.FromSelectors(out.Selectors); err != nil {
		return nil, errors.Wrap(err, "setting filter from selectors")
	}

	for _, selector := range s.RegexSelectors {
		selectorInterface, err := selector.ToService()
		if err != nil {
			return nil, err
		}
		newSelector, ok := selectorInterface.(event.Selector)
		if !ok {
			return nil, errors.New("unable to convert selector")
		}
		out.RegexSelectors = append(out.RegexSelectors, newSelector)
	}

	return out, nil
}
