package host

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

// IPAddress represents a single public IPv4 address available to use for a
// host.
type IPAddress struct {
	ID string `bson:"_id"`
	// AllocationID is the unique identifier for the allocated IP address.
	AllocationID string `bson:"allocation_id"`
	// HostTag is the unique tag (i.e. the intent host ID) for the host that the IP
	// address is associated with, if any.
	HostTag string `bson:"host_tag,omitempty"`
}

func (a *IPAddress) Insert(ctx context.Context) error {
	return db.Insert(ctx, IPAddressCollection, a)
}

// SetHostTag sets the host tag for the IP address if it is not already set. If
// a host tag is already set, this will return an error.
func (a *IPAddress) SetHostTag(ctx context.Context, hostTag string) error {
	if err := db.UpdateContext(ctx, IPAddressCollection, bson.M{
		ipAddressIDKey: a.ID,
		ipAddressHostTagKey: bson.M{
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			ipAddressHostTagKey: hostTag,
		},
	}); err != nil {
		return err
	}

	a.HostTag = hostTag
	return nil
}

// UnsetHostTag unsets the host tag for the IP address if it's set. If a host
// tag is not set to the expected IP address's host ID, this will return an
// error.
func (a *IPAddress) UnsetHostTag(ctx context.Context) error {
	if err := db.UpdateContext(ctx, IPAddressCollection, bson.M{
		ipAddressIDKey:      a.ID,
		ipAddressHostTagKey: a.HostTag,
	}, bson.M{
		"$unset": bson.M{
			ipAddressHostTagKey: 1,
		},
	}); err != nil {
		return err
	}

	a.HostTag = ""
	return nil
}

// FindUnusedIPAddress finds any IP address that's not currently being used by a
// host.
func FindUnusedIPAddress(ctx context.Context) (*IPAddress, error) {
	ipAddrs := make([]IPAddress, 0, 1)
	err := db.Aggregate(ctx, IPAddressCollection, []bson.M{
		{
			"$match": bson.M{
				ipAddressHostTagKey: bson.M{
					"$exists": false,
				},
			},
		},
		{
			"$sample": bson.M{"size": 1},
		},
	}, &ipAddrs)
	if err != nil {
		return nil, err
	}
	if len(ipAddrs) == 0 {
		return nil, nil
	}
	return &ipAddrs[0], nil
}

// FindIPAddressByAllocationID finds an IP address by the IP address's
// allocation ID.
func FindIPAddressByAllocationID(ctx context.Context, allocationID string) (*IPAddress, error) {
	ipAddr := &IPAddress{}
	err := db.FindOneQContext(ctx, IPAddressCollection, db.Query(bson.M{
		ipAddressAllocationIDKey: allocationID,
	}), ipAddr)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return ipAddr, err
}
