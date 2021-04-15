package event

import (
	"fmt"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	GithubPullRequestSubscriberType = "github_pull_request"
	GithubCheckSubscriberType       = "github_check"
	JIRAIssueSubscriberType         = "jira-issue"
	JIRACommentSubscriberType       = "jira-comment"
	EvergreenWebhookSubscriberType  = "evergreen-webhook"
	EmailSubscriberType             = "email"
	SlackSubscriberType             = "slack"
	EnqueuePatchSubscriberType      = "enqueue-patch"
	SubscriberTypeNone              = "none"
	RunChildPatchSubscriberType     = "run-child-patch"
)

var SubscriberTypes = []string{
	GithubPullRequestSubscriberType,
	GithubCheckSubscriberType,
	JIRAIssueSubscriberType,
	JIRACommentSubscriberType,
	EvergreenWebhookSubscriberType,
	EmailSubscriberType,
	SlackSubscriberType,
	EnqueuePatchSubscriberType,
	RunChildPatchSubscriberType,
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
	Type   string      `bson:"type"`
	Target mgobson.Raw `bson:"target"`
}

func (s *Subscriber) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *Subscriber) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

func (s *Subscriber) SetBSON(raw mgobson.Raw) error {
	temp := unmarshalSubscriber{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal subscriber data")
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
	case EvergreenWebhookSubscriberType:
		s.Target = &WebhookSubscriber{}
	case JIRAIssueSubscriberType:
		s.Target = &JIRAIssueSubscriber{}
	case JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType:
		str := ""
		s.Target = &str
	case RunChildPatchSubscriberType:
		s.Target = &ChildPatchSubscriber{}
	case EnqueuePatchSubscriberType:
		s.Target = nil
		return nil

	default:
		return errors.Errorf("unknown subscriber type: '%s'", temp.Type)
	}

	if err := temp.Target.Unmarshal(s.Target); err != nil {
		return errors.Wrap(err, "couldn't unmarshal subscriber info")
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
	if !utility.StringSliceContains(SubscriberTypes, s.Type) {
		catcher.Add(errors.Errorf("%s is not a valid subscriber type", s.Type))
	}
	if s.Target == nil {
		catcher.Add(errors.New("target is required for subscriber"))
	}
	return catcher.Resolve()
}

type WebhookSubscriber struct {
	URL     string          `bson:"url"`
	Secret  []byte          `bson:"secret"`
	Headers []WebhookHeader `bson:"headers"`
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
	Type     string `bson:"type"`
}

const (
	WaitOnChild           = "wait-on-child"
	SendChildPatchOutcome = "send-child-patch-outcome"
)

func (s *GithubPullRequestSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%d-%s-%s", s.Owner, s.Repo, s.PRNumber, s.Ref, s.ChildId)
}

type GithubCheckSubscriber struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`
	Ref   string `bson:"ref"`
}

type ChildPatchSubscriber struct {
	ParentStatus string `bson:"parent_status"`
	ChildPatchId string `bson:"child_patch_id"`
	Requester    string `bson:"requester"`
}

func (s *GithubCheckSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%s", s.Owner, s.Repo, s.Ref)
}

func NewEnqueuePatchSubscriber() Subscriber {
	return Subscriber{
		Type:   EnqueuePatchSubscriberType,
		Target: nil,
	}
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
