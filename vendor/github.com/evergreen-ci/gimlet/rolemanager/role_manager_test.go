package rolemanager

import (
	"context"
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestRoleManager(t *testing.T) {
	dbName := "gimlet"
	roleCollection := "roles"
	scopeCollection := "scopes"
	client, err := mongo.NewClient()
	require.NoError(t, err)
	require.NoError(t, client.Connect(context.Background()))

	dbManager := NewMongoBackedRoleManager(MongoBackedRoleManagerOpts{
		Client:          client,
		DBName:          dbName,
		RoleCollection:  roleCollection,
		ScopeCollection: scopeCollection,
	})
	require.NoError(t, client.Database(dbName).Collection(roleCollection).Drop(context.Background()))
	require.NoError(t, client.Database(dbName).Collection(scopeCollection).Drop(context.Background()))
	memManager := NewInMemoryRoleManager()

	toTest := map[string]gimlet.RoleManager{
		"mongo-backed": dbManager,
		"in-memory":    memManager,
	}
	for name, m := range toTest {
		t.Run(name, testSingleManager(t, m))
	}
}

func testSingleManager(t *testing.T, m gimlet.RoleManager) func(*testing.T) {
	return func(t *testing.T) {
		s := &RoleManagerSuite{
			m: m,
		}
		suite.Run(t, s)
	}
}

type RoleManagerSuite struct {
	suite.Suite
	m gimlet.RoleManager
}

func (s *RoleManagerSuite) SetupSuite() {
	scope1 := gimlet.Scope{
		ID:          "1",
		Resources:   []string{"resource1", "resource2"},
		ParentScope: "3",
	}
	s.NoError(s.m.AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:          "2",
		Resources:   []string{"resource3"},
		ParentScope: "3",
	}
	s.NoError(s.m.AddScope(scope2))
	scope3 := gimlet.Scope{
		ID:          "3",
		ParentScope: "root",
	}
	s.NoError(s.m.AddScope(scope3))
	scope4 := gimlet.Scope{
		ID:          "4",
		Resources:   []string{"resource4"},
		ParentScope: "root",
	}
	s.NoError(s.m.AddScope(scope4))
	root := gimlet.Scope{
		ID: "root",
	}
	s.NoError(s.m.AddScope(root))

	permissions := []string{"edit", "read"}
	s.NoError(s.m.RegisterPermissions(permissions))
	s.Error(s.m.RegisterPermissions(permissions))
}

func (s *RoleManagerSuite) SetupTest() {
	roles, err := s.m.GetAllRoles()
	s.NoError(err)
	for _, role := range roles {
		s.NoError(s.m.DeleteRole(role.ID))
	}
}

func (s *RoleManagerSuite) TestGetAndUpdate() {
	role1 := gimlet.Role{
		ID:   "r1",
		Name: "role1",
		Permissions: map[string]int{
			"edit": 2,
		},
		Owners: []string{"me"},
	}
	s.NoError(s.m.UpdateRole(role1))
	dbRoles, err := s.m.GetRoles([]string{role1.ID})
	s.NoError(err)
	s.Equal(role1.Name, dbRoles[0].Name)
	s.Equal(role1.Permissions, dbRoles[0].Permissions)
	s.Equal(role1.Owners, dbRoles[0].Owners)
}

func (s *RoleManagerSuite) TestFilterForResource() {
	role1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
	}
	s.NoError(s.m.UpdateRole(role1))
	role2 := gimlet.Role{
		ID:    "r2",
		Scope: "2",
	}
	s.NoError(s.m.UpdateRole(role2))
	role3 := gimlet.Role{
		ID:    "r3",
		Scope: "3",
	}
	s.NoError(s.m.UpdateRole(role3))
	role4 := gimlet.Role{
		ID:    "r4",
		Scope: "4",
	}
	s.NoError(s.m.UpdateRole(role4))
	roleRoot := gimlet.Role{
		ID:    "rRoot",
		Scope: "root",
	}
	s.NoError(s.m.UpdateRole(roleRoot))
	allRoles := []gimlet.Role{role1, role2, role3, role4, roleRoot}

	filtered, err := s.m.FilterForResource(allRoles, "resource1")
	s.NoError(err)
	s.Equal([]gimlet.Role{role1, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource2")
	s.NoError(err)
	s.Equal([]gimlet.Role{role1, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource3")
	s.NoError(err)
	s.Equal([]gimlet.Role{role2, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource4")
	s.NoError(err)
	s.Equal([]gimlet.Role{role4, roleRoot}, filtered)
}
