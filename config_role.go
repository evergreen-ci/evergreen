package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LDAPRoleMap contains mappings of LDAP groups for a user to roles. LDAP
// groups are represented by their name and roles by their unique ID in the
// roles collection.
type LDAPRoleMap []LDAPRoleMapping

// LDAPRoleMapping contains a single mapping of a LDAP group to a role ID.
type LDAPRoleMapping struct {
	LDAPGroup string `bson:"ldap_group" json:"ldap_group" yaml:"ldap_group"`
	RoleID    string `bson:"role_id" json:"role_id" yaml:"role_id"`
}

var (
	ldapRoleMappingLDAPGroupKey = bsonutil.MustHaveTag(LDAPRoleMap{}, "LDAPGroup")
	ldapRoleMappingRoleIDKey    = bsonutil.MustHaveTag(LDAPRoleMap{}, "RoleID")
)

// Add adds a new (or updates an existing) LDAP group to role mapping in the
// database.
func (m *LDAPRoleMap) Add(group, roleID string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)
	s := &Settings{}

	res, err := coll.UpdateOne(
		ctx,
		byId(s.SectionId()),
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(ldapRoleMapKey, "$[elem]", ldapRoleMappingRoleIDKey): roleID,
			},
		},
		options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []interface{}{
				bson.M{
					bsonutil.GetDottedKeyName("elem", ldapRoleMappingLDAPGroupKey): bson.M{"$eq": group},
				},
			},
		}),
	)

	if err != nil || res.MatchedCount > 0 {
		return errors.Wrapf(err, "adding '%s:%s' to the LDAP-role map", group, roleID)
	}

	_, err = coll.UpdateOne(ctx, byId(s.SectionId()), bson.M{
		"$push": bson.M{
			ldapRoleMapKey: bson.M{
				ldapRoleMappingLDAPGroupKey: group,
				ldapRoleMappingRoleIDKey:    roleID,
			},
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "adding '%s:%s' to the LDAP-role map", group, roleID)
}

// Remove removes a LDAP group to role mapping from the database.
func (m *LDAPRoleMap) Remove(group string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)
	s := &Settings{}

	_, err := coll.UpdateOne(ctx, byId(s.SectionId()), bson.M{
		"$pull": bson.M{
			ldapRoleMapKey: bson.M{
				ldapRoleMappingLDAPGroupKey: group,
			},
		},
	})

	return errors.Wrapf(err, "removing group '%s' from the LDAP-role map", group)
}
