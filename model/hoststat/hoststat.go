package hoststat

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HostStat records statistics about host usage by distro.
type HostStat struct {
	ID        string    `bson:"_id"`
	DistroID  string    `bson:"distro"`
	Timestamp time.Time `bson:"timestamp"`
	NumHosts  int       `bson:"num_hosts"`
}

const tsFormat = "2006-01-02.15-04-05"

func NewHostStat(distroID string, numHosts int) *HostStat {
	return &HostStat{
		ID:        primitive.NewObjectID().Hex(),
		Timestamp: time.Now().Round(time.Minute),
		DistroID:  distroID,
		NumHosts:  numHosts,
	}
}

// kim: NOTE: need to suppress duplicate key errors if the compound
// index on timestamp requires unique timestamps.
func (hs *HostStat) Insert(ctx context.Context) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, hs)
	return err
}
