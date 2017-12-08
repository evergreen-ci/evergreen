package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/pkg/errors"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent) error {
	if err := intent.Insert(); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "couldn't insert patch intent",
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

func (p *MockPatchIntentConnector) AddPatchIntent(newIntent patch.Intent) error {
	key := MockPatchIntentKey{newIntent.GetType(), newIntent.ID()}
	if _, ok := p.CachedIntents[key]; ok {
		return errors.New("intent with msg_id already exists")
	}
	p.CachedIntents[key] = newIntent

	return nil
}
