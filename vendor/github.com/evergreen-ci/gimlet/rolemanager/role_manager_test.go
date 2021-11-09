package rolemanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/mux"
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
	permissions := []string{"edit", "read"}
	s.NoError(s.m.RegisterPermissions(permissions))
	s.Error(s.m.RegisterPermissions(permissions))
}

func (s *RoleManagerSuite) SetupTest() {
	s.Require().NoError(s.m.Clear())
	root := gimlet.Scope{
		ID:        "root",
		Type:      "project",
		Resources: []string{},
	}
	s.Require().NoError(s.m.AddScope(root))
	scope3 := gimlet.Scope{
		ID:          "3",
		ParentScope: "root",
		Type:        "project",
		Resources:   []string{},
	}
	s.Require().NoError(s.m.AddScope(scope3))
	scope1 := gimlet.Scope{
		ID:          "1",
		Resources:   []string{"resource1", "resource2"},
		ParentScope: "3",
		Type:        "project",
	}
	s.Require().NoError(s.m.AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:          "2",
		Resources:   []string{"resource3"},
		ParentScope: "3",
		Type:        "project",
	}
	s.Require().NoError(s.m.AddScope(scope2))
	scope4 := gimlet.Scope{
		ID:          "4",
		Resources:   []string{"resource4"},
		ParentScope: "root",
		Type:        "project",
	}
	s.Require().NoError(s.m.AddScope(scope4))
	wrongType := gimlet.Scope{
		ID:   "wrongType",
		Type: "foo",
	}
	s.Require().NoError(s.m.AddScope(wrongType))
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
	wrongType := gimlet.Role{
		ID:    "wrong",
		Scope: "wrongType",
	}
	s.NoError(s.m.UpdateRole(wrongType))
	allRoles := []gimlet.Role{role1, role2, role3, role4, roleRoot, wrongType}

	filtered, err := s.m.FilterForResource(allRoles, "resource1", "project")
	s.NoError(err)
	s.Equal([]gimlet.Role{role1, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource2", "project")
	s.NoError(err)
	s.Equal([]gimlet.Role{role1, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource3", "project")
	s.NoError(err)
	s.Equal([]gimlet.Role{role2, role3, roleRoot}, filtered)
	filtered, err = s.m.FilterForResource(allRoles, "resource4", "project")
	s.NoError(err)
	s.Equal([]gimlet.Role{role4, roleRoot}, filtered)
}

func (s *RoleManagerSuite) TestFilterScopesByResourceType() {
	correctType := "correctType"
	wrongType := "wrongType"
	typeMap := map[string]map[string]bool{}
	typeMap[correctType] = map[string]bool{}
	typeMap[wrongType] = map[string]bool{}

	scope1 := gimlet.Scope{
		ID:   "s1",
		Type: correctType,
	}
	typeMap[correctType]["s1"] = true
	s.Require().NoError(s.m.AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:   "s2",
		Type: correctType,
	}
	typeMap[correctType]["s2"] = true
	s.Require().NoError(s.m.AddScope(scope2))
	scope3 := gimlet.Scope{
		ID:   "s3",
		Type: wrongType,
	}
	typeMap[wrongType]["s3"] = true
	s.Require().NoError(s.m.AddScope(scope3))
	scope4 := gimlet.Scope{
		ID:   "s4",
		Type: correctType,
	}
	typeMap[correctType]["s4"] = true
	s.Require().NoError(s.m.AddScope(scope4))
	scope5 := gimlet.Scope{
		ID:   "s5",
		Type: wrongType,
	}
	typeMap[wrongType]["s5"] = true
	s.Require().NoError(s.m.AddScope(scope5))
	allIds := []string{"s1", "s2", "s3", "s4", "s5"}

	filtered, err := s.m.FilterScopesByResourceType(allIds, correctType)
	s.Require().NoError(err)
	s.Require().Len(filtered, len(typeMap[correctType]))
	for _, scope := range filtered {
		s.True(typeMap[correctType][scope.ID])
	}

	filtered, err = s.m.FilterScopesByResourceType(allIds, wrongType)
	s.Require().NoError(err)
	s.Require().Len(filtered, len(typeMap[wrongType]))
	for _, scope := range filtered {
		s.True(typeMap[wrongType][scope.ID])
	}
}

func (s *RoleManagerSuite) TestRequiresPermissionMiddleware() {
	//setup
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	role1 := gimlet.Role{
		ID:          "r1",
		Scope:       "1",
		Permissions: map[string]int{"edit": 1},
	}
	s.NoError(s.m.UpdateRole(role1))
	resourceLevels := []string{"resource_id"}
	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:             s.m,
		PermissionKey:  "edit",
		ResourceType:   "project",
		RequiredLevel:  1,
		ResourceLevels: resourceLevels,
	}
	permissionMiddleware := gimlet.RequiresPermission(opts)
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	userOpts, err := gimlet.NewBasicUserOptions("user")
	s.Require().NoError(err)
	user := gimlet.NewBasicUser(userOpts.Name("name").Email("email").Password("password").Key("key").AccessToken("access_token").RefreshToken("refresh_token").RoleManager(s.m))
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{*user}, s.m)
	s.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest("GET", "http://foo.com/bar", nil)
	req = mux.SetURLVars(req, map[string]string{"resource_id": "resource1"})

	// no user attached should 401
	rw := httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	s.Equal(http.StatusUnauthorized, rw.Code)
	s.Equal(0, counter)

	// attach a user, but with no permissions yet
	ctx := gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	s.Equal(http.StatusUnauthorized, rw.Code)
	s.Equal(0, counter)

	// give user the right permissions
	userOpts, err = gimlet.NewBasicUserOptions("user")
	s.Require().NoError(err)
	user = gimlet.NewBasicUser(userOpts.Name("name").Email("email").Password("password").Key("key").AccessToken("access_token").RefreshToken("refresh_token").Roles(role1.ID).RoleManager(s.m))
	_, err = um.GetOrCreateUser(user)
	s.NoError(err)
	ctx = gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	s.Equal(http.StatusOK, rw.Code)
	s.Equal(1, counter)

	// request for a resource the user doesn't have access to
	rw = httptest.NewRecorder()
	req = mux.SetURLVars(req, map[string]string{"resource_id": "resource3"})
	authHandler.ServeHTTP(rw, req, checkPermission)
	s.Equal(http.StatusUnauthorized, rw.Code)
	s.Equal(1, counter)

	// no resource found = not allowed
	rw = httptest.NewRecorder()
	req = mux.SetURLVars(req, map[string]string{})
	authHandler.ServeHTTP(rw, req, checkPermission)
	s.Equal(http.StatusNotFound, rw.Code)
	s.Equal(1, counter)
}

func (s *RoleManagerSuite) TestHighestPermissionsForRoles() {
	r1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 20,
			"read": 20,
		},
	}
	s.NoError(s.m.UpdateRole(r1))
	r2 := gimlet.Role{
		ID:    "r2",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 50,
		},
	}
	s.NoError(s.m.UpdateRole(r2))
	r3 := gimlet.Role{
		ID:    "r3",
		Scope: "2",
		Permissions: map[string]int{
			"read": 40,
		},
	}
	s.NoError(s.m.UpdateRole(r3))

	opts := gimlet.PermissionOpts{
		Resource:     "resource1",
		ResourceType: "project",
	}
	permissions, err := HighestPermissionsForRoles([]string{"r1", "r2", "r3"}, s.m, opts)
	s.NoError(err)
	s.Len(permissions, 2)
	s.EqualValues(map[string]int{"edit": 50, "read": 20}, permissions)
}

