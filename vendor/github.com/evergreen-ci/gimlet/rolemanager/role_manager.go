package rolemanager

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver"
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
	if len(ids) == 0 {
		return out, nil
	}
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
	if len(scopeIDs) == 0 {
		return []gimlet.Scope{}, nil
	}
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
	toUpdate := []string{}
	var err error
	if scope.ParentScope != "" {
		toUpdate, err = m.findParentsOfScope(scope.ParentScope)
		if err != nil {
			return err
		}
	}

	updateFunc := func(ctx mongo.SessionContext) error {
		err := ctx.StartTransaction()
		if err != nil {
			return err
		}
		scopeCollection := m.client.Database(m.db).Collection(m.scopeColl)
		_, err = scopeCollection.InsertOne(ctx, scope)
		if err != nil {
			return err
		}
		if len(scope.Resources) > 0 && len(toUpdate) > 0 {
			filter := bson.M{
				"_id": bson.M{
					"$in": toUpdate,
				},
			}
			update := bson.M{
				"$push": bson.M{
					"resources": bson.M{
						"$each": scope.Resources,
					},
				},
			}
			_, err = scopeCollection.UpdateMany(ctx, filter, update)
			if err != nil {
				return err
			}
		}
		return ctx.CommitTransaction(ctx)
	}
	return m.retryTransaction(updateFunc)
}

// note this assumes that resources in parent scopes are not duplicated (SERVER-1014)
func (m *mongoBackedRoleManager) DeleteScope(scope gimlet.Scope) error {
	var toUpdate []string
	var err error
	toUpdate, err = m.findParentsOfScope(scope.ParentScope)
	if err != nil {
		return err
	}

	updateFunc := func(ctx mongo.SessionContext) error {
		err := ctx.StartTransaction()
		if err != nil {
			return err
		}
		scopeCollection := m.client.Database(m.db).Collection(m.scopeColl)
		_, err = scopeCollection.DeleteOne(ctx, bson.M{"_id": scope.ID})
		if err != nil {
			return err
		}
		if len(toUpdate) > 0 {
			filter := bson.M{
				"_id": bson.M{
					"$in": toUpdate,
				},
			}
			update := bson.M{
				"$pull": bson.M{
					"resources": bson.M{
						"$each": scope.Resources,
					},
				},
			}
			_, err = scopeCollection.UpdateMany(ctx, filter, update)
			if err != nil {
				return err
			}
		}
		return ctx.CommitTransaction(ctx)
	}
	return m.retryTransaction(updateFunc)
}

func (m *mongoBackedRoleManager) GetScope(ctx context.Context, id string) (*gimlet.Scope, error) {
	var err error
	scopeCollection := m.client.Database(m.db).Collection(m.scopeColl)
	result := scopeCollection.FindOne(ctx, bson.M{"_id": id})
	if err = result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	scope := &gimlet.Scope{}
	err = result.Decode(scope)
	if err != nil {
		return nil, err
	}
	return scope, nil
}

func (m *mongoBackedRoleManager) AddResourceToScope(scope, resource string) error {
	toUpdate, err := m.findParentsOfScope(scope)
	if err != nil {
		return err
	}
	filter := bson.M{
		"_id": bson.M{
			"$in": toUpdate,
		},
	}
	update := bson.M{
		"$addToSet": bson.M{
			"resources": resource,
		},
	}
	_, err = m.client.Database(m.db).Collection(m.scopeColl).UpdateMany(context.Background(), filter, update)
	return err
}

func (m *mongoBackedRoleManager) RemoveResourceFromScope(scope, resource string) error {
	toUpdate, err := m.findParentsOfScope(scope)
	if err != nil {
		return err
	}
	filter := bson.M{
		"_id": bson.M{
			"$in": toUpdate,
		},
	}
	update := bson.M{
		"$pull": bson.M{
			"resources": resource,
		},
	}
	_, err = m.client.Database(m.db).Collection(m.scopeColl).UpdateMany(context.Background(), filter, update)
	return err
}

