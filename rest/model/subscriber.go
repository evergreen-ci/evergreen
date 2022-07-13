package model

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type APISubscriber struct {
	Type                *string                 `json:"type"`
	Target              interface{}             `json:"target"`
	WebhookSubscriber   *APIWebhookSubscriber   `json:"-"`
	JiraIssueSubscriber *APIJIRAIssueSubscriber `json:"-"`
}

type APIGithubPRSubscriber struct {
	Owner    *string `json:"owner" mapstructure:"owner"`
	Repo     *string `json:"repo" mapstructure:"repo"`
	PRNumber int     `json:"pr_number" mapstructure:"pr_number"`
	Ref      *string `json:"ref" mapstructure:"ref"`
}

type APIGithubCheckSubscriber struct {
	Owner *string `json:"owner" mapstructure:"owner"`
	Repo  *string `json:"repo" mapstructure:"repo"`
	Ref   *string `json:"ref" mapstructure:"ref"`
}

type APIPRInfo struct {
	Owner       *string `json:"owner" mapstructure:"owner"`
	Repo        *string `json:"repo" mapstructure:"repo"`
	PRNumber    int     `json:"pr_number" mapstructure:"pr_number"`
	Ref         *string `json:"ref" mapstructure:"ref"`
	CommitTitle *string `json:"commit_title" mapstructure:"commit_title"`
}

type APIWebhookSubscriber struct {
	URL     *string            `json:"url" mapstructure:"url"`
	Secret  *string            `json:"secret" mapstructure:"secret"`
	Headers []APIWebhookHeader `json:"headers" mapstructure:"headers"`
}

type APIWebhookHeader struct {
	Key   *string `json:"key" mapstructure:"key"`
	Value *string `json:"value" mapstructure:"value"`
}

func (s *APISubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.Subscriber:
		s.Type = utility.ToStringPtr(v.Type)
		var target interface{}

		switch v.Type {
		case event.GithubPullRequestSubscriberType:
			sub := APIGithubPRSubscriber{}
			err := sub.BuildFromService(v.Target)
			if err != nil {
				return err
			}
			target = sub
		case event.GithubCheckSubscriberType:
			sub := APIGithubCheckSubscriber{}
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
			event.SlackSubscriberType, event.EnqueuePatchSubscriberType:
			target = v.Target

		default:
			return errors.Errorf("unknown subscriber type '%s'", v.Type)
		}

		s.Target = target

	default:
		return errors.Errorf("programmatic error: expected event subscriber but got type %T", h)
	}

	return nil
}

func (s *APISubscriber) ToService() (interface{}, error) {
	var target interface{}
	var err error
	out := event.Subscriber{
		Type: utility.FromStringPtr(s.Type),
	}
	switch utility.FromStringPtr(s.Type) {

	case event.GithubPullRequestSubscriberType:
		apiModel := APIGithubPRSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "GitHub PR subscriber target is malformed").Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting GitHub PR subscriber to service model").Error(),
			}
		}
	case event.GithubCheckSubscriberType:
		apiModel := APIGithubCheckSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "GitHub check subscriber target is malformed").Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting GitHub check subscriber to service model").Error(),
			}
		}

	case event.EvergreenWebhookSubscriberType:
		apiModel := APIWebhookSubscriber{}
		if s.WebhookSubscriber != nil {
			apiModel = *s.WebhookSubscriber
		} else {
			if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    errors.Wrap(err, "webhook subscriber target is malformed").Error(),
				}
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting webhook subscriber to service model").Error(),
			}
		}

	case event.JIRAIssueSubscriberType:
		apiModel := APIJIRAIssueSubscriber{}
		if s.JiraIssueSubscriber != nil {
			apiModel = *s.JiraIssueSubscriber
		} else {
			if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    errors.Wrap(err, "Jira issue subscriber target is malformed").Error(),
				}
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting Jira issue subscriber to service model").Error(),
			}
		}

	case event.JIRACommentSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.EnqueuePatchSubscriberType:
		target = s.Target

	default:
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("unknown subscriber type '%s'", utility.FromStringPtr(s.Type)),
		}
	}

	out.Target = target
	return out, nil
}

