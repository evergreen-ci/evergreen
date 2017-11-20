package data

import (
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent) error {
	return intent.Insert()
}

type MockPatchIntentKey struct {
	intentType string
	msgId      string
}

type MockPatchIntentConnector struct {
	CachedIntents map[MockPatchIntentKey]patch.Intent
}

func (p *MockPatchIntentConnector) AddPatchIntent(newIntent patch.Intent) error {
	key := MockPatchIntentKey{newIntent.GetType(), newIntent.UniqueId()}
	if _, ok := p.CachedIntents[key]; ok {
		return errors.New("intent with msg_id already exists")
	}
	p.CachedIntents[key] = newIntent

	return nil
}
