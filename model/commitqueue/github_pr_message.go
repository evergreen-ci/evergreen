package commitqueue

import (
	"fmt"

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
	PatchSucceeded bool   `bson:"patch_succeeded"`
	URL            string `bson:"url"`
	ProjectID      string `bson:"project_id"`
	Owner          string `bson:"owner"`
	Repo           string `bson:"repo"`
	Ref            string `bson:"ref"`
	PRNum          int    `bson:"pr_number"`
	CommitMessage  string `bson:"commit_message"`
	CommitTitle    string `bson:"commit_title"`
	MergeMethod    string `bson:"merge_method"`
}

func (m *GithubMergePR) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(m) }
func (m *GithubMergePR) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, m) }

// Valid returns nil if the message is well formed
func (p *GithubMergePR) Valid() error {
	catcher := grip.NewBasicCatcher()
	if len(p.ProjectID) == 0 {
		catcher.Add(errors.New("Project ID can't be empty"))
	}
	if len(p.Owner) == 0 {
		catcher.Add(errors.New("Owner can't be empty"))
	}
	if len(p.Repo) == 0 {
		catcher.Add(errors.New("Repo can't be empty"))
	}
	if len(p.CommitMessage) == 0 {
		catcher.Add(errors.New("Commit message can't be empty"))
	}
	if len(p.Ref) == 0 {
		catcher.Add(errors.New("Ref can't be empty"))
	}

	if p.PRNum <= 0 {
		catcher.Add(errors.New("Invalid pull request number"))
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
	str := fmt.Sprintf("Merge Pull Request #%d (Ref: %s) on %s/%s: %s", c.raw.PRNum, c.raw.Ref, c.raw.Owner, c.raw.Repo, c.raw.CommitMessage)
	if len(c.raw.CommitTitle) > 0 {
		str = fmt.Sprintf("%s. Commit Title: %s", str, c.raw.CommitTitle)
	}
	if len(c.raw.MergeMethod) > 0 {
		str = fmt.Sprintf("%s. Merge Method: %s", str, c.raw.MergeMethod)
	}

	return str
}

func (c *githubMergePRMessage) Raw() interface{} {
	return &c.raw
}