func (s *APIGithubPRSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.GithubPullRequestSubscriber:
		s.Owner = utility.ToStringPtr(v.Owner)
		s.Repo = utility.ToStringPtr(v.Repo)
		s.Ref = utility.ToStringPtr(v.Ref)
		s.PRNumber = v.PRNumber

	default:
		return errors.Errorf("programmatic error: expected GitHub PR subscriber but got type %T", h)
	}

	return nil
}

func (s *APIGithubCheckSubscriber) ToService() (interface{}, error) {
	return event.GithubCheckSubscriber{
		Owner: utility.FromStringPtr(s.Owner),
		Repo:  utility.FromStringPtr(s.Repo),
		Ref:   utility.FromStringPtr(s.Ref),
	}, nil
}

func (s *APIGithubCheckSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.GithubCheckSubscriber:
		s.Owner = utility.ToStringPtr(v.Owner)
		s.Repo = utility.ToStringPtr(v.Repo)
		s.Ref = utility.ToStringPtr(v.Ref)

	default:
		return errors.Errorf("programmatic error: expected GitHub check subscriber but got type %T", h)
	}

	return nil
}

func (s *APIGithubPRSubscriber) ToService() (interface{}, error) {
	return event.GithubPullRequestSubscriber{
		Owner:    utility.FromStringPtr(s.Owner),
		Repo:     utility.FromStringPtr(s.Repo),
		Ref:      utility.FromStringPtr(s.Ref),
		PRNumber: s.PRNumber,
	}, nil
}

func (s *APIPRInfo) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.PRInfo:
		s.Owner = utility.ToStringPtr(v.Owner)
		s.Repo = utility.ToStringPtr(v.Repo)
		s.PRNumber = v.PRNum
		s.Ref = utility.ToStringPtr(v.Ref)
		s.CommitTitle = utility.ToStringPtr(v.CommitTitle)
	}

	return nil
}

func (s *APIPRInfo) ToService() (interface{}, error) {
	return event.PRInfo{
		Owner:       utility.FromStringPtr(s.Owner),
		Repo:        utility.FromStringPtr(s.Repo),
		PRNum:       s.PRNumber,
		Ref:         utility.FromStringPtr(s.Ref),
		CommitTitle: utility.FromStringPtr(s.CommitTitle),
	}, nil
}

func (s *APIWebhookSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.WebhookSubscriber:
		s.URL = utility.ToStringPtr(v.URL)
		s.Secret = utility.ToStringPtr(string(v.Secret))
		s.Headers = []APIWebhookHeader{}
		for _, header := range v.Headers {
			apiHeader := APIWebhookHeader{}
			apiHeader.BuildFromService(header)
			s.Headers = append(s.Headers, apiHeader)
		}

	default:
		return errors.Errorf("programmatic error: expected webhook subscriber but got type %T", v)
	}

	return nil
}

func (s *APIWebhookSubscriber) ToService() (interface{}, error) {
	sub := event.WebhookSubscriber{
		URL:     utility.FromStringPtr(s.URL),
		Secret:  []byte(utility.FromStringPtr(s.Secret)),
		Headers: []event.WebhookHeader{},
	}
	for _, apiHeader := range s.Headers {
		sub.Headers = append(sub.Headers, apiHeader.ToService())
	}
	return sub, nil
}

func (s *APIWebhookHeader) BuildFromService(h event.WebhookHeader) {
	s.Key = &h.Key
	s.Value = &h.Value
}

func (s *APIWebhookHeader) ToService() event.WebhookHeader {
	return event.WebhookHeader{
		Key:   *s.Key,
		Value: *s.Value,
	}
}

type APIJIRAIssueSubscriber struct {
	Project   *string `json:"project" mapstructure:"project"`
	IssueType *string `json:"issue_type" mapstructure:"issue_type"`
}

func (s *APIJIRAIssueSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.JIRAIssueSubscriber:
		s.Project = utility.ToStringPtr(v.Project)
		s.IssueType = utility.ToStringPtr(v.IssueType)

	default:
		return errors.Errorf("programmatic error: expected Jira issue subscriber but got type %T", h)
	}

	return nil
}

func (s *APIJIRAIssueSubscriber) ToService() (interface{}, error) {
	return event.JIRAIssueSubscriber{
		Project:   utility.FromStringPtr(s.Project),
		IssueType: utility.FromStringPtr(s.IssueType),
	}, nil
}
