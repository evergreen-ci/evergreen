package model

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type APISubscriber struct {
	Type *string `json:"type"`
	// Target can be either a slice or a string. However, since swaggo does not
	// support the OpenAPI `oneOf` keyword, we set `swaggerignore` and document
	// the field manually in the "Fetch all projects" endpoint.
	Target              interface{}             `json:"target" swaggerignore:"true"`
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

type APIGithubMergeSubscriber struct {
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
	URL        *string            `json:"url" mapstructure:"url"`
	Secret     *string            `json:"secret" mapstructure:"secret"`
	Retries    int                `json:"retries" mapstructure:"retries"`
	MinDelayMS int                `json:"min_delay_ms" mapstructure:"min_delay_ms"`
	TimeoutMS  int                `json:"timeout_ms" mapstructure:"timeout_ms"`
	Headers    []APIWebhookHeader `json:"headers" mapstructure:"headers"`
}

type APIWebhookHeader struct {
	Key   *string `json:"key" mapstructure:"key"`
	Value *string `json:"value" mapstructure:"value"`
}

// BuildFromService for APISubscriber needs to return an error so that we can validate the target interface type.
func (s *APISubscriber) BuildFromService(in event.Subscriber) error {
	s.Type = utility.ToStringPtr(in.Type)
	var target interface{}

	switch in.Type {
	case event.GithubPullRequestSubscriberType:
		sub := APIGithubPRSubscriber{}
		err := sub.BuildFromService(in.Target)
		if err != nil {
			return err
		}
		target = sub

	case event.GithubCheckSubscriberType:
		sub := APIGithubCheckSubscriber{}
		err := sub.BuildFromService(in.Target)
		if err != nil {
			return err
		}
		target = sub

	case event.GithubMergeSubscriberType:
		sub := APIGithubMergeSubscriber{}
		err := sub.BuildFromService(in.Target)
		if err != nil {
			return err
		}
		target = sub

	case event.EvergreenWebhookSubscriberType:
		sub := APIWebhookSubscriber{}
		err := sub.BuildFromService(in.Target)
		if err != nil {
			return err
		}
		target = sub
		s.WebhookSubscriber = &sub

	case event.JIRAIssueSubscriberType:
		sub := APIJIRAIssueSubscriber{}
		err := sub.BuildFromService(in.Target)
		if err != nil {
			return err
		}
		target = sub
		s.JiraIssueSubscriber = &sub

	case event.JIRACommentSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.EnqueuePatchSubscriberType:
		target = in.Target

	default:
		return errors.Errorf("unknown subscriber type '%s'", in.Type)
	}

	s.Target = target
	return nil
}

func (s *APISubscriber) ToService() (event.Subscriber, error) {
	var target interface{}
	var err error
	out := event.Subscriber{
		Type: utility.FromStringPtr(s.Type),
	}
	switch utility.FromStringPtr(s.Type) {
	case event.GithubPullRequestSubscriberType:
		apiModel := APIGithubPRSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return event.Subscriber{}, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "GitHub PR subscriber target is malformed").Error(),
			}
		}
		target = apiModel.ToService()

	case event.GithubCheckSubscriberType:
		apiModel := APIGithubCheckSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return event.Subscriber{}, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "GitHub check subscriber target is malformed").Error(),
			}
		}
		target = apiModel.ToService()

	case event.GithubMergeSubscriberType:
		apiModel := APIGithubMergeSubscriber{}
		if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
			return event.Subscriber{}, gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "GitHub merge subscriber target is malformed").Error(),
			}
		}
		target = apiModel.ToService()

	case event.EvergreenWebhookSubscriberType:
		apiModel := APIWebhookSubscriber{}
		if s.WebhookSubscriber != nil {
			apiModel = *s.WebhookSubscriber
		} else {
			if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
				return event.Subscriber{}, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    errors.Wrap(err, "webhook subscriber target is malformed").Error(),
				}
			}
		}
		target = apiModel.ToService()

	case event.JIRAIssueSubscriberType:
		apiModel := APIJIRAIssueSubscriber{}
		if s.JiraIssueSubscriber != nil {
			apiModel = *s.JiraIssueSubscriber
		} else {
			if err = mapstructure.Decode(s.Target, &apiModel); err != nil {
				return event.Subscriber{}, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    errors.Wrap(err, "Jira issue subscriber target is malformed").Error(),
				}
			}
		}
		target = apiModel.ToService()

	case event.JIRACommentSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.EnqueuePatchSubscriberType:
		target = s.Target

	default:
		return event.Subscriber{}, gimlet.ErrorResponse{
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

func (s *APIGithubCheckSubscriber) ToService() event.GithubCheckSubscriber {
	return event.GithubCheckSubscriber{
		Owner: utility.FromStringPtr(s.Owner),
		Repo:  utility.FromStringPtr(s.Repo),
		Ref:   utility.FromStringPtr(s.Ref),
	}
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

func (s *APIGithubMergeSubscriber) ToService() event.GithubMergeSubscriber {
	return event.GithubMergeSubscriber{
		Owner: utility.FromStringPtr(s.Owner),
		Repo:  utility.FromStringPtr(s.Repo),
		Ref:   utility.FromStringPtr(s.Ref),
	}
}

func (s *APIGithubMergeSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.GithubMergeSubscriber:
		s.Owner = utility.ToStringPtr(v.Owner)
		s.Repo = utility.ToStringPtr(v.Repo)
		s.Ref = utility.ToStringPtr(v.Ref)

	default:
		return errors.Errorf("programmatic error: expected GitHub merge subscriber but got type %T", h)
	}

	return nil
}

func (s *APIGithubPRSubscriber) ToService() event.GithubPullRequestSubscriber {
	return event.GithubPullRequestSubscriber{
		Owner:    utility.FromStringPtr(s.Owner),
		Repo:     utility.FromStringPtr(s.Repo),
		Ref:      utility.FromStringPtr(s.Ref),
		PRNumber: s.PRNumber,
	}
}

func (s *APIPRInfo) BuildFromService(info event.PRInfo) {
	s.Owner = utility.ToStringPtr(info.Owner)
	s.Repo = utility.ToStringPtr(info.Repo)
	s.PRNumber = info.PRNum
	s.Ref = utility.ToStringPtr(info.Ref)
	s.CommitTitle = utility.ToStringPtr(info.CommitTitle)
}

func (s *APIPRInfo) ToService() event.PRInfo {
	return event.PRInfo{
		Owner:       utility.FromStringPtr(s.Owner),
		Repo:        utility.FromStringPtr(s.Repo),
		PRNum:       s.PRNumber,
		Ref:         utility.FromStringPtr(s.Ref),
		CommitTitle: utility.FromStringPtr(s.CommitTitle),
	}
}

func (s *APIWebhookSubscriber) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *event.WebhookSubscriber:
		s.URL = utility.ToStringPtr(v.URL)
		s.Secret = utility.ToStringPtr(evergreen.RedactedValue)
		s.Headers = []APIWebhookHeader{}
		s.Retries = v.Retries
		s.MinDelayMS = v.MinDelayMS
		s.TimeoutMS = v.TimeoutMS
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

func (s *APIWebhookSubscriber) ToService() event.WebhookSubscriber {
	sub := event.WebhookSubscriber{
		URL:        utility.FromStringPtr(s.URL),
		Secret:     []byte(utility.FromStringPtr(s.Secret)),
		Headers:    []event.WebhookHeader{},
		Retries:    s.Retries,
		MinDelayMS: s.MinDelayMS,
		TimeoutMS:  s.TimeoutMS,
	}
	for _, apiHeader := range s.Headers {
		sub.Headers = append(sub.Headers, apiHeader.ToService())
	}
	return sub
}

func (s *APIWebhookHeader) BuildFromService(h event.WebhookHeader) {
	s.Key = &h.Key
	if h.Key == "Authorization" {
		s.Value = utility.ToStringPtr(evergreen.RedactedValue)
	} else {
		s.Value = &h.Value
	}
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

func (s *APIJIRAIssueSubscriber) ToService() event.JIRAIssueSubscriber {
	return event.JIRAIssueSubscriber{
		Project:   utility.FromStringPtr(s.Project),
		IssueType: utility.FromStringPtr(s.IssueType),
	}
}
