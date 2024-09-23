package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// AddPRPatchIntent inserts the intent and adds it to the queue if PR testing is enabled for the branch.
func AddPRPatchIntent(intent patch.Intent, queue amboy.Queue) error {
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
