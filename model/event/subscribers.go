package event

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	GithubPullRequestSubscriberType = "github_pull_request"
	JIRAIssueSubscriberType         = "jira-issue"
	JIRACommentSubscriberType       = "jira-comment"
	EvergreenWebhookSubscriberType  = "evergreen-webhook"
	EmailSubscriberType             = "email"
	SlackSubscriberType             = "slack"
	GithubMergeSubscriberType       = "github-merge"
)

var SubscriberTypes = []string{
	GithubPullRequestSubscriberType,
	JIRAIssueSubscriberType,
	JIRACommentSubscriberType,
	EvergreenWebhookSubscriberType,
	EmailSubscriberType,
	SlackSubscriberType,
	GithubMergeSubscriberType,
}

//nolint: deadcode, megacheck, unused
var (
	subscriberTypeKey   = bsonutil.MustHaveTag(Subscriber{}, "Type")
	subscriberTargetKey = bsonutil.MustHaveTag(Subscriber{}, "Target")
)

type Subscriber struct {
	Type string `bson:"type"`
	// sad violin
	Target interface{} `bson:"target"`
}

type unmarshalSubscriber struct {
	Type   string   `bson:"type"`
	Target bson.Raw `bson:"target"`
}

func (s *Subscriber) SetBSON(raw bson.Raw) error {
	temp := unmarshalSubscriber{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal subscriber data")
	}
	if len(temp.Type) == 0 {
		return errors.New("could not find subscriber type")
	}
	s.Type = temp.Type

	switch s.Type {
	case GithubPullRequestSubscriberType:
		s.Target = &GithubPullRequestSubscriber{}

	case EvergreenWebhookSubscriberType:
		s.Target = &WebhookSubscriber{}

	case JIRAIssueSubscriberType:
		s.Target = &JIRAIssueSubscriber{}

	case JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType:
		str := ""
		s.Target = &str

	case GithubMergeSubscriberType:
		s.Target = &GithubMergeSubscriber{}

	default:
		return errors.Errorf("unknown subscriber type: '%s'", s.Type)
	}

	if err := temp.Target.Unmarshal(s.Target); err != nil {
		return errors.Wrap(err, "couldn't unmarshal subscriber info")
	}

	return nil
}

func (s *Subscriber) String() string {
	subscriberStr := "NIL_SUBSCRIBER"

	switch v := s.Target.(type) {
	case GithubPullRequestSubscriber:
		subscriberStr = v.String()
	case *GithubPullRequestSubscriber:
		subscriberStr = v.String()

	case GithubMergeSubscriber:
		subscriberStr = v.String()
	case *GithubMergeSubscriber:
		subscriberStr = v.String()

	case WebhookSubscriber:
		subscriberStr = v.String()
	case *WebhookSubscriber:
		subscriberStr = v.String()

	case JIRAIssueSubscriber:
		subscriberStr = v.String()
	case *JIRAIssueSubscriber:
		subscriberStr = v.String()

	case string:
		subscriberStr = v
	case *string:
		subscriberStr = *v
	}

	return fmt.Sprintf("%s-%s", s.Type, subscriberStr)
}

func (s *Subscriber) Validate() error {
	catcher := grip.NewBasicCatcher()
	if !util.StringSliceContains(SubscriberTypes, s.Type) {
		catcher.Add(errors.Errorf("%s is not a valid subscriber type", s.Type))
	}
	if s.Target == nil {
		catcher.Add(errors.New("type is required for subscriber"))
	}
	return catcher.Resolve()
}

type WebhookSubscriber struct {
	URL    string `bson:"url"`
	Secret []byte `bson:"secret"`
}

func (s *WebhookSubscriber) String() string {
	if len(s.URL) == 0 {
		return "NIL_URL"
	}
	return s.URL
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
}

func (s *GithubPullRequestSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%d-%s", s.Owner, s.Repo, s.PRNumber, s.Ref)
}

type GithubMergeSubscriber struct {
	ProjectID     string `bson:"project_id"`
	Owner         string `bson:"owner"`
	Repo          string `bson:"repo"`
	PRNumber      int    `bson:"pr_number"`
	Ref           string `bson:"ref"`
	CommitMessage string `bson:"commit_message"`
	MergeMethod   string `bson:"merge_method"`
	CommitTitle   string `bson:"commit_title"`
}

func (s *GithubMergeSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%s-%d-%s-%s-%s-%s",
		s.ProjectID,
		s.Owner,
		s.Repo,
		s.PRNumber,
		s.Ref,
		s.CommitMessage,
		s.MergeMethod,
		s.CommitTitle,
	)
}

func NewGithubMergeSubscriber(s GithubMergeSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubMergeSubscriberType,
		Target: s,
	}
}

func NewGithubStatusAPISubscriber(s GithubPullRequestSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubPullRequestSubscriberType,
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
