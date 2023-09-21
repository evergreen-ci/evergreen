package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// AddPatchIntent inserts the intent and adds it to the queue if PR testing is enabled for the branch.
func AddPatchIntent(intent patch.Intent, queue amboy.Queue) error {
	// Verify that the owner/repo uses PR testing before inserting the intent.
	patchDoc := intent.NewPatch()
	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch, intent.GetCalledBy())
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding project ref for patch").Error(),
		}
	}
	if projectRef == nil {
		return nil
	}

	if err := intent.Insert(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "couldn't insert patch intent",
			"id":        intent.ID(),
			"type":      intent.GetType(),
			"requester": intent.RequesterIdentity(),
		}))
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "couldn't insert patch intent",
		}
	}

	job := units.NewPatchIntentProcessor(evergreen.GetEnvironment(), mgobson.NewObjectId(), intent)
	if err := queue.Put(context.Background(), job); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":    "GitHub hook",
			"message":   "GitHub pull request not queued for processing",
			"intent_id": intent.ID(),
		}))

		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "enqueueing patch intent for processing",
		}
	}

	grip.Info(message.Fields{
		"message":     "GitHub pull request queued",
		"intent_type": intent.GetType(),
		"intent_id":   intent.ID(),
	})

	return nil
}

func AddGithubMergeIntent(intent patch.Intent, queue amboy.Queue) error {
	patchDoc := intent.NewPatch()
	projectRef, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(patchDoc.GithubMergeData.Org,
		patchDoc.GithubMergeData.Repo, patchDoc.GithubMergeData.BaseBranch)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding project ref for GitHub merge group").Error(),
		}
	}
	if projectRef == nil {
		return nil
	}

	if err := intent.Insert(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "couldn't insert GitHub merge group intent",
			"id":        intent.ID(),
			"type":      intent.GetType(),
			"requester": intent.RequesterIdentity(),
		}))
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "couldn't insert GitHub merge group intent",
		}
	}

	job := units.NewPatchIntentProcessor(evergreen.GetEnvironment(), mgobson.NewObjectId(), intent)
	if err := queue.Put(context.Background(), job); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":    "GitHub hook",
			"message":   "GitHub merge group not queued for processing",
			"intent_id": intent.ID(),
		}))

		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "enqueueing GitHub merge group for processing",
		}
	}

	grip.Info(message.Fields{
		"message":     "GitHub merge group queued",
		"intent_type": intent.GetType(),
		"intent_id":   intent.ID(),
	})

	return nil
}