func (m *mongoBackedRoleManager) FindRolesWithResources(resourceType string, resources []string) ([]gimlet.Role, error) {
	ctx := context.Background()
	pipeline := m.resourcesPipeline(resourceType, resources)
	cursor, err := m.client.Database(m.db).Collection(m.roleColl).Aggregate(ctx, pipeline)
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

	return roles, nil
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
	pipeline := append(m.resourcesPipeline(resourceType, resources), bson.M{"$match": permissionMatch})
	cursor, err := m.client.Database(m.db).Collection(m.roleColl).Aggregate(ctx, pipeline)
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

func (m *mongoBackedRoleManager) resourcesPipeline(resourceType string, resources []string) []bson.M {
	return []bson.M{
		{
			"$lookup": bson.M{
				"from":         m.scopeColl,
				"localField":   "scope",
				"foreignField": "_id",
				"as":           "scope_document",
			},
		},
		{
			"$match": bson.M{
				"$and": []bson.M{
					{"scope_document.0.resources": bson.M{
						"$all": resources,
					}},
					{"scope_document.0.resources": bson.M{
						"$size": len(resources),
					}},
					{
						"scope_document.0.type": resourceType,
					},
				},
			},
		},
	}
}

func (m *mongoBackedRoleManager) Clear() error {
	ctx := context.Background()
	catcher := grip.NewBasicCatcher()
	catcher.Add(m.client.Database(m.db).Collection(m.scopeColl).Drop(ctx))
	catcher.Add(m.client.Database(m.db).Collection(m.roleColl).Drop(ctx))
	cmd := map[string]string{
		"create": m.scopeColl,
	}
	catcher.Add(m.client.Database(m.db).RunCommand(context.Background(), cmd).Err())
	cmd = map[string]string{
		"create": m.roleColl,
	}
	catcher.Add(m.client.Database(m.db).RunCommand(context.Background(), cmd).Err())
	return catcher.Resolve()
}

func (m *mongoBackedRoleManager) findParentsOfScope(scopeId string) ([]string, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"_id": scopeId,
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
		{
			"$project": bson.M{
				"_id": 1,
			},
		},
	}

	ctx := context.Background()
	cursor, err := m.client.Database(m.db).Collection(m.scopeColl).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	scopes := []gimlet.Scope{}
	err = cursor.All(ctx, &scopes)
	if err != nil {
		return nil, err
	}
	scopeIds := []string{}
	for _, scope := range scopes {
		scopeIds = append(scopeIds, scope.ID)
	}
	return scopeIds, nil
}

func (m *mongoBackedRoleManager) retryTransaction(f func(mongo.SessionContext) error) error {
	const retryCount = 5
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	for i := 0; i < retryCount; i++ {
		err := m.client.UseSession(ctx, f)
		if !isTransientTxErr(err) {
			return err
		}
	}
	return errors.New("hit max retries while retrying transaction")
}

func isTransientTxErr(err error) bool {
	if err == nil {
		return false
	}
	rootErr := errors.Cause(err)
	cmdErr, isCmdErr := rootErr.(mongo.CommandError)
	if isCmdErr && cmdErr.HasErrorLabel(driver.TransientTransactionError) {
		return true
	}
	return false
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
	parents := m.findScopesRecursive(scope)
	for _, parentId := range parents {
		if parentId != scope.ID {
			parent := m.scopes[parentId]
			parent.Resources = append(parent.Resources, scope.Resources...)
			m.scopes[parentId] = parent
		}
	}
	return nil
}

func (m *inMemoryRoleManager) DeleteScope(scope gimlet.Scope) error {
	delete(m.scopes, scope.ID)
	for _, resource := range scope.Resources {
		if err := m.RemoveResourceFromScope(scope.ParentScope, resource); err != nil {
			return err
		}
	}
	return nil
}

func (m *inMemoryRoleManager) GetScope(_ context.Context, id string) (*gimlet.Scope, error) {
	scope, found := m.scopes[id]
	if !found {
		return nil, nil
	}
	return &scope, nil
}

func (m *inMemoryRoleManager) AddResourceToScope(scopeId, resource string) error {
	baseScope, found := m.scopes[scopeId]
	if !found {
		return errors.New("no scope found")
	}
	toUpdate := m.findScopesRecursive(baseScope)
	for _, scopeId := range toUpdate {
		scope := m.scopes[scopeId]
		scope.Resources = append(scope.Resources, resource)
		m.scopes[scopeId] = scope
	}

	return nil
}

