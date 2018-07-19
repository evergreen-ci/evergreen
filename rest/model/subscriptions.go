package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/model/event"
)

type APISelector struct {
	Type APIString `json:"type"`
	Data APIString `json:"data"`
}

type APISubscription struct {
	ID             APIString         `json:"id"`
	ResourceType   APIString         `json:"resource_type"`
	Trigger        APIString         `json:"trigger"`
	Selectors      []APISelector     `json:"selectors"`
	RegexSelectors []APISelector     `json:"regex_selectors"`
	Subscriber     APISubscriber     `json:"subscriber"`
	OwnerType      APIString         `json:"owner_type"`
	Owner          APIString         `json:"owner"`
	TriggerData    map[string]string `json:"trigger_data,omitempty"`
}

func (s *APISelector) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.Selector:
		s.Data = ToAPIString(v.Data)
		s.Type = ToAPIString(v.Type)
	default:
		return errors.New("unrecognized type for APISelector")
	}

	return nil
}

func (s *APISelector) ToService() (interface{}, error) {
	return event.Selector{
		Data: FromAPIString(s.Data),
		Type: FromAPIString(s.Type),
	}, nil
}

func (s *APISubscription) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.Subscription:
		s.ID = ToAPIString(v.ID)
		s.ResourceType = ToAPIString(v.Type)
		s.Trigger = ToAPIString(v.Trigger)
		s.Owner = ToAPIString(v.Owner)
		s.OwnerType = ToAPIString(string(v.OwnerType))
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
		ID:             FromAPIString(s.ID),
		Type:           FromAPIString(s.ResourceType),
		Trigger:        FromAPIString(s.Trigger),
		Owner:          FromAPIString(s.Owner),
		OwnerType:      event.OwnerType(FromAPIString(s.OwnerType)),
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
