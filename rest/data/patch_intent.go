package data

import (
	"context"
	"net/http"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func AddPatchIntent(intent patch.Intent, queue amboy.Queue) error {
	patchDoc := intent.NewPatch()
	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch, intent.GetCalledBy())
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to fetch project_ref",
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

	job := units.NewPatchIntentProcessor(mgobson.NewObjectId(), intent)
	job.SetPriority(1)
	if err := queue.Put(context.TODO(), job); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":    "github hook",
			"message":   "Github pull request not queued for processing",
			"intent_id": intent.ID(),
		}))

		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to queue patch intent for processing",
		}
	}

	grip.Info(message.Fields{
		"message":     "Github pull request queued",
		"intent_type": intent.GetType(),
		"intent_id":   intent.ID(),
	})

	return nil
}
