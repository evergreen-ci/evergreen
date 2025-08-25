package event

import (
	"fmt"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	GithubPullRequestSubscriberType = "github_pull_request"
	GithubCheckSubscriberType       = "github_check"
	GithubMergeSubscriberType       = "github_merge"
	JIRAIssueSubscriberType         = "jira-issue"
	JIRACommentSubscriberType       = "jira-comment"
	EvergreenWebhookSubscriberType  = "evergreen-webhook"
	EmailSubscriberType             = "email"
	SlackSubscriberType             = "slack"
	SubscriberTypeNone              = "none"
	RunChildPatchSubscriberType     = "run-child-patch"

	webhookRetryLimit    = 10
	webhookMinDelayLimit = 10000
	webhookTimeoutLimit  = 30000
)

var SubscriberTypes = []string{
	GithubPullRequestSubscriberType,
	GithubCheckSubscriberType,
	GithubMergeSubscriberType,
	JIRAIssueSubscriberType,
	JIRACommentSubscriberType,
	EvergreenWebhookSubscriberType,
	EmailSubscriberType,
	SlackSubscriberType,
	RunChildPatchSubscriberType,
}

type Subscriber struct {
	Type string `bson:"type"`
	// sad violin
	Target any `bson:"target"`
}

type unmarshalSubscriber struct {
	Type   string      `bson:"type"`
	Target mgobson.Raw `bson:"target"`
}

func (s *Subscriber) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *Subscriber) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

func (s *Subscriber) SetBSON(raw mgobson.Raw) error {
	temp := unmarshalSubscriber{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "unmarshalling subscriber data")
	}
	if len(temp.Type) == 0 {
		return errors.New("could not find subscriber type")
	}
	s.Type = temp.Type

	switch temp.Type {
	case GithubPullRequestSubscriberType:
		s.Target = &GithubPullRequestSubscriber{}
	case GithubCheckSubscriberType:
		s.Target = &GithubCheckSubscriber{}
	case GithubMergeSubscriberType:
		s.Target = &GithubMergeSubscriber{}
	case EvergreenWebhookSubscriberType:
		s.Target = &WebhookSubscriber{}
	case JIRAIssueSubscriberType:
		s.Target = &JIRAIssueSubscriber{}
	case JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType:
		str := ""
		s.Target = &str
	case RunChildPatchSubscriberType:
		s.Target = &ChildPatchSubscriber{}

	default:
		return errors.Errorf("unknown subscriber type '%s'", temp.Type)
	}

	if err := temp.Target.Unmarshal(s.Target); err != nil {
		return errors.Wrap(err, "unmarshalling subscriber info")
	}

	return nil
}

func (s *Subscriber) String() string {
	var subscriberStr string
	switch v := s.Target.(type) {
	case string:
		subscriberStr = v
	case *string:
		subscriberStr = *v
	case fmt.Stringer:
		subscriberStr = v.String()
	default:
		subscriberStr = "NIL_SUBSCRIBER"
	}

	return fmt.Sprintf("%s-%s", s.Type, subscriberStr)
}

func (s *Subscriber) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.ErrorfWhen(!utility.StringSliceContains(SubscriberTypes, s.Type), "'%s' is not a valid subscriber type", s.Type)
	catcher.NewWhen(s.Target == nil, "target is required for subscriber")

	switch v := s.Target.(type) {
	case WebhookSubscriber:
		catcher.Add(v.validate())
	case *WebhookSubscriber:
		catcher.Add(v.validate())
	}

	return catcher.Resolve()
}

type WebhookSubscriber struct {
	URL        string          `bson:"url"`
	Secret     []byte          `bson:"secret"`
	Retries    int             `bson:"retries"`
	MinDelayMS int             `bson:"min_delay_ms"`
	TimeoutMS  int             `bson:"timeout_ms"`
	Headers    []WebhookHeader `bson:"headers"`
}

