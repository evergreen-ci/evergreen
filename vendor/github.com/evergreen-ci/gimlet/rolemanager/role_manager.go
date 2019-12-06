package rolemanager

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/mongodb/grip"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type mongoBackedRoleManager struct {
	client    *mongo.Client
	db        string
	roleColl  string
	scopeColl string

	base
}

type MongoBackedRoleManagerOpts struct {
	Client          *mongo.Client
	DBName          string
	RoleCollection  string
	ScopeCollection string
}

func NewMongoBackedRoleManager(opts MongoBackedRoleManagerOpts) gimlet.RoleManager {
	return &mongoBackedRoleManager{
		client:    opts.Client,
		db:        opts.DBName,
		roleColl:  opts.RoleCollection,
		scopeColl: opts.ScopeCollection,
	}
}

func (m *mongoBackedRoleManager) GetAllRoles() ([]gimlet.Role, error) {
	out := []gimlet.Role{}
	ctx := context.Background()
	cursor, err := m.client.Database(m.db).Collection(m.roleColl).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (m *mongoBackedRoleManager) GetRoles(ids []string) ([]gimlet.Role, error) {
	out := []gimlet.Role{}
	ctx := context.Background()
	cursor, err := m.client.Database(m.db).Collection(m.roleColl).Find(ctx, bson.M{
		"_id": bson.M{
			"$in": ids,
		},
	})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (m *mongoBackedRoleManager) UpdateRole(role gimlet.Role) error {
	for permission := range role.Permissions {
		if !m.isValidPermission(permission) {
			return fmt.Errorf("'%s' is not a valid permission for role '%s'", permission, role.ID)
		}
	}
	ctx := context.Background()
	coll := m.client.Database(m.db).Collection(m.roleColl)
	upsert := true
	result := coll.FindOneAndReplace(ctx, bson.M{"_id": role.ID}, role, &options.FindOneAndReplaceOptions{Upsert: &upsert})
	if result == nil {
		return errors.New("did not receive a response from MongoDB")
	}
	if result.Err() == mongo.ErrNoDocuments {
		return nil
	}
	return result.Err()
}

func (m *mongoBackedRoleManager) DeleteRole(id string) error {
	ctx := context.Background()
	coll := m.client.Database(m.db).Collection(m.roleColl)
	_, err := coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (m *mongoBackedRoleManager) FilterForResource(roles []gimlet.Role, resource, resourceType string) ([]gimlet.Role, error) {
	coll := m.client.Database(m.db).Collection(m.scopeColl)
	ctx := context.Background()
	applicableScopes := []gimlet.Scope{}

	cursor, err := coll.Find(ctx, bson.M{
		"resources": resource,
	})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &applicableScopes)
	if err != nil {
		return nil, err
	}

	scopes := map[string]bool{}
	for _, scope := range applicableScopes {
		scopes[scope.ID] = true
	}

	filtered := []gimlet.Role{}
	for _, role := range roles {
		if scopes[role.Scope] {
			filtered = append(filtered, role)
		}
	}

	return filtered, nil
}

func (m *mongoBackedRoleManager) FilterScopesByResourceType(scopeIDs []string, resourceType string) ([]gimlet.Scope, error) {
	coll := m.client.Database(m.db).Collection(m.scopeColl)
	ctx := context.Background()
	query := bson.M{
		"_id":  bson.M{"$in": scopeIDs},
		"type": resourceType,
	}
	cursor, err := coll.Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "problem filtering scopeIDs by resource type in db")
	}
	scopes := []gimlet.Scope{}
	if err = cursor.All(ctx, &scopes); err != nil {
		return nil, errors.Wrap(err, "problem marshalling scope data")
	}

	return scopes, nil
}

func (m *mongoBackedRoleManager) FindScopeForResources(resourceType string, resources ...string) (*gimlet.Scope, error) {
	coll := m.client.Database(m.db).Collection(m.scopeColl)
	ctx := context.Background()
	query := bson.M{
		"type": resourceType,
		"$and": []bson.M{
			{"resources": bson.M{
				"$all": resources,
			}},
			{"resources": bson.M{
				"$size": len(resources),
			}},
		},
	}
	result := coll.FindOne(ctx, query)
	err := result.Err()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	scope := &gimlet.Scope{}
	if err := result.Decode(scope); err != nil {
		return nil, err
	}

	return scope, nil
}

func (m *mongoBackedRoleManager) AddScope(scope gimlet.Scope) error {
	_, err := m.client.Database(m.db).Collection(m.scopeColl).InsertOne(context.Background(), scope)
	return err
}

func (m *mongoBackedRoleManager) DeleteScope(id string) error {
	_, err := m.client.Database(m.db).Collection(m.scopeColl).DeleteOne(context.Background(), bson.M{"_id": id})
	return err
}

