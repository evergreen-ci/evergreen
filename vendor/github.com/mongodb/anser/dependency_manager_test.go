package anser

import (
	"errors"
	"testing"

	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type DependencyManagerSuite struct {
	dep    *migrationDependency
	helper *MigrationHelperMock
	suite.Suite
}

func TestDependencyManagerSuite(t *testing.T) {
	suite.Run(t, new(DependencyManagerSuite))
}

func (s *DependencyManagerSuite) SetupTest() {
	s.dep = makeMigrationDependencyManager()
	s.helper = &MigrationHelperMock{}
	s.dep.MigrationHelper = s.helper
}

func (s *DependencyManagerSuite) TestFactory() {
	factory, err := registry.GetDependencyFactory("anser-migration")
	s.NoError(err)
	dep, ok := factory().(*migrationDependency)
	s.True(ok)
	dep.MigrationHelper = s.helper
	s.Equal(dep, s.dep)
}

func (s *DependencyManagerSuite) TestDefaultTypeInfo() {
	s.Zero(s.dep.MigrationID)
	s.Equal(s.dep.Type().Name, "anser-migration")
}

func (s *DependencyManagerSuite) TestStateIsPassedIfNoPendingMigrations() {
	// this is the default state of the mock
	s.Equal(s.dep.State(), dependency.Passed)
}

func (s *DependencyManagerSuite) TestStateIsUnresolvedIfPendingMigrationsError() {
	s.helper.NumPendingMigrations = -1
	s.Equal(s.dep.State(), dependency.Unresolved)
}

func (s *DependencyManagerSuite) TestNoEdgesReported() {
	s.helper.NumPendingMigrations = 2
	s.Equal(s.dep.State(), dependency.Ready)
}

func (s *DependencyManagerSuite) TestEdgeQueryReturnsError() {
	s.helper.NumPendingMigrations = 2
	s.helper.GetMigrationEventsError = errors.New("problem")
	s.NoError(s.dep.AddEdge("foo"))
	s.Equal(s.dep.State(), dependency.Blocked)
}

func (s *DependencyManagerSuite) TestEdgeQueryReturnsNoResults() {
	s.helper.NumPendingMigrations = 2
	s.helper.GetMigrationEventsIter = &mock.Iterator{}
	s.NoError(s.dep.AddEdge("foo"))
	// the outcome is blocked because processEdges() ends up
	// having seen 0 documents, but there are two edges pending
	s.Equal(s.dep.State(), dependency.Blocked)
}

func TestDependencyStateQuery(t *testing.T) {
	assert := assert.New(t)
	keys := []string{"foo", "bar"}
	query := getDependencyStateQuery(keys)

	assert.Len(query, 1)
	idClause, ok := query["_id"]
	assert.True(ok)
	assert.Len(idClause, 1)
	inClause := idClause.(bson.M)["$in"].([]string)
	assert.Len(inClause, 2)
}

func TestDependencyEdgeProcessing(t *testing.T) {
	assert := assert.New(t)

	// if we don't iterate (the default), then the number of edges of 1 will
	// always be greater than 0, resulting  in a blocked state
	assert.Equal(dependency.Blocked, processEdges(1, &mock.Iterator{}))

	// if there are -1 edges (not possible in normal situations,
	// but...) then we will have seen in the iteration more
	// dependencies (e.g. 0) than the number of edges (0)
	assert.Equal(dependency.Ready, processEdges(-1, &mock.Iterator{}))

	// if the close method returns an error, then it's blocked.
	assert.Equal(dependency.Blocked, processEdges(-1, &mock.Iterator{Error: errors.New("blocked")}))

	assert.True((&model.MigrationMetadata{Completed: true, HasErrors: false}).Satisfied())
	assert.False((&model.MigrationMetadata{}).Satisfied())

	iter := &mock.Iterator{ShouldIter: true, Results: []interface{}{
		&model.MigrationMetadata{ID: "four", Completed: true, HasErrors: false},
		&model.MigrationMetadata{ID: "five", Completed: true, HasErrors: false},
		&model.MigrationMetadata{ID: "six", Completed: true, HasErrors: false},
		&model.MigrationMetadata{ID: "seven"},
	}}
	assert.Equal(dependency.Blocked, processEdges(1, iter))
}
