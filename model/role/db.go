package role

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	IdKey          = bsonutil.MustHaveTag(Role{}, "Id")
	NameKey        = bsonutil.MustHaveTag(Role{}, "Name")
	ScopeTypeKey   = bsonutil.MustHaveTag(Role{}, "ScopeType")
	ScopeKey       = bsonutil.MustHaveTag(Role{}, "Scope")
	PermissionsKey = bsonutil.MustHaveTag(Role{}, "Permissions")
)

func (r *Role) Upsert() (*adb.ChangeInfo, error) {
	update := bson.M{
		NameKey:        r.Name,
		ScopeTypeKey:   r.ScopeType,
		ScopeKey:       r.Scope,
		PermissionsKey: r.Permissions,
	}
	return db.Upsert(Collection, bson.M{IdKey: r.Id}, update)
}

func FindOne(query db.Q) (*Role, error) {
	r := &Role{}
	err := db.FindOneQ(Collection, query, r)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return r, err
}

func FindOneId(id string) (*Role, error) {
	return FindOne(db.Query(bson.M{IdKey: id}))
}
