package event

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	GithubPullRequestSubscriberType  = "github_pull_request"
	JIRAIssueSubscriberType          = "jira-issue"
	JIRACommentSubscriberType        = "jira-comment"
	EvergreenWebhookSubscriberType   = "evergreen-webhook"
	EmailSubscriberType              = "email"
	SlackSubscriberType              = "slack"
	GithubMergeSubscriberType        = "github-merge"
	CommitQueueDequeueSubscriberType = "commit-queue-dequeue"
	SubscriberTypeNone               = "none"
)

var SubscriberTypes = []string{
	GithubPullRequestSubscriberType,
	JIRAIssueSubscriberType,
	JIRACommentSubscriberType,
	EvergreenWebhookSubscriberType,
	EmailSubscriberType,
	SlackSubscriberType,
	GithubMergeSubscriberType,
	CommitQueueDequeueSubscriberType,
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
	case EvergreenWebhookSubscriberType:
		s.Target = &WebhookSubscriber{}
	case JIRAIssueSubscriberType:
		s.Target = &JIRAIssueSubscriber{}
	case JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType:
		str := ""
		s.Target = &str
	case GithubMergeSubscriberType:
		s.Target = &GithubMergeSubscriber{}
	case CommitQueueDequeueSubscriberType:
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
	if !util.StringSliceContains(SubscriberTypes, s.Type) {
		catcher.Add(errors.Errorf("%s is not a valid subscriber type", s.Type))
	}
	if s.Target == nil {
		catcher.Add(errors.New("target is required for subscriber"))
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
	PRs         []PRInfo `bson:"prs"`
	Item        string   `bson:"item"`
	MergeMethod string   `bson:"merge_method"`
}

func (s *GithubMergeSubscriber) String() string {
	if len(s.PRs) == 0 {
		return "No PRs"
	}

	return fmt.Sprintf("%s-%s-%d-%s-%s-%s-%s",
		s.PRs[len(s.PRs)-1].Owner,
		s.PRs[len(s.PRs)-1].Repo,
		s.PRs[len(s.PRs)-1].PRNum,
		s.PRs[len(s.PRs)-1].Ref,
		s.PRs[len(s.PRs)-1].CommitTitle,
		s.MergeMethod,
		s.Item,
	)
}

func NewGithubMergeSubscriber(s GithubMergeSubscriber) Subscriber {
	return Subscriber{
		Type:   GithubMergeSubscriberType,
		Target: s,
	}
}

func NewCommitQueueDequeueSubscriber() Subscriber {
	return Subscriber{
		Type:   CommitQueueDequeueSubscriberType,
		Target: nil,
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
