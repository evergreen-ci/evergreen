package manifest

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	// BSON fields for artifact file structs
	IdKey               = bsonutil.MustHaveTag(Manifest{}, "Id")
	ManifestRevisionKey = bsonutil.MustHaveTag(Manifest{}, "Revision")
	ProjectNameKey      = bsonutil.MustHaveTag(Manifest{}, "ProjectName")
	ModulesKey          = bsonutil.MustHaveTag(Manifest{}, "Modules")
	ManifestBranchKey   = bsonutil.MustHaveTag(Manifest{}, "Branch")
	ModuleBranchKey     = bsonutil.MustHaveTag(Module{}, "Branch")
	ModuleRevisionKey   = bsonutil.MustHaveTag(Module{}, "Revision")
	OwnerKey            = bsonutil.MustHaveTag(Module{}, "Owner")
	UrlKey              = bsonutil.MustHaveTag(Module{}, "URL")
)

// FindOne gets one Manifest for the given query.
func FindOne(query db.Q) (*Manifest, error) {
	m := &Manifest{}
	err := db.FindOneQ(Collection, query, m)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return m, err
}

// TryInsert writes the manifest to the database if possible.
// If the document already exists, it returns true and the error
// If it does not it will return false and the error
func (m *Manifest) TryInsert() (bool, error) {
	err := db.Insert(Collection, m)
	if mgo.IsDup(err) {
		return true, nil
	}
	return false, err
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) db.Q {
	return db.Query(bson.D{{IdKey, id}})
}
