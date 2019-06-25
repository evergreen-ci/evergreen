package commitqueue

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

// valid Github merge methods
const (
	githubMergeMethodMerge  = "merge"
	githubMergeMethodSquash = "squash"
	githubMergeMethodRebase = "rebase"
)

type GithubMergePR struct {
	Status      string         `bson:"status"`
	PatchID     string         `bson:"patch_id"`
	URL         string         `bson:"url"`
	ProjectID   string         `bson:"project_id"`
	MergeMethod string         `bson:"merge_method"`
	Item        string         `bson:"item"`
	PRs         []event.PRInfo `bson:"prs"`
}

func (m *GithubMergePR) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(m) }
func (m *GithubMergePR) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, m) }

// Valid returns nil if the message is well formed
func (p *GithubMergePR) Valid() error {
	catcher := grip.NewBasicCatcher()
	if len(p.ProjectID) == 0 {
		catcher.Add(errors.New("Project ID can't be empty"))
	}
	if len(p.Status) == 0 {
		catcher.Add(errors.New("Status can't be empty"))
	}
	if len(p.Item) == 0 {
		catcher.Add(errors.New("item can't be empty"))
	}
	for _, pr := range p.PRs {
		if len(pr.Owner) == 0 {
			catcher.Add(errors.New("Owner can't be empty"))
		}
		if len(pr.Repo) == 0 {
			catcher.Add(errors.New("Repo can't be empty"))
		}
		if len(pr.Ref) == 0 {
			catcher.Add(errors.New("Ref can't be empty"))
		}
		if pr.PRNum <= 0 {
			catcher.Add(errors.New("Invalid pull request number"))
		}
	}

	if len(p.MergeMethod) > 0 {
		switch p.MergeMethod {
		case githubMergeMethodMerge, githubMergeMethodSquash, githubMergeMethodRebase:
		default:
			catcher.Add(errors.New("Invalid merge method"))
		}
	}

	return catcher.Resolve()
}

type githubMergePRMessage struct {
	raw          GithubMergePR
	message.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// NewGithubMergePRMessage returns a composer for GithubMergePR messages
func NewGithubMergePRMessage(p level.Priority, mergeMsg GithubMergePR) message.Composer {
	s := &githubMergePRMessage{
		raw: mergeMsg,
	}
	if err := s.SetPriority(p); err != nil {
		_ = s.SetPriority(level.Notice)
	}

	return s
}

func (c *githubMergePRMessage) Loggable() bool {
	return c.raw.Valid() == nil
}

func (c *githubMergePRMessage) String() string {
	return fmt.Sprintf("GitHub commit queue merge '%s'", c.raw.Item)
}

func (c *githubMergePRMessage) Raw() interface{} {
	return &c.raw
}
