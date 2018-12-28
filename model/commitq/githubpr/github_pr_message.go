package githubpr

import (
	"fmt"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type GithubMergeMethod string

// valid Github merge methods
const (
	GithubMergeMethodMerge  = GithubMergeMethod("merge")
	GithubMergeMethodSquash = GithubMergeMethod("squash")
	GithubMergeMethodRebase = GithubMergeMethod("rebase")
)

type GithubMergePR struct {
	ProjectID   string            `bson:"projectID,omitempty" json:"projectID,omitempty" yaml:"projectID,omitempty"`
	Owner       string            `bson:"owner,omitempty" json:"owner,omitempty" yaml:"owner,omitempty"`
	Repo        string            `bson:"repo,omitempty" json:"repo,omitempty" yaml:"repo,omitempty"`
	Ref         string            `bson:"ref,omitempty" json:"ref,omitempty" yaml:"ref,omitempty"`
	PRNum       int               `bson:"PR_num,omitempty" json:"PR_num,omitempty" yaml:"PR_num,omitempty"`
	CommitMsg   string            `bson:"commit_msg,omitempty" json:"commit_msg,omitempty" yaml:"commit_msg,omitempty"`
	CommitTitle string            `bson:"commit_title,omitempty" json:"commit_title,omitempty" yaml:"commit_title,omitempty"`
	MergeMethod GithubMergeMethod `bson:"merge_method,omitempty" json:"merge_method,omitempty" yaml:"merge_method,omitempty"`
}

// Valid returns true if the message is well formed
func (p *GithubMergePR) Valid() bool {
	// owner, repo and ref must be empty or must be set
	projectIDEmpty := len(p.ProjectID) == 0
	ownerEmpty := len(p.Owner) == 0
	repoEmpty := len(p.Repo) == 0
	commitMsgEmpty := len(p.CommitMsg) == 0
	refEmpty := len(p.Ref) == 0
	if projectIDEmpty || ownerEmpty || repoEmpty || commitMsgEmpty || refEmpty {
		return false
	}

	if p.PRNum <= 0 {
		return false
	}

	if len(p.MergeMethod) > 0 {
		switch p.MergeMethod {
		case GithubMergeMethodMerge, GithubMergeMethodSquash, GithubMergeMethodRebase:
		default:
			return false
		}
	}

	return true
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
	_ = s.SetPriority(p)

	return s
}

func (c *githubMergePRMessage) Loggable() bool {
	return c.raw.Valid()
}

func (c *githubMergePRMessage) String() string {
	str := fmt.Sprintf("Merge Pull Request #%d (Ref: %s) for %s on %s/%s: %s", c.raw.PRNum, c.raw.Ref, c.raw.ProjectID, c.raw.Owner, c.raw.Repo, c.raw.CommitMsg)
	if len(c.raw.CommitTitle) > 0 {
		str += fmt.Sprintf(". Commit Title: %s", c.raw.CommitTitle)
	}
	if len(c.raw.MergeMethod) > 0 {
		str += fmt.Sprintf(". Merge Method: %s", c.raw.MergeMethod)
	}

	return str
}

func (c *githubMergePRMessage) Raw() interface{} {
	return &c.raw
}