func (s *RoleManagerSuite) TestFindRoleWithPermissions() {
	r1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 20,
			"read": 20,
		},
	}
	s.NoError(s.m.UpdateRole(r1))
	r2 := gimlet.Role{
		ID:    "r2",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 50,
		},
	}
	s.NoError(s.m.UpdateRole(r2))
	r3 := gimlet.Role{
		ID:    "r3",
		Scope: "2",
		Permissions: map[string]int{
			"edit": 20,
			"read": 20,
		},
	}
	s.NoError(s.m.UpdateRole(r3))

	// test that we can find the role with the correct criteria
	r, err := s.m.FindRoleWithPermissions("project", []string{"resource1", "resource2"}, gimlet.Permissions{"read": 20, "edit": 20})
	s.NoError(err)
	s.Equal("r1", r.ID)
	// make sure that order does not matter
	r, err = s.m.FindRoleWithPermissions("project", []string{"resource2", "resource1"}, gimlet.Permissions{"edit": 20, "read": 20})
	s.NoError(err)
	s.Equal("r1", r.ID)
	// making such a role should not return anything
	r, err = MakeRoleWithPermissions(s.m, "project", []string{"resource2", "resource1"}, gimlet.Permissions{"edit": 20, "read": 20})
	s.NoError(err)
	s.Equal("r1", r.ID)
	allRoles, err := s.m.GetAllRoles()
	s.NoError(err)
	s.Len(allRoles, 3)
	// non-matching permissions should find nothing
	r, err = s.m.FindRoleWithPermissions("project", []string{"resource2", "resource1"}, gimlet.Permissions{"edit": 10, "read": 20})
	s.NoError(err)
	s.Nil(r)
	// making such a role should create a new role with the existing scope
	r, err = MakeRoleWithPermissions(s.m, "project", []string{"resource2", "resource1"}, gimlet.Permissions{"edit": 10, "read": 20})
	s.NoError(err)
	s.Len(r.ID, 24)
	s.Equal("1", r.Scope)
	allRoles, err = s.m.GetAllRoles()
	s.NoError(err)
	s.Len(allRoles, 4)
	// wrong resources should find nothing
	r, err = s.m.FindRoleWithPermissions("project", []string{"resource2"}, gimlet.Permissions{"edit": 20, "read": 20})
	s.NoError(err)
	s.Nil(r)
	r, err = s.m.FindRoleWithPermissions("project", []string{"resource2", "resource3"}, gimlet.Permissions{"edit": 20, "read": 20})
	s.NoError(err)
	s.Nil(r)
	r, err = s.m.FindRoleWithPermissions("project", []string{"resource2", "resource3"}, gimlet.Permissions{})
	s.NoError(err)
	s.Nil(r)
	// wrong type should find nothing
	r, err = s.m.FindRoleWithPermissions("distro", []string{"resource1", "resource2"}, gimlet.Permissions{"read": 20, "edit": 20})
	s.NoError(err)
	s.Nil(r)
	// making such a role should create a new role and scope
	r, err = MakeRoleWithPermissions(s.m, "distro", []string{"resource1", "resource2"}, gimlet.Permissions{"read": 20, "edit": 20})
	s.NoError(err)
	s.Len(r.ID, 24)
	s.Len(r.Scope, 24)
	allRoles, err = s.m.GetAllRoles()
	s.NoError(err)
	s.Len(allRoles, 5)
}

