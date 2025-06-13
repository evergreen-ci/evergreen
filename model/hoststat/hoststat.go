package hoststat

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HostStat records statistics about host usage by distro.
type HostStat struct {
	ID        primitive.ObjectID `bson:"_id"`
	Timestamp time.Time          `bson:"timestamp"`
	Metadata  HostStatMetadata   `bson:"metadata"`
	NumHosts  int                `bson:"num_hosts"`
}

type HostStatMetadata struct {
	Distro string `bson:"distro"`
}

func NewHostStat(distro string, numHosts int) *HostStat {
	return &HostStat{
		ID: primitive.NewObjectID(),
		// kim: NOTE: rounded to the nearest minute to reduce density of data
		// points. Host allocator runs more than once per minute.
		Timestamp: time.Now(),
		Metadata: HostStatMetadata{
			Distro: distro,
		},
		NumHosts: numHosts,
	}
}

// kim: NOTE: need to suppress duplicate key errors if the compound
// index on timestamp requires unique timestamps.
func (hs *HostStat) Insert(ctx context.Context) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, hs)
	return err
}
