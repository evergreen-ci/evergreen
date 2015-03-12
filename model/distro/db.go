package distro

import (
	"10gen.com/mci/db/bsonutil"
)

var (
	// bson fields for the distro struct
	IdKey           = bsonutil.MustHaveTag(Distro{}, "Name")
	ArchKey         = bsonutil.MustHaveTag(Distro{}, "Arch")
	SpawnAllowedKey = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	SetupKey        = bsonutil.MustHaveTag(Distro{}, "Setup")
	SetupMCIOnlyKey = bsonutil.MustHaveTag(Distro{}, "SetupMciOnly")
	SetupAsSudoKey  = bsonutil.MustHaveTag(Distro{}, "SetupAsSudo")
	KeyKey          = bsonutil.MustHaveTag(Distro{}, "Key")
	UserKey         = bsonutil.MustHaveTag(Distro{}, "User")
	HostsKey        = bsonutil.MustHaveTag(Distro{}, "Hosts")
	MaxHostsKey     = bsonutil.MustHaveTag(Distro{}, "MaxHosts")
	ExpansionsKey   = bsonutil.MustHaveTag(Distro{}, "Expansions")

	// bson fields for the spawn userdata struct
	SpawnUserDataFileKey     = bsonutil.MustHaveTag(SpawnUserData{}, "File")
	SpawnUserDataValidateKey = bsonutil.MustHaveTag(SpawnUserData{}, "Validate")
)
