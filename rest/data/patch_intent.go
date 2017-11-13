package data

import (
	"github.com/evergreen-ci/evergreen/model/patch"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent) error {
	return intent.Insert()
}

type MockPatchIntentConnector struct {
	CachedIntents []patch.Intent
}

func (p *MockPatchIntentConnector) AddPatchIntent(intent patch.Intent) error {
	// TODO: duplicates?
	p.CachedIntents = append(p.CachedIntents, intent)
	return nil
}
