package model

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type APISubscriber struct {
	Type   APIString   `json:"type"`
	Target interface{} `json:"target"`
}

type APIGithubPRSubscriber struct {
	Owner    APIString `json:"owner" mapstructure:"owner"`
	Repo     APIString `json:"repo" mapstructure:"repo"`
	PRNumber int       `json:"pr_number" mapstructure:"pr_number"`
	Ref      APIString `json:"ref" mapstructure:"ref"`
}

type APIWebhookSubscriber struct {
	URL    APIString `json:"url" mapstructure:"url"`
	Secret APIString `json:"secret" mapstructure:"secret"`
}

func (s *APISubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {

	case event.Subscriber:
		s.Type = ToAPIString(v.Type)
		var target interface{}

		switch v.Type {
		case event.GithubPullRequestSubscriberType:
			sub := APIGithubPRSubscriber{}
			err := sub.BuildFromService(v.Target)
			if err != nil {
				return err
			}
			target = sub

		case event.EvergreenWebhookSubscriberType:
			sub := APIWebhookSubscriber{}
			err := sub.BuildFromService(v.Target)
			if err != nil {
				return err
			}
			target = sub

		case event.JIRAIssueSubscriberType:
			sub := APIJIRAIssueSubscriber{}
			err := sub.BuildFromService(v.Target)
			if err != nil {
				return err
			}
			target = sub

		case event.JIRACommentSubscriberType, event.EmailSubscriberType,
			event.SlackSubscriberType:
			target = v.Target

		default:
			return errors.Errorf("unknown subscriber type: '%s'", v.Type)
		}

		s.Target = target

	default:
		return errors.New("unknown type for APISubscriber")
	}

	return nil
}

func (s *APISubscriber) ToService() (interface{}, error) {
	var target interface{}
	var err error
	out := event.Subscriber{
		Type: FromAPIString(s.Type),
	}
	switch FromAPIString(s.Type) {

	case event.GithubPullRequestSubscriberType:
		apiModel := APIGithubPRSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Subscriber target is malformed",
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, err
		}

	case event.EvergreenWebhookSubscriberType:
		apiModel := APIWebhookSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("webhook subscriber is malformed: %s", err.Error()),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, err
		}

	case event.JIRAIssueSubscriberType:
		apiModel := APIJIRAIssueSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("jira issue subscriber is malformed: %s", err.Error()),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, err
		}

	case event.JIRACommentSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType:
		target = s.Target

	default:
		return nil, errors.Errorf("unknown subscriber type: '%s'", FromAPIString(s.Type))
	}

	out.Target = target
	return out, nil
}

func (s *APIGithubPRSubscriber) BuildFromService(h interface{}) error {
	if v, ok := h.(event.GithubPullRequestSubscriber); ok {
		h = &v
	}

	switch v := h.(type) {
	case *event.GithubPullRequestSubscriber:
		s.Owner = ToAPIString(v.Owner)
		s.Repo = ToAPIString(v.Repo)
		s.Ref = ToAPIString(v.Ref)
		s.PRNumber = v.PRNumber

	default:
		return errors.New("unknown type for APIGithubPRSubscriber")
	}

	return nil
}

func (s *APIGithubPRSubscriber) ToService() (interface{}, error) {
	return event.GithubPullRequestSubscriber{
		Owner:    FromAPIString(s.Owner),
		Repo:     FromAPIString(s.Repo),
		Ref:      FromAPIString(s.Ref),
		PRNumber: s.PRNumber,
	}, nil
}

func (s *APIWebhookSubscriber) BuildFromService(h interface{}) error {
	if v, ok := h.(event.WebhookSubscriber); ok {
		h = &v
	}

	switch v := h.(type) {
	case *event.WebhookSubscriber:
		s.URL = ToAPIString(v.URL)
		s.Secret = ToAPIString(string(v.Secret))

	default:
		return errors.New("unknown type for APIWebhookSubscriber")
	}

	return nil
}

func (s *APIWebhookSubscriber) ToService() (interface{}, error) {
	return event.WebhookSubscriber{
		URL:    FromAPIString(s.URL),
		Secret: []byte(FromAPIString(s.Secret)),
	}, nil
}

type APIJIRAIssueSubscriber struct {
	Project   APIString `json:"project" mapstructure:"project"`
	IssueType APIString `json:"issue_type" mapstructure:"issue_type"`
}

func (s *APIJIRAIssueSubscriber) BuildFromService(h interface{}) error {
	if v, ok := h.(event.JIRAIssueSubscriber); ok {
		h = &v
	}

	switch v := h.(type) {
	case *event.JIRAIssueSubscriber:
		s.Project = ToAPIString(v.Project)
		s.IssueType = ToAPIString(v.IssueType)

	default:
		return errors.New("unknown type for APIWebhookSubscriber")
	}

	return nil
}

func (s *APIJIRAIssueSubscriber) ToService() (interface{}, error) {
	return event.JIRAIssueSubscriber{
		Project:   FromAPIString(s.Project),
		IssueType: FromAPIString(s.IssueType),
	}, nil
}