func (s *RoleManagerSuite) TestHighestPermissionsForRolesAndResourceType() {
	r1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 20,
			"read": 20,
		},
	}
	s.NoError(s.m.UpdateRole(r1))
	r2 := gimlet.Role{
		ID:    "r2",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 50,
		},
	}
	s.NoError(s.m.UpdateRole(r2))
	r3 := gimlet.Role{
		ID:    "r3",
		Scope: "2",
		Permissions: map[string]int{
			"read": 40,
		},
	}
	s.NoError(s.m.UpdateRole(r3))

	expectedMap := map[string]gimlet.Permissions{}
	expectedMap["resource1"] = map[string]int{"edit": 50, "read": 20}
	expectedMap["resource2"] = map[string]int{"edit": 50, "read": 20}
	expectedMap["resource3"] = map[string]int{"read": 40}

	highestPermissions, err := HighestPermissionsForRolesAndResourceType([]string{"r1", "r2", "r3"}, "project", s.m)
	s.NoError(err)
	s.Equal(expectedMap, highestPermissions)
}

func (s *RoleManagerSuite) TestFindScopeForResources() {
	// normal scenario where scope exists
	scope, err := s.m.FindScopeForResources("project", "resource1", "resource2")
	s.NoError(err)
	s.Equal(scope.ID, "1")
	s.Equal(scope.Type, "project")
	// order should not matter
	scope, err = s.m.FindScopeForResources("project", "resource2", "resource1")
	s.NoError(err)
	s.Equal(scope.ID, "1")
	// not exact match should find nothing
	scope, err = s.m.FindScopeForResources("project", "resource2", "resource3")
	s.NoError(err)
	s.Nil(scope)
	scope, err = s.m.FindScopeForResources("project", "foo")
	s.NoError(err)
	s.Nil(scope)
	// wrong type
	scope, err = s.m.FindScopeForResources("distro", "resource1", "resource2")
	s.NoError(err)
	s.Nil(scope)
}

func (s *RoleManagerSuite) TestAddAndRemoveResources() {
	s.NoError(s.m.AddResourceToScope("1", "somethingelse"))
	foundScope, err := s.m.FindScopeForResources("project", "resource1", "resource2", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "1")
	foundScope, err = s.m.FindScopeForResources("project", "resource1", "resource2", "resource3", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "3")
	foundScope, err = s.m.FindScopeForResources("project", "resource1", "resource2", "resource3", "resource4", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "root")

	s.NoError(s.m.RemoveResourceFromScope("1", "resource1"))
	foundScope, err = s.m.FindScopeForResources("project", "resource2", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "1")
	foundScope, err = s.m.FindScopeForResources("project", "resource2", "resource3", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "3")
	foundScope, err = s.m.FindScopeForResources("project", "resource2", "resource3", "resource4", "somethingelse")
	s.NoError(err)
	s.Equal(foundScope.ID, "root")
}

