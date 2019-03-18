package units

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type commitQueueSuite struct {
	suite.Suite
	env *mock.Environment
	ctx context.Context

	prBody     []byte
	pr         *github.PullRequest
	projectRef *model.ProjectRef
}

func TestCommitQueueJob(t *testing.T) {
	suite.Run(t, &commitQueueSuite{})
}

func (s *commitQueueSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Require().Len(s.prBody, 24757)

	s.projectRef = &model.ProjectRef{
		Identifier: "mci",
		Owner:      "baxterthehacker",
		Repo:       "public-repo",
		CommitQueue: model.CommitQueueParams{
			Enabled:     true,
			MergeMethod: "squash",
		},
	}
	s.Require().NoError(s.projectRef.Insert())

	s.env = &mock.Environment{}
	s.ctx = context.Background()
	s.NoError(s.env.Configure(s.ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
}

func (s *commitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, user.Collection))

	webhookInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	prEvent, ok := webhookInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.pr = prEvent.GetPullRequest()

	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "1",
			},
			commitqueue.CommitQueueItem{
				Issue: "2",
			},
			commitqueue.CommitQueueItem{
				Issue: "3",
			},
			commitqueue.CommitQueueItem{
				Issue: "4",
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
}

func (s *commitQueueSuite) TestNewCommitQueueJob() {
	job := NewCommitQueueJob(s.env, "mci", "job-1")
	s.Equal("commit-queue:mci_job-1", job.ID())
}

func (s *commitQueueSuite) TestSubscribeMerge() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))
	s.NoError(subscribeMerge(s.projectRef.Identifier, s.projectRef.Owner, s.projectRef.Repo, "squash", "abcdef", s.pr))

	selectors := []event.Selector{
		event.Selector{
			Type: "id",
			Data: "abcdef",
		},
	}
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, selectors)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)

	subscription := subscriptions[0]

	s.Equal(event.GithubMergeSubscriberType, subscription.Subscriber.Type)
	s.Equal(event.ResourceTypePatch, subscription.ResourceType)
	target, ok := subscription.Subscriber.Target.(*event.GithubMergeSubscriber)
	s.True(ok)
	s.Equal(s.projectRef.Identifier, target.ProjectID)
	s.Equal(s.projectRef.Owner, target.Owner)
	s.Equal(s.projectRef.Repo, target.Repo)
	s.Equal(s.pr.GetTitle(), target.CommitTitle)
}

func (s *commitQueueSuite) TestWritePatchInfo() {
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	patchDoc := &patch.Patch{
		Id:      bson.ObjectIdHex("aabbccddeeff112233445566"),
		Githash: "abcdef",
	}
	config := &model.Project{
		Enabled: true,
	}

	patchSummaries := []patch.Summary{
		patch.Summary{
			Name:      "myfile.go",
			Additions: 1,
			Deletions: 0,
		},
	}

	patchContent := `diff --git a/myfile.go b/myfile.go
	index abcdef..123456 100644
	--- a/myfile.go
	+++ b/myfile.go
	@@ +2,1 @@ func myfunc {
	+				fmt.Print(\"hello world\")
			}
	`

	s.NoError(writePatchInfo(patchDoc, config, patchSummaries, patchContent))
	s.Len(patchDoc.Patches, 1)
	s.Equal(patchSummaries, patchDoc.Patches[0].PatchSet.Summary)
	reader, err := db.GetGridFile(patch.GridFSPrefix, patchDoc.Patches[0].PatchSet.PatchFileId)
	s.NoError(err)
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	s.NoError(err)
	s.Equal(patchContent, string(bytes))
}
