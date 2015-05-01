package repotracker

import (
	"github.com/evergreen-ci/evergreen/model"
)

// MockRepoPoller is a utility for testing the repotracker using a dummy
// project
type mockRepoPoller struct {
	project   *model.Project
	revisions []model.Revision

	ConfigGets uint
	nextError  error
}

// Creates a new MockRepo poller with the given project settings
func NewMockRepoPoller(mockProject *model.Project, mockRevisions []model.Revision) *mockRepoPoller {
	return &mockRepoPoller{
		project: mockProject,
	}
}

func (d *mockRepoPoller) setNextError(err error) {
	d.nextError = err
}

func (d *mockRepoPoller) clearError() (err error) {
	err, d.nextError = d.nextError, nil
	return
}

func (d *mockRepoPoller) GetRemoteConfig(revision string) (*model.Project, error) {
	d.ConfigGets++
	if d.nextError != nil {
		return nil, d.clearError()
	}
	return d.project, nil
}

// TODO: Maybe actually make these work at some point?
func (d *mockRepoPoller) GetRevisionsSince(revision string, maxRevisionsToSearch int) (
	[]model.Revision, error) {
	if d.nextError != nil {
		return nil, d.clearError()
	}
	return d.revisions, nil
}

func (d *mockRepoPoller) GetRecentRevisions(maxRevisionsToSearch int) (
	[]model.Revision, error) {
	if d.nextError != nil {
		return nil, d.clearError()
	}
	return d.revisions, nil
}
