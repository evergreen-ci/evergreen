package data

import (
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

type DBPatchIntentConnector struct{}

func (p *DBPatchIntentConnector) AddPatchIntent(intent patch.Intent) error {
	return intent.Insert()
}

type MockPatchIntentConnector struct {
	CachedIntents map[string]patch.Intent
}

func (p *MockPatchIntentConnector) AddPatchIntent(newIntent patch.Intent) error {
	switch intent := newIntent.(type) {
	case *patch.GithubIntent:
		if _, ok := p.CachedIntents[intent.MsgId]; ok {
			return errors.New("intent with msg_id already exists")
		}

		p.CachedIntents[intent.MsgId] = newIntent

	default:
		panic("unknown intent type")

	}
	return nil
}
