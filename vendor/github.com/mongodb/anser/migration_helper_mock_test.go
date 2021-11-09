package anser

import (
	"context"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
)

// this has to be implemented in the anser package because of the
// interface dependency between MigrationHelper and Environment.

type MigrationHelperMock struct {
	Environment             *mock.Environment
	MigrationEvents         []*model.MigrationMetadata
	SaveMigrationEventError error
	NumPendingMigrations    int
	GetMigrationEventsIter  MigrationMetadataIterator
	GetMigrationEventsError error
}

func (m *MigrationHelperMock) Env() Environment { return m.Environment }

func (m *MigrationHelperMock) FinishMigration(ctx context.Context, name string, j *job.Base) {
	j.MarkComplete()
	meta := model.MigrationMetadata{
		ID:        j.ID(),
		Migration: name,
		HasErrors: j.HasErrors(),
		Completed: true,
	}
	err := m.SaveMigrationEvent(ctx, &meta)
	if err != nil {
		j.AddError(err)
		grip.Warningf("encountered problem [%s] saving migration metadata", err.Error())
	}
}

func (m *MigrationHelperMock) SaveMigrationEvent(ctx context.Context, data *model.MigrationMetadata) error {
	m.MigrationEvents = append(m.MigrationEvents, data)

	return m.SaveMigrationEventError
}

func (m *MigrationHelperMock) PendingMigrationOperations(ctx context.Context, _ model.Namespace, _ map[string]interface{}) int {
	return m.NumPendingMigrations
}

func (m *MigrationHelperMock) GetMigrationEvents(ctx context.Context, _ map[string]interface{}) MigrationMetadataIterator {
	return m.GetMigrationEventsIter
}
