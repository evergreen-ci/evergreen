package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent, queue amboy.Queue) error {
	patchDoc := intent.NewPatch()
	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch)
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

type MockPatchIntentKey struct {
	intentType string
	msgID      string
}

type MockPatchIntentConnector struct {
	CachedIntents map[MockPatchIntentKey]patch.Intent
}

func (p *MockPatchIntentConnector) AddPatchIntent(newIntent patch.Intent, _ amboy.Queue) error {
	key := MockPatchIntentKey{newIntent.GetType(), newIntent.ID()}
	if _, ok := p.CachedIntents[key]; ok {
		return errors.New("intent with msg_id already exists")
	}
	p.CachedIntents[key] = newIntent

	return nil
}
