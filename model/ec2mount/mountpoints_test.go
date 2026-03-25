package ec2mount

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMountPointsFromProviderDocument(t *testing.T) {
	settings := struct {
		Region      string       `bson:"region"`
		MountPoints []MountPoint `bson:"mount_points"`
	}{
		Region: "us-east-1",
		MountPoints: []MountPoint{
			{VolumeType: "gp3", Throughput: 200, Size: 100, DeviceName: "/dev/sdf"},
		},
	}
	raw, err := bson.Marshal(settings)
	require.NoError(t, err)
	doc := &birch.Document{}
	require.NoError(t, doc.UnmarshalBSON(raw))

	mps, err := MountPointsFromProviderDocument(doc)
	require.NoError(t, err)
	require.Len(t, mps, 1)
	require.Equal(t, "gp3", mps[0].VolumeType)
	require.EqualValues(t, 200, mps[0].Throughput)
	require.Equal(t, "/dev/sdf", mps[0].DeviceName)
}

func TestMountPointsFromProviderDocument_NilDoc(t *testing.T) {
	mps, err := MountPointsFromProviderDocument(nil)
	require.NoError(t, err)
	require.Nil(t, mps)
}
