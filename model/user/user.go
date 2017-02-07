package user

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type DBUser struct {
	Id           string       `bson:"_id"`
	FirstName    string       `bson:"first_name"`
	LastName     string       `bson:"last_name"`
	DispName     string       `bson:"display_name"`
	EmailAddress string       `bson:"email"`
	PatchNumber  int          `bson:"patch_number"`
	PubKeys      []PubKey     `bson:"public_keys" json:"public_keys"`
	CreatedAt    time.Time    `bson:"created_at"`
	Settings     UserSettings `bson:"settings"`
	APIKey       string       `bson:"apikey"`
}

type PubKey struct {
	Name      string    `bson:"name" json:"name"`
	Key       string    `bson:"key" json:"key"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

type UserSettings struct {
	Timezone     string `json:"timezone" bson:"timezone"`
	NewWaterfall bool   `json:"new_waterfall" bson:"new_waterfall"`
}

func (u *DBUser) Username() string {
	return u.Id
}

func (u *DBUser) DisplayName() string {
	if u.DispName != "" {
		return u.DispName
	}
	return u.Id
}

func (u *DBUser) Email() string {
	return u.EmailAddress
}

func (u *DBUser) GetAPIKey() string {
	return u.APIKey
}

func (u *DBUser) GetPublicKey(keyname string) (string, error) {
	for _, publicKey := range u.PubKeys {
		if publicKey.Name == keyname {
			return publicKey.Key, nil
		}
	}
	return "", fmt.Errorf("Unable to find public key '%v' for user '%v'", keyname, u.Username())
}

func (u *DBUser) PublicKeys() []PubKey {
	return u.PubKeys
}

func (u *DBUser) Insert() error {
	u.CreatedAt = time.Now()
	return db.Insert(Collection, u)
}

// IncPatchNumber increases the count for the user's patch submissions by one,
// and then returns the new count.
func (u *DBUser) IncPatchNumber() (int, error) {
	dbUser := &DBUser{}
	_, err := db.FindAndModify(
		Collection,
		bson.M{
			IdKey: u.Id,
		},
		nil,
		mgo.Change{
			Update: bson.M{
				"$inc": bson.M{
					PatchNumberKey: 1,
				},
			},
			Upsert:    true,
			ReturnNew: true,
		},
		dbUser,
	)
	if err != nil {
		return 0, err
	}
	return dbUser.PatchNumber, nil

}