type WebhookHeader struct {
	Key   string `bson:"key"`
	Value string `bson:"value"`
}

func (s *WebhookSubscriber) String() string {
	if len(s.URL) == 0 {
		return "NIL_URL"
	}
	return s.URL
}

func (s *WebhookSubscriber) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.AddWhen(s.URL == "", errors.New("url cannot be empty"))
	catcher.AddWhen(len(s.Secret) == 0, errors.New("secret cannot be empty"))

	catcher.AddWhen(s.Retries < 0, errors.New("retries cannot be negative"))
	catcher.AddWhen(s.Retries > webhookRetryLimit, errors.Errorf("cannot retry more than %d times", webhookRetryLimit))

	catcher.AddWhen(s.MinDelayMS < 0, errors.New("min delay cannot be negative"))
	catcher.AddWhen(s.MinDelayMS > webhookMinDelayLimit, errors.Errorf("min delay cannot be greater than %d ms", webhookMinDelayLimit))

	catcher.AddWhen(s.TimeoutMS < 0, errors.New("timeout cannot be negative"))
	catcher.AddWhen(s.TimeoutMS > webhookTimeoutLimit, errors.Errorf("timeout cannot be greater than %d ms", webhookTimeoutLimit))

	for _, header := range s.Headers {
		catcher.AddWhen(header.Key == "", errors.New("header key cannot be empty"))
		catcher.AddWhen(header.Value == "", errors.New("header value cannot be empty"))
	}

	return catcher.Resolve()
}

// GetHeader gets the value for the given key.
func (s *WebhookSubscriber) GetHeader(key string) string {
	for _, h := range s.Headers {
		if h.Key == key {
			return h.Value
		}
	}
	return ""
}

type JIRAIssueSubscriber struct {
	Project   string `bson:"project"`
	IssueType string `bson:"issue_type"`
}

func (s *JIRAIssueSubscriber) String() string {
	return fmt.Sprintf("%s-%s", s.Project, s.IssueType)
}

type GithubPullRequestSubscriber struct {
	Owner    string `bson:"owner"`
	Repo     string `bson:"repo"`
	PRNumber int    `bson:"pr_number"`
	Ref      string `bson:"ref"`
	ChildId  string `bson:"child"`
}

func (s *GithubPullRequestSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%d-%s-%s", s.Owner, s.Repo, s.PRNumber, s.Ref, s.ChildId)
}

type GithubCheckSubscriber struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`
	Ref   string `bson:"ref"`
}

func (s *GithubCheckSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%s", s.Owner, s.Repo, s.Ref)
}

type GithubMergeSubscriber struct {
	Owner   string `bson:"owner"`
	Repo    string `bson:"repo"`
	Ref     string `bson:"ref"`
	ChildId string `bson:"child"`
}

func (s *GithubMergeSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%s-%s", s.Owner, s.Repo, s.Ref, s.ChildId)
}

type ChildPatchSubscriber struct {
	ParentStatus string `bson:"parent_status"`
	ChildPatchId string `bson:"child_patch_id"`
	Requester    string `bson:"requester"`
}

func NewRunChildPatchSubscriber(s ChildPatchSubscriber) Subscriber {
	return Subscriber{
		Type:   RunChildPatchSubscriberType,
		Target: s,
	}
}

func NewGithubStatusAPISubscriber(s GithubPullRequestSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubPullRequestSubscriberType,
		Target: s,
	}
}

func NewGithubCheckAPISubscriber(s GithubCheckSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubCheckSubscriberType,
		Target: s,
	}
}

func NewGithubMergeAPISubscriber(s GithubMergeSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubMergeSubscriberType,
		Target: s,
	}
}

func NewEmailSubscriber(t string) Subscriber {
	return Subscriber{
		Type:   EmailSubscriberType,
		Target: t,
	}
}

func NewSlackSubscriber(t string) Subscriber {
	return Subscriber{
		Type:   SlackSubscriberType,
		Target: t,
	}
}
