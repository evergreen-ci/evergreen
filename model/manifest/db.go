package manifest

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// BSON fields for artifact file structs
	IdKey               = bsonutil.MustHaveTag(Manifest{}, "Id")
	ManifestRevisionKey = bsonutil.MustHaveTag(Manifest{}, "Revision")
	ProjectNameKey      = bsonutil.MustHaveTag(Manifest{}, "ProjectName")
	ModulesKey          = bsonutil.MustHaveTag(Manifest{}, "Modules")
	ManifestBranchKey   = bsonutil.MustHaveTag(Manifest{}, "Branch")
	IsBaseKey           = bsonutil.MustHaveTag(Manifest{}, "IsBase")
	ModuleBranchKey     = bsonutil.MustHaveTag(Module{}, "Branch")
	ModuleRevisionKey   = bsonutil.MustHaveTag(Module{}, "Revision")
	OwnerKey            = bsonutil.MustHaveTag(Module{}, "Owner")
	UrlKey              = bsonutil.MustHaveTag(Module{}, "URL")
)

// FindOne gets one Manifest for the given query.
func FindOne(query db.Q) (*Manifest, error) {
	m := &Manifest{}
	err := db.FindOneQ(Collection, query, m)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return m, err
}

// TryInsert writes the manifest to the database if possible.
// If the document already exists, it returns true and the error
// If it does not it will return false and the error
func (m *Manifest) TryInsert() (bool, error) {
	err := db.Insert(Collection, m)
	if db.IsDuplicateKey(err) {
		return true, nil
	}
	return false, err
}

// InsertWithContext is the same as Insert, but it respects the given context by
// avoiding the global Anser DB session.
func (m *Manifest) InsertWithContext(ctx context.Context) error {
	if _, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, m); err != nil {
		return err
	}
	return nil
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) db.Q {
	return db.Query(bson.M{IdKey: id})
}

func ByBaseProjectAndRevision(project, revision string) db.Q {
	return db.Query(bson.M{
		ProjectNameKey:      project,
		ManifestRevisionKey: revision,
		IsBaseKey:           true,
	})
}

func FindFromVersion(versionID, project, revision, requester string) (*Manifest, error) {
	manifest, err := FindOne(ById(versionID))
	if err != nil {
		return nil, errors.Wrap(err, "finding manifest")
	}
	if manifest != nil {
		return manifest, nil
	}

	// the version wasn't from the repotracker
	// find the base commit's manifest
	manifest, err = FindOne(ByBaseProjectAndRevision(project, revision))
	if err != nil {
		return nil, errors.Wrap(err, "finding manifest")
	}
	if manifest == nil {
		return nil, nil
	}

	if evergreen.IsPatchRequester(requester) {
		var p *patch.Patch
		p, err = patch.FindOneId(versionID)
		if err != nil {
			return nil, errors.Wrapf(err, "getting patch '%s'", versionID)
		}
		if p == nil {
			return nil, errors.Errorf("no corresponding patch '%s'", versionID)
		}
		manifest.ModuleOverrides = make(map[string]string)
		for _, patchModule := range p.Patches {
			if patchModule.ModuleName != "" && patchModule.Githash != "" {
				manifest.ModuleOverrides[patchModule.ModuleName] = patchModule.Githash
			}
		}
	}

	return manifest, err
}
