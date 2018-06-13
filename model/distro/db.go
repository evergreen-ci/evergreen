package distro

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

var (
	// bson fields for the Distro struct
	IdKey               = bsonutil.MustHaveTag(Distro{}, "Id")
	ArchKey             = bsonutil.MustHaveTag(Distro{}, "Arch")
	PoolSizeKey         = bsonutil.MustHaveTag(Distro{}, "PoolSize")
	ProviderKey         = bsonutil.MustHaveTag(Distro{}, "Provider")
	ProviderSettingsKey = bsonutil.MustHaveTag(Distro{}, "ProviderSettings")
	SetupAsSudoKey      = bsonutil.MustHaveTag(Distro{}, "SetupAsSudo")
	SetupKey            = bsonutil.MustHaveTag(Distro{}, "Setup")
	UserKey             = bsonutil.MustHaveTag(Distro{}, "User")
	SSHKeyKey           = bsonutil.MustHaveTag(Distro{}, "SSHKey")
	SSHOptionsKey       = bsonutil.MustHaveTag(Distro{}, "SSHOptions")
	WorkDirKey          = bsonutil.MustHaveTag(Distro{}, "WorkDir")
	SpawnAllowedKey     = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	ExpansionsKey       = bsonutil.MustHaveTag(Distro{}, "Expansions")
	DisabledKey         = bsonutil.MustHaveTag(Distro{}, "Disabled")
	MaxContainersKey    = bsonutil.MustHaveTag(Distro{}, "MaxContainers")
)

const Collection = "distro"

// All is a query that returns all distros.
var All = db.Query(nil).Sort([]string{IdKey})

// FindOne gets one Distro for the given query.
func FindOne(query db.Q) (Distro, error) {
	d := Distro{}
	return d, db.FindOneQ(Collection, query, &d)
}

func FindActive() ([]string, error) {
	out := []struct {
		Distros []string `bson:"distros"`
	}{}
	err := db.Aggregate(Collection, []bson.M{
		{
			"$match": bson.M{
				DisabledKey: bson.M{
					"$exists": false,
				},
			},
		},
		{
			"$project": bson.M{
				IdKey: 1,
			},
		},
		{
			"$group": bson.M{
				"_id": 0,
				"distros": bson.M{
					"$push": "$_id",
				},
			},
		},
	}, &out)

	if err != nil {
		return nil, errors.Wrap(err, "problem building list of all distros")
	}

	if len(out) != 1 {
		return nil, errors.New("produced invalid results")
	}

	return out[0].Distros, nil
}

// FindActiveWithContainers gets all active distros that support containers
func FindActiveWithContainers() ([]string, error) {
	out := []struct {
		Distros []string `bson:"distros"`
	}{}
	err := db.Aggregate(Collection, []bson.M{
		{
			"$match": bson.M{
				DisabledKey: bson.M{
					"$exists": false,
				},
				MaxContainersKey: bson.M{
					"$gt": 0,
				},
			},
		},
		{
			"$project": bson.M{
				IdKey: 1,
			},
		},
		{
			"$group": bson.M{
				"_id": 0,
				"distros": bson.M{
					"$push": "$_id",
				},
			},
		},
	}, &out)

	if err != nil {
		return nil, errors.Wrap(err, "problem building list of all distros supporting containers")
	}

	if len(out) != 1 {
		return nil, errors.New("produced invalid results")
	}

	return out[0].Distros, nil
}

// Find gets every Distro matching the given query.
func Find(query db.Q) ([]Distro, error) {
	distros := []Distro{}
	err := db.FindAllQ(Collection, query, &distros)
	return distros, err
}

// Insert writes the distro to the database.
func (d *Distro) Insert() error {
	return db.Insert(Collection, d)
}

// Update updates one distro.
func (d *Distro) Update() error {
	return db.UpdateId(Collection, d.Id, d)
}

// Remove removes one distro.
func Remove(id string) error {
	return db.Remove(Collection, bson.D{{IdKey, id}})
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) db.Q {
	return db.Query(bson.D{{IdKey, id}})
}

// ByProvider returns a query that contains a Provider selector on the string, p.
func ByProvider(p string) db.Q {
	return db.Query(bson.D{{ProviderKey, p}})
}

// BySpawnAllowed returns a query that contains the SpawnAllowed selector.
func BySpawnAllowed() db.Q {
	return db.Query(bson.D{{SpawnAllowedKey, true}})
}
