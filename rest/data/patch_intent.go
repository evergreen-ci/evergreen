package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent, queue amboy.Queue) error {
	if err := intent.Insert(); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "couldn't insert patch intent",
		}
	}

	if err := queue.Put(units.NewPatchIntentProcessor(bson.NewObjectId(), intent)); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to queue patch intent for processing",
		}
	}

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
