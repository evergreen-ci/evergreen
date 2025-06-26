package hoststat

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HostStat records statistics about host usage by distro.
type HostStat struct {
	ID        string    `bson:"_id"`
	Distro    string    `bson:"distro"`
	Timestamp time.Time `bson:"timestamp"`
	NumHosts  int       `bson:"num_hosts"`
}

func NewHostStat(distroID string, numHosts int) *HostStat {
	return &HostStat{
		ID:        primitive.NewObjectID().Hex(),
		Timestamp: time.Now().Round(time.Minute),
		Distro:    distroID,
		NumHosts:  numHosts,
	}
}

func (hs *HostStat) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, hs)
}
