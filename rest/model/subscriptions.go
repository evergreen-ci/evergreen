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

func (s *APISelector) BuildFromService(selector event.Selector) {
	s.Data = utility.ToStringPtr(selector.Data)
	s.Type = utility.ToStringPtr(selector.Type)
}

func (s *APISelector) ToService() event.Selector {
	return event.Selector{
		Data: utility.FromStringPtr(s.Data),
		Type: utility.FromStringPtr(s.Type),
	}
}

func (s *APISubscription) BuildFromService(sub event.Subscription) error {
	s.ID = utility.ToStringPtr(sub.ID)
	s.ResourceType = utility.ToStringPtr(sub.ResourceType)
	s.Trigger = utility.ToStringPtr(sub.Trigger)
	s.Owner = utility.ToStringPtr(sub.Owner)
	s.OwnerType = utility.ToStringPtr(string(sub.OwnerType))
	s.TriggerData = sub.TriggerData
	err := s.Subscriber.BuildFromService(sub.Subscriber)
	if err != nil {
		return err
	}
	s.Selectors = []APISelector{}
	s.RegexSelectors = []APISelector{}
	for _, selector := range sub.Selectors {
		newSelector := APISelector{}
		newSelector.BuildFromService(selector)
		s.Selectors = append(s.Selectors, newSelector)
	}
	for _, selector := range sub.RegexSelectors {
		newSelector := APISelector{}
		newSelector.BuildFromService(selector)
		s.RegexSelectors = append(s.RegexSelectors, newSelector)
	}
	return nil
}

func (s *APISubscription) ToService() (event.Subscription, error) {
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
	subscriber, err := s.Subscriber.ToService()
	if err != nil {
		return event.Subscription{}, err
	}

	out.Subscriber = subscriber
	for _, selector := range s.Selectors {
		out.Selectors = append(out.Selectors, selector.ToService())
	}
	if err = out.Filter.FromSelectors(out.Selectors); err != nil {
		return event.Subscription{}, errors.Wrap(err, "setting filter from selectors")
	}

	for _, selector := range s.RegexSelectors {
		out.RegexSelectors = append(out.RegexSelectors, selector.ToService())
	}

	return out, nil
}