func (m *mongoBackedRoleManager) FindRoleWithPermissions(resourceType string, resources []string, permissions gimlet.Permissions) (*gimlet.Role, error) {
	ctx := context.Background()
	var permissionMatch bson.M
	if len(permissions) > 0 {
		andClause := []bson.M{}
		for key, level := range permissions {
			andClause = append(andClause, bson.M{fmt.Sprintf("permissions.%s", key): level})
		}
		permissionMatch = bson.M{
			"$and": andClause,
		}
	} else {
		permissionMatch = bson.M{
			"permissions": nil,
		}
	}
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					{"resources": bson.M{
						"$all": resources,
					}},
					{"resources": bson.M{
						"$size": len(resources),
					}},
					{
						"type": resourceType,
					},
				},
			},
		},
		{
			"$lookup": bson.M{
				"from":         m.roleColl,
				"localField":   "_id",
				"foreignField": "scope",
				"as":           "temp_roles",
			},
		},
		{
			"$replaceRoot": bson.M{
				"newRoot": bson.M{
					"$arrayElemAt": []interface{}{"$temp_roles", 0},
				},
			},
		},
		{
			"$match": permissionMatch,
		},
	}
	cursor, err := m.client.Database(m.db).Collection(m.scopeColl).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var roles []gimlet.Role
	err = cursor.All(ctx, &roles)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, nil
	}

	return &roles[0], nil
}

func (m *mongoBackedRoleManager) Clear() error {
	ctx := context.Background()
	catcher := grip.NewBasicCatcher()
	catcher.Add(m.client.Database(m.db).Collection(m.scopeColl).Drop(ctx))
	catcher.Add(m.client.Database(m.db).Collection(m.roleColl).Drop(ctx))
	return catcher.Resolve()
}

type inMemoryRoleManager struct {
	roles  map[string]gimlet.Role
	scopes map[string]gimlet.Scope

	base
}

func NewInMemoryRoleManager() gimlet.RoleManager {
	return &inMemoryRoleManager{
		roles:  map[string]gimlet.Role{},
		scopes: map[string]gimlet.Scope{},
	}
}

func (m *inMemoryRoleManager) GetAllRoles() ([]gimlet.Role, error) {
	out := []gimlet.Role{}
	for _, role := range m.roles {
		out = append(out, role)
	}
	return out, nil
}

func (m *inMemoryRoleManager) GetRoles(ids []string) ([]gimlet.Role, error) {
	foundRoles := []gimlet.Role{}
	for _, id := range ids {
		role, found := m.roles[id]
		if found {
			foundRoles = append(foundRoles, role)
		}
	}
	return foundRoles, nil
}

func (m *inMemoryRoleManager) UpdateRole(role gimlet.Role) error {
	for permission := range role.Permissions {
		if !m.isValidPermission(permission) {
			return fmt.Errorf("'%s' is not a valid permission for role '%s'", permission, role.ID)
		}
	}
	m.roles[role.ID] = role
	return nil
}

func (m *inMemoryRoleManager) DeleteRole(id string) error {
	delete(m.roles, id)
	return nil
}

func (m *inMemoryRoleManager) FilterForResource(roles []gimlet.Role, resource, resourceType string) ([]gimlet.Role, error) {
	scopes := map[string]bool{}
	for _, scope := range m.scopes {
		if scope.Type != resourceType {
			continue
		}
		if stringSliceContains(scope.Resources, resource) {
			toAdd := m.findScopesRecursive(scope)
			for _, scopeID := range toAdd {
				scopes[scopeID] = true
			}
		}
	}

	filtered := []gimlet.Role{}
	for _, role := range roles {
		if scopes[role.Scope] {
			filtered = append(filtered, role)
		}
	}

	return filtered, nil
}

func (m *inMemoryRoleManager) FilterScopesByResourceType(scopeIDs []string, resourceType string) ([]gimlet.Scope, error) {
	scopeIdMap := map[string]bool{}
	for _, id := range scopeIDs {
		scopeIdMap[id] = true
	}

	scopes := []gimlet.Scope{}
	for _, scope := range m.scopes {
		if scopeIdMap[scope.ID] && scope.Type == resourceType {
			scopes = append(scopes, scope)
		}
	}

	return scopes, nil
}

func (m *inMemoryRoleManager) FindScopeForResources(resourceType string, resources ...string) (*gimlet.Scope, error) {
	for _, scope := range m.scopes {
		if scope.Type == resourceType && slicesContainSameElements(resources, scope.Resources) {
			return &scope, nil
		}
	}
	return nil, nil
}

func (m *inMemoryRoleManager) AddScope(scope gimlet.Scope) error {
	m.scopes[scope.ID] = scope
	return nil
}

func (m *inMemoryRoleManager) DeleteScope(id string) error {
	delete(m.scopes, id)
	return nil
}

func (m *inMemoryRoleManager) Clear() error {
	m.roles = map[string]gimlet.Role{}
	m.scopes = map[string]gimlet.Scope{}
	return nil
}

func (m *inMemoryRoleManager) findScopesRecursive(currScope gimlet.Scope) []string {
	scopes := []string{currScope.ID}
	if currScope.ParentScope == "" {
		return scopes
	}
	return append(scopes, m.findScopesRecursive(m.scopes[currScope.ParentScope])...)
}

