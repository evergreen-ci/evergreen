package host

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
)

// IPAddress represents a single public IPv4 address available to use for a
// host.
type IPAddress struct {
	ID string `bson:"_id"`
	// AllocationID is the unique identifier for the allocated IP address.
	AllocationID string `bson:"allocation_id"`
	// HostID is the host that the IP address is associated with, if any.
	HostID string `bson:"host_id,omitempty"`
}

func (a *IPAddress) Insert(ctx context.Context) error {
	return db.Insert(ctx, IPAddressCollection, a)
}
