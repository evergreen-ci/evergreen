package host

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
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

// AssignUnusedIPAddress finds any IP address that's not currently being used by
// a host and assigns the host to it. If no free IP addresses are available,
// this will return a nil IPAddress and no error.
func AssignUnusedIPAddress(ctx context.Context, hostTag string) (*IPAddress, error) {
	var ipAddr IPAddress
	changeInfo, err := db.FindAndModify(ctx, IPAddressCollection, bson.M{
		ipAddressHostTagKey: bson.M{"$exists": false},
	}, []string{}, adb.Change{
		Update:    bson.M{"$set": bson.M{ipAddressHostTagKey: hostTag}},
		ReturnNew: true,
	}, &ipAddr)

	if err != nil {
		return nil, err
	}
	if changeInfo.Updated == 0 {
		return nil, nil
	}
	return &ipAddr, nil
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

// IPAddressUnsetHostTags unsets the host tag for many IP addresses by ID.
func IPAddressUnsetHostTags(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	_, err := db.UpdateAllContext(ctx, IPAddressCollection, bson.M{
		ipAddressIDKey: bson.M{"$in": ids},
	}, bson.M{
		"$unset": bson.M{ipAddressHostTagKey: 1},
	})
	return err
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

// FindStaleIPAddresses finds all IP addresses that are currently assigned to
// a host but that host is no longer actively using the IP address.
func FindStaleIPAddresses(ctx context.Context) ([]IPAddress, error) {
	ipAddrs := []IPAddress{}
	const hostKey = "host"
	err := db.Aggregate(ctx, IPAddressCollection, []bson.M{
		{"$match": bson.M{ipAddressHostTagKey: bson.M{"$exists": true}}},
		{"$lookup": bson.M{
			"from":         Collection,
			"localField":   ipAddressHostTagKey,
			"foreignField": TagKey,
			"as":           hostKey,
		}},
		{"$match": bson.M{"$or": []bson.M{
			// No corresponding host at all.
			{hostKey: bson.M{"$size": 0}},
			// It has a matching host but it's terminated, so the host doesn't
			// need the IP address anymore.
			{bsonutil.GetDottedKeyName(hostKey, StatusKey): evergreen.HostTerminated},
		}}},
	}, &ipAddrs)
	return ipAddrs, err
}