func (m *inMemoryRoleManager) RemoveResourceFromScope(scopeId, resource string) error {
	baseScope, found := m.scopes[scopeId]
	if !found {
		return errors.New("no scope found")
	}
	toUpdate := m.findScopesRecursive(baseScope)
	for _, scopeId := range toUpdate {
		scope := m.scopes[scopeId]
		for i := len(scope.Resources) - 1; i >= 0; i-- {
			if scope.Resources[i] == resource {
				scope.Resources = append(scope.Resources[:i], scope.Resources[i+1:]...)
			}
		}
		m.scopes[scopeId] = scope
	}

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

func (m *inMemoryRoleManager) FindRolesWithResources(resourceType string, resources []string) ([]gimlet.Role, error) {
	validScopes := []string{}
	for _, scope := range m.scopes {
		if slicesContainSameElements(resources, scope.Resources) && scope.Type == resourceType {
			validScopes = append(validScopes, scope.ID)
		}
	}
	roles := []gimlet.Role{}
	for _, role := range m.roles {
		if stringSliceContains(validScopes, role.Scope) {
			roles = append(roles, role)
		}
	}
	return roles, nil
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

func (b *base) IsValidPermissions(permissions gimlet.Permissions) error {
	catcher := grip.NewBasicCatcher()
	for permission := range permissions {
		catcher.AddWhen(!b.isValidPermission(permission), errors.Errorf("'%s' is not a valid permission", permission))
	}
	return catcher.Resolve()
}

type PermissionSummary struct {
	Type        string                  `json:"type"`
	Permissions PermissionsForResources `json:"permissions"`
}

type PermissionsForResources map[string]gimlet.Permissions

func PermissionSummaryForRoles(ctx context.Context, rolesIDs []string, rm gimlet.RoleManager) ([]PermissionSummary, error) {
	roles, err := rm.GetRoles(rolesIDs)
	if err != nil {
		return nil, err
	}
	summary := []PermissionSummary{}
	highestPermissions := map[string]PermissionsForResources{}
	for _, role := range roles {
		scope, err := rm.GetScope(ctx, role.Scope)
		if err != nil {
			return nil, err
		}
		resourceType := scope.Type
		highestPermissionsForType, exists := highestPermissions[resourceType]
		if !exists {
			highestPermissionsForType = PermissionsForResources{}
		}
		for _, resource := range scope.Resources {
			highestPermissionsForResource, exists := highestPermissions[resourceType][resource]
			if !exists {
				highestPermissionsForResource = gimlet.Permissions{}
			}
			for permission, level := range role.Permissions {
				highestLevel, exists := highestPermissionsForResource[permission]
				if !exists || level > highestLevel {
					highestPermissionsForResource[permission] = level
				}
			}
			highestPermissionsForType[resource] = highestPermissionsForResource
		}
		highestPermissions[resourceType] = highestPermissionsForType
	}
	for resourceType, permissionsForType := range highestPermissions {
		summary = append(summary, PermissionSummary{Type: resourceType, Permissions: permissionsForType})
	}
	return summary, nil
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
	if err := rm.IsValidPermissions(permissions); err != nil {
		return nil, err
	}
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

// FindAllowedResources takes a list of roles and a permission to check in those roles. It returns
// a list of all resources that the given roles have access to with the given permission check.
// It answers the question "Given this list of roles (likely from a single user), what resources
// can they access, given this permission check?"
func FindAllowedResources(ctx context.Context, rm gimlet.RoleManager, roles []string, resourceType, requiredPermission string, requiredLevel int) ([]string, error) {
	if resourceType == "" {
		return nil, errors.New("must specify a resource type")
	}
	if requiredPermission == "" {
		return nil, errors.New("must specify a required permission")
	}
	allowedResources := map[string]bool{}
	roleDocs, err := rm.GetRoles(roles)
	if err != nil {
		return nil, errors.Wrap(err, "error getting roles")
	}
	for _, role := range roleDocs {
		level := role.Permissions[requiredPermission]
		if level < requiredLevel {
			continue
		}
		scope, err := rm.GetScope(ctx, role.Scope)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get scope '%s'", role.Scope)
		}
		if scope == nil {
			return nil, errors.Errorf("scope '%s' not found", role.Scope)
		}
		if scope.Type == resourceType {
			for _, resource := range scope.Resources {
				allowedResources[resource] = true
			}
		}
	}
	deduplicatedResources := []string{}
	for resource := range allowedResources {
		deduplicatedResources = append(deduplicatedResources, resource)
	}
	return deduplicatedResources, nil
}
