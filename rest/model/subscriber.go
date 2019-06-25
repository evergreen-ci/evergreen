package model

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
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

type APIGithubMergeSubscriber struct {
	PRs         []APIPRInfo `json:"prs" mapstructure:"prs"`
	Item        APIString   `json:"item" mapstructure:"item"`
	MergeMethod APIString   `json:"merge_method" mapstructure:"merge_method"`
}

type APIPRInfo struct {
	Owner       APIString `json:"owner" mapstructure:"owner"`
	Repo        APIString `json:"repo" mapstructure:"repo"`
	PRNumber    int       `json:"pr_number" mapstructure:"pr_number"`
	Ref         APIString `json:"ref" mapstructure:"ref"`
	CommitTitle APIString `json:"commit_title" mapstructure:"commit_title"`
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
		case event.GithubMergeSubscriberType:
			sub := APIGithubMergeSubscriber{}
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
			event.SlackSubscriberType, event.CommitQueueDequeueSubscriberType:
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
			err = errors.Wrap(err, "GitHub PR subscriber target is malformed")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			err = errors.Wrap(err, "can't read subscriber target from API model")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}

	case event.GithubMergeSubscriberType:
		apiModel := APIGithubMergeSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			err = errors.Wrap(err, "GitHub merge subscriber target is malformed")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			err = errors.Wrap(err, "can't read subscriber target from API model")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}

	case event.EvergreenWebhookSubscriberType:
		apiModel := APIWebhookSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			err = errors.Wrap(err, "webhook subscriber target is malformed")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			err = errors.Wrap(err, "can't read subscriber target from API model")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}

	case event.JIRAIssueSubscriberType:
		apiModel := APIJIRAIssueSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			err = errors.Wrap(err, "JIRA issue subscriber target is malformed")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		target, err = apiModel.ToService()
		if err != nil {
			err = errors.Wrap(err, "can't read subscriber target from API model")
			grip.Error(err)
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}

	case event.JIRACommentSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.CommitQueueDequeueSubscriberType:
		target = s.Target

	default:
		err = errors.Errorf("unknown subscriber type: '%s'", FromAPIString(s.Type))
		grip.Error(err)
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	out.Target = target
	return out, nil
}

func (s *APIGithubPRSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.GithubPullRequestSubscriber:
		s.Owner = ToAPIString(v.Owner)
		s.Repo = ToAPIString(v.Repo)
		s.Ref = ToAPIString(v.Ref)
		s.PRNumber = v.PRNumber

	default:
		return errors.Errorf("type '%T' does not match subscriber type APIGithubPRSubscriber", v)
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

func (s *APIGithubMergeSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.GithubMergeSubscriber:
		s.MergeMethod = ToAPIString(v.MergeMethod)
		s.Item = ToAPIString(v.Item)
		s.PRs = make([]APIPRInfo, 0, len(v.PRs))
		for _, pr := range v.PRs {
			apiPR := APIPRInfo{}
			if err := apiPR.BuildFromService(pr); err != nil {
				return errors.Wrap(err, "can't build PR from service")
			}
			s.PRs = append(s.PRs, apiPR)
		}
	default:
		return errors.Errorf("type '%T' does not match subscriber type APIGithubMergeSubscriber", v)
	}

	return nil
}

func (s *APIGithubMergeSubscriber) ToService() (interface{}, error) {
	prs := make([]event.PRInfo, 0, len(s.PRs))
	for _, apiPR := range s.PRs {
		pr, err := apiPR.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "can't convert PR to service")
		}
		prs = append(prs, pr.(event.PRInfo))
	}

	return event.GithubMergeSubscriber{
		PRs:         prs,
		MergeMethod: FromAPIString(s.MergeMethod),
		Item:        FromAPIString(s.Item),
	}, nil
}

func (s *APIPRInfo) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case event.PRInfo:
		s.Owner = ToAPIString(v.Owner)
		s.Repo = ToAPIString(v.Repo)
		s.PRNumber = v.PRNum
		s.Ref = ToAPIString(v.Ref)
		s.CommitTitle = ToAPIString(v.CommitTitle)
	}

	return nil
}

func (s *APIPRInfo) ToService() (interface{}, error) {
	return event.PRInfo{
		Owner:       FromAPIString(s.Owner),
		Repo:        FromAPIString(s.Repo),
		PRNum:       s.PRNumber,
		Ref:         FromAPIString(s.Ref),
		CommitTitle: FromAPIString(s.CommitTitle),
	}, nil
}

func (s *APIWebhookSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.WebhookSubscriber:
		s.URL = ToAPIString(v.URL)
		s.Secret = ToAPIString(string(v.Secret))

	default:
		return errors.Errorf("type '%T' does not match subscriber type APIWebhookSubscriber", v)
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
	switch v := h.(type) {
	case *event.JIRAIssueSubscriber:
		s.Project = ToAPIString(v.Project)
		s.IssueType = ToAPIString(v.IssueType)

	default:
		return errors.Errorf("type '%T' does not match subscriber type APIJIRAIssueSubscriber", v)
	}

	return nil
}

func (s *APIJIRAIssueSubscriber) ToService() (interface{}, error) {
	return event.JIRAIssueSubscriber{
		Project:   FromAPIString(s.Project),
		IssueType: FromAPIString(s.IssueType),
	}, nil
}
