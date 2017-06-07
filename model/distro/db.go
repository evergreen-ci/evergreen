package distro

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
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

	UserDataKey = bsonutil.MustHaveTag(Distro{}, "UserData")

	SpawnAllowedKey = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	ExpansionsKey   = bsonutil.MustHaveTag(Distro{}, "Expansions")

	// bson fields for the UserData struct
	UserDataFileKey     = bsonutil.MustHaveTag(UserData{}, "File")
	UserDataValidateKey = bsonutil.MustHaveTag(UserData{}, "Validate")
)

const Collection = "distro"

// All is a query that returns all distros.
var All = db.Query(nil).Sort([]string{IdKey})

// FindOne gets one Distro for the given query.
func FindOne(query db.Q) (*Distro, error) {
	d := &Distro{}
	return d, db.FindOneQ(Collection, query, d)
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
