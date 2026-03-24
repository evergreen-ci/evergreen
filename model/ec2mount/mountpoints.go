// Package ec2mount holds shared EC2 provider BSON types and parsing helpers used by cloud and model code
// without import cycles (this package does not import cloud, distro, or task).
package ec2mount

import (
	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// MountPoint describes an EBS mount in EC2 provider settings. BSON/mapstructure/json tags must stay aligned
// with cloud.EC2ProviderSettings usage.
type MountPoint struct {
	VirtualName string `mapstructure:"virtual_name" json:"virtual_name,omitempty" bson:"virtual_name,omitempty"`
	DeviceName  string `mapstructure:"device_name" json:"device_name,omitempty" bson:"device_name,omitempty"`
	Size        int32  `mapstructure:"size" json:"size,omitempty" bson:"size,omitempty"`
	Iops        int32  `mapstructure:"iops" json:"iops,omitempty" bson:"iops,omitempty"`
	Throughput  int32  `mapstructure:"throughput" json:"throughput,omitempty" bson:"throughput,omitempty"`
	SnapshotID  string `mapstructure:"snapshot_id" json:"snapshot_id,omitempty" bson:"snapshot_id,omitempty"`
	VolumeType  string `mapstructure:"volume_type" json:"volume_type,omitempty" bson:"volume_type,omitempty"`
}

type mountPointsOnly struct {
	MountPoints []MountPoint `bson:"mount_points,omitempty"`
}

// MountPointsFromProviderDocument extracts the mount_points array from a single EC2 provider settings
// document. This is the same field subset unmarshaled into cloud.EC2ProviderSettings via FromDocument.
func MountPointsFromProviderDocument(doc *birch.Document) ([]MountPoint, error) {
	if doc == nil {
		return nil, nil
	}
	raw, err := doc.MarshalBSON()
	if err != nil {
		return nil, errors.Wrap(err, "marshaling provider settings document")
	}
	var parsed mountPointsOnly
	if err := bson.Unmarshal(raw, &parsed); err != nil {
		return nil, errors.Wrap(err, "unmarshaling mount_points from provider settings")
	}
	return parsed.MountPoints, nil
}