func (m *inMemoryRoleManager) FindRoleWithPermissions(resourceType string, resources []string, permissions gimlet.Permissions) (*gimlet.Role, error) {
	validScopes := []string{}
	for _, scope := range m.scopes {
		if slicesContainSameElements(resources, scope.Resources) && scope.Type == resourceType {
			validScopes = append(validScopes, scope.ID)
		}
	}
	for _, role := range m.roles {
		if stringSliceContains(validScopes, role.Scope) {
			if reflect.DeepEqual(role.Permissions, permissions) {
				return &role, nil
			}
		}
	}
	return nil, nil
}

func stringSliceContains(slice []string, toFind string) bool {
	for _, str := range slice {
		if str == toFind {
			return true
		}
	}
	return false
}

func slicesContainSameElements(slice1 []string, slice2 []string) bool {
	elements1 := map[string]int{}
	elements2 := map[string]int{}
	for _, elem := range slice1 {
		elements1[elem]++
	}
	for _, elem := range slice2 {
		elements2[elem]++
	}
	return reflect.DeepEqual(elements1, elements2)
}

type base struct {
	permissionsMux        sync.RWMutex
	registeredPermissions map[string]interface{}
}

func (b *base) RegisterPermissions(permissions []string) error {
	b.permissionsMux.Lock()
	defer b.permissionsMux.Unlock()
	if b.registeredPermissions == nil {
		b.registeredPermissions = map[string]interface{}{}
	}
	for _, permission := range permissions {
		_, exists := b.registeredPermissions[permission]
		if exists {
			return fmt.Errorf("permission '%s' has already been registered", permission)
		}
		b.registeredPermissions[permission] = nil
	}
	return nil
}

func (b *base) isValidPermission(permission string) bool {
	b.permissionsMux.RLock()
	defer b.permissionsMux.RUnlock()
	_, valid := b.registeredPermissions[permission]
	return valid
}

// HighestPermissionsForRoles takes in a list of roles and returns an aggregated list of the highest
// levels for all permissions
func HighestPermissionsForRoles(rolesIDs []string, rm gimlet.RoleManager, opts gimlet.PermissionOpts) (gimlet.Permissions, error) {
	roles, err := rm.GetRoles(rolesIDs)
	if err != nil {
		return nil, err
	}
	roles, err = rm.FilterForResource(roles, opts.Resource, opts.ResourceType)
	if err != nil {
		return nil, err
	}
	highestPermissions := map[string]int{}
	for _, role := range roles {
		for permission, level := range role.Permissions {
			highestLevel, exists := highestPermissions[permission]
			if !exists || level > highestLevel {
				highestPermissions[permission] = level
			}
		}
	}
	return highestPermissions, nil
}

// HighestPermissionsForResourceType takes a list of role IDs, a resource type,
// and a role manager and returns a mapping of all resource IDs for the given
// roles to their highest permissions based on those roles.
func HighestPermissionsForRolesAndResourceType(roleIDs []string, resourceType string, rm gimlet.RoleManager) (map[string]gimlet.Permissions, error) {
	roles, err := rm.GetRoles(roleIDs)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting roles")
	}
	scopeIDs := make([]string, len(roles))
	for i, role := range roles {
		scopeIDs[i] = role.Scope
	}

	scopes, err := rm.FilterScopesByResourceType(scopeIDs, resourceType)
	if err != nil {
		return nil, errors.Wrap(err, "problem filtering scopes by resource types")
	}
	scopeMap := map[string][]string{}
	for _, scope := range scopes {
		scopeMap[scope.ID] = scope.Resources
	}

	highestPermissions := map[string]gimlet.Permissions{}
	for _, role := range roles {
		for _, resource := range scopeMap[role.Scope] {
			if _, ok := highestPermissions[resource]; ok {
				for permission, level := range role.Permissions {
					highestLevel, exists := highestPermissions[resource][permission]
					if !exists || level > highestLevel {
						highestPermissions[resource][permission] = level
					}
				}
			} else {
				highestPermissions[resource] = role.Permissions
			}
		}
	}

	return highestPermissions, nil
}

func MakeRoleWithPermissions(rm gimlet.RoleManager, resourceType string, resources []string, permissions gimlet.Permissions) (*gimlet.Role, error) {
	existing, err := rm.FindRoleWithPermissions(resourceType, resources, permissions)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	scope, err := rm.FindScopeForResources(resourceType, resources...)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		scope = &gimlet.Scope{
			ID:        primitive.NewObjectID().Hex(),
			Type:      resourceType,
			Resources: resources,
		}
		err = rm.AddScope(*scope)
		if err != nil {
			return nil, err
		}
	}
	newRole := gimlet.Role{
		ID:          primitive.NewObjectID().Hex(),
		Scope:       scope.ID,
		Permissions: permissions,
	}
	err = rm.UpdateRole(newRole)
	if err != nil {
		return nil, err
	}

	return &newRole, nil
}
