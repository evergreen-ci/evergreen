package anser

import (
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
)

// this has to be implemeneted in the anser package because of the
// interface dependency between MigrationHelper and Environment.

type MigrationHelperMock struct {
	Environment             *mock.Environment
	MigrationEvents         []*model.MigrationMetadata
	SaveMigrationEventError error
	NumPendingMigrations    int
	GetMigrationEventsIter  *mock.Iterator
	GetMigrationEventsError error
}

func (m *MigrationHelperMock) Env() Environment { return m.Environment }

func (m *MigrationHelperMock) FinishMigration(name string, j *job.Base) {
	j.MarkComplete()
	meta := model.MigrationMetadata{
		ID:        j.ID(),
		Migration: name,
		HasErrors: j.HasErrors(),
		Completed: true,
	}
	err := m.SaveMigrationEvent(&meta)
	if err != nil {
		j.AddError(err)
		grip.Warningf("encountered problem [%s] saving migration metadata", err.Error())
	}
}

func (m *MigrationHelperMock) SaveMigrationEvent(data *model.MigrationMetadata) error {
	m.MigrationEvents = append(m.MigrationEvents, data)

	return m.SaveMigrationEventError
}

func (m *MigrationHelperMock) PendingMigrationOperations(_ model.Namespace, _ map[string]interface{}) int {
	return m.NumPendingMigrations
}

func (m *MigrationHelperMock) GetMigrationEvents(_ map[string]interface{}) (db.Iterator, error) {
	return m.GetMigrationEventsIter, m.GetMigrationEventsError
}
