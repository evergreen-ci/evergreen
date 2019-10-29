package evergreen

import (
	"time"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LDAPRoleMap contains a mapping of LDAP groups for a user to roles. LDAP
// groups are represented by their name and roles by their unique ID in the
// roles collection.
type LDAPRoleMap struct {
	LastUpdated time.Time         `bson:"last_updated" json:"last_updated" yaml:"last_updated"`
	Map         map[string]string `bson:"map" json:"map" yaml:"map"`
}

var (
	ldapRoleMapLastUpdatedKey = bsonutil.MustHaveTag(LDAPRoleMap{}, "LastUpdated")
	ldapRoleMapMapKey         = bsonutil.MustHaveTag(LDAPRoleMap{}, "Map")
)

// SectionId returns the ID for this section of the admin settings.
func (m *LDAPRoleMap) SectionId() string { return "ldap_role_map" }

// Get queries the database for the LDAP-role map.
func (m *LDAPRoleMap) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(m.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", m.SectionId())
	}

	if err := res.Decode(m); err != nil {
		if err == mongo.ErrNoDocuments {
			*m = LDAPRoleMap{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", m.SectionId())
	}

	return nil
}

// Add adds a new (or updates an existing) LDAP group to role mapping in the
// database.
func (m *LDAPRoleMap) Add(group, roleID string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(m.SectionId()), bson.M{
		"$set": bson.M{
			ldapRoleMapLastUpdatedKey:                           time.Now(),
			bsonutil.GetDottedKeyName(ldapRoleMapMapKey, group): roleID,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error adding %s:%s to the LDAP-role map", group, roleID)
}

// Remove removes a LDAP group to role mapping from the database.
func (m *LDAPRoleMap) Remove(group string) error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(m.SectionId()), bson.M{
		"$set": bson.M{
			ldapRoleMapLastUpdatedKey: time.Now(),
		},
		"$unset": bson.M{
			bsonutil.GetDottedKeyName(ldapRoleMapMapKey, group): "",
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error removing %s from the LDAP-role map", group)
}

// GetMap returns the actual mapping of LDAP groups to roles. If none exists,
// an empty map is returned.
func (m *LDAPRoleMap) GetMap() map[string]string {
	if m.Map == nil {
		return map[string]string{}
	}

	return m.Map
}
