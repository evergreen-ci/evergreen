package rolemanager

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/evergreen-ci/gimlet"
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
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"resources": resource,
				"type":      resourceType,
			},
		},
		{
			"$graphLookup": bson.M{
				"from":             m.scopeColl,
				"startWith":        "$parent",
				"connectFromField": "parent",
				"connectToField":   "_id",
				"as":               "parents_temp",
			},
		},
		{
			"$addFields": bson.M{
				"parents_temp": bson.M{
					"$concatArrays": []interface{}{"$parents_temp", []string{"$$ROOT"}},
				},
			},
		},
		{
			"$project": bson.M{
				"_id":     0,
				"results": "$parents_temp",
			},
		},
		{
			"$unwind": "$results",
		},
		{
			"$replaceRoot": bson.M{
				"newRoot": "$results",
			},
		},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	applicableScopes := []gimlet.Scope{}
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

func (m *mongoBackedRoleManager) AddScope(scope gimlet.Scope) error {
	_, err := m.client.Database(m.db).Collection(m.scopeColl).InsertOne(context.Background(), scope)
	return err
}

func (m *mongoBackedRoleManager) DeleteScope(id string) error {
	_, err := m.client.Database(m.db).Collection(m.scopeColl).DeleteOne(context.Background(), bson.M{"_id": id})
	return err
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

func (m *inMemoryRoleManager) AddScope(scope gimlet.Scope) error {
	m.scopes[scope.ID] = scope
	return nil
}

func (m *inMemoryRoleManager) DeleteScope(id string) error {
	delete(m.scopes, id)
	return nil
}

func (m *inMemoryRoleManager) findScopesRecursive(currScope gimlet.Scope) []string {
	scopes := []string{currScope.ID}
	if currScope.ParentScope == "" {
		return scopes
	}
	return append(scopes, m.findScopesRecursive(m.scopes[currScope.ParentScope])...)
}

func stringSliceContains(slice []string, toFind string) bool {
	for _, str := range slice {
		if str == toFind {
			return true
		}
	}
	return false
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
