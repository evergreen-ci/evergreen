package host

import (
	"10gen.com/mci"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// DecommissionInactiveStaticHosts decommissions static hosts
// in the database provided their ids aren't contained in the
// passed in activeStaticHosts slice
func DecommissionInactiveStaticHosts(activeStaticHosts []string) error {
	if activeStaticHosts == nil {
		return nil
	}
	err := UpdateAll(
		bson.M{
			IdKey: bson.M{
				"$nin": activeStaticHosts,
			},
			ProviderKey: mci.HostTypeStatic,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: mci.HostDecommissioned,
			},
		},
	)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}