func (s *RoleManagerSuite) TestFindRolesWithResources() {
	r1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
	}
	s.NoError(s.m.UpdateRole(r1))
	r2 := gimlet.Role{
		ID:    "r2",
		Scope: "2",
	}
	s.NoError(s.m.UpdateRole(r2))
	r3 := gimlet.Role{
		ID:    "r3",
		Scope: "1",
	}
	s.NoError(s.m.UpdateRole(r3))

	roles, err := s.m.FindRolesWithResources("project", []string{"resource1", "resource2"})
	s.NoError(err)
	s.Len(roles, 2)

	roles, err = s.m.FindRolesWithResources("project", []string{"resource1"})
	s.NoError(err)
	s.Len(roles, 0)

	roles, err = s.m.FindRolesWithResources("project", []string{"resource4"})
	s.NoError(err)
	s.Len(roles, 0)
}

func (s *RoleManagerSuite) TestValidPermissions() {
	permissions := gimlet.Permissions{
		"edit":          10,
		"notvalid":      20,
		"stillnotvalid": 30,
	}
	err := s.m.IsValidPermissions(permissions)
	s.Error(err)
	s.Contains(err.Error(), "'notvalid' is not a valid permission")
	s.Contains(err.Error(), "'stillnotvalid' is not a valid permission")
	s.NotContains(err.Error(), "edit")
}

func (s *RoleManagerSuite) TestPermissionSummaryForRoles() {
	r1 := gimlet.Role{
		ID:    "r1",
		Scope: "1",
		Permissions: gimlet.Permissions{
			"edit": 20,
		},
	}
	s.NoError(s.m.UpdateRole(r1))
	r2 := gimlet.Role{
		ID:    "r2",
		Scope: "2",
		Permissions: gimlet.Permissions{
			"read": 50,
		},
	}
	s.NoError(s.m.UpdateRole(r2))
	r3 := gimlet.Role{
		ID:    "r3",
		Scope: "3",
		Permissions: gimlet.Permissions{
			"edit": 40,
			"read": 30,
		},
	}
	s.NoError(s.m.UpdateRole(r3))

	summary, err := PermissionSummaryForRoles(context.Background(), []string{"r1", "r2", "r3"}, s.m)
	s.NoError(err)
	s.Len(summary[0].Permissions, 3)
	s.Equal("project", summary[0].Type)
	s.Equal(40, summary[0].Permissions["resource1"]["edit"])
	s.Equal(30, summary[0].Permissions["resource1"]["read"])
	s.Equal(40, summary[0].Permissions["resource2"]["edit"])
	s.Equal(30, summary[0].Permissions["resource2"]["read"])
	s.Equal(40, summary[0].Permissions["resource3"]["edit"])
	s.Equal(50, summary[0].Permissions["resource3"]["read"])
}

func (s *RoleManagerSuite) TestHasPermission() {
	opts := gimlet.PermissionOpts{
		Resource:      "resource1",
		ResourceType:  "project",
		Permission:    "edit",
		RequiredLevel: 10,
	}
	hasPermission := gimlet.Role{
		ID:    "1",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 10,
		},
	}
	s.NoError(s.m.UpdateRole(hasPermission))
	noPermission := gimlet.Role{
		ID:    "1",
		Scope: "1",
		Permissions: map[string]int{
			"edit": 0,
		},
	}
	s.NoError(s.m.UpdateRole(noPermission))
	s.False(gimlet.HasPermission(s.m, opts, []gimlet.Role{noPermission}))
	s.True(gimlet.HasPermission(s.m, opts, []gimlet.Role{hasPermission}))
}

func (s *RoleManagerSuite) TestFindAllowedResources() {
	roles := []gimlet.Role{
		{ID: "1", Scope: "1", Permissions: gimlet.Permissions{"read": 10}},
		{ID: "all", Scope: "root", Permissions: gimlet.Permissions{"read": 10}},
		{ID: "no_permissions", Scope: "root", Permissions: gimlet.Permissions{}},
	}
	for _, role := range roles {
		s.NoError(s.m.UpdateRole(role))
	}
	ctx := context.Background()

	resources, err := FindAllowedResources(ctx, s.m, []string{"1"}, "project", "read", 10)
	s.NoError(err)
	s.Len(resources, 2)
	s.Contains(resources, "resource1")
	s.Contains(resources, "resource2")

	resources, err = FindAllowedResources(ctx, s.m, []string{"1", "all"}, "project", "read", 10)
	s.NoError(err)
	s.Len(resources, 4)
	s.Contains(resources, "resource1")
	s.Contains(resources, "resource2")
	s.Contains(resources, "resource3")
	s.Contains(resources, "resource4")

	resources, err = FindAllowedResources(ctx, s.m, []string{"no_permissions"}, "project", "read", 10)
	s.NoError(err)
	s.Len(resources, 0)

	resources, err = FindAllowedResources(ctx, s.m, []string{}, "project", "read", 10)
	s.NoError(err)
	s.Len(resources, 0)
}
