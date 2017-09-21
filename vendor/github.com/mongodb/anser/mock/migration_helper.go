package mock

import (
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
)

type MigrationHelper struct {
	Environment             Environment
	MigrationEvents         []*model.MigrationMetadata
	SaveMigrationEventError error
	NumPendingMigrations    int
	GetMigrationEventsIter  *Iterator
	GetMigrationEventsError error
}

func (m *MigrationHelper) Env() Environment { return m.Environment }

func (m *MigrationHelper) FinishMigration(name string, j *job.Base) {
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

func (m *MigrationHelper) SaveMigrationEvent(data *model.MigrationMetadata) error {
	m.MigrationEvents = append(m.MigrationEvents, data)

	return m.SaveMigrationEventError
}

func (m *MigrationHelper) PendingMigrationOperations(_ model.Namespace, _ map[string]interface{}) int {
	return m.NumPendingMigrations
}

func (m *MigrationHelper) GetMigrationEvents(_ map[string]interface{}) (db.Iterator, error) {
	return m.GetMigrationEventsIter, m.GetMigrationEventsError
}
