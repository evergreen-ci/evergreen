package ec2settings

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMountPointsForDistro_NonEC2(t *testing.T) {
	d := &distro.Distro{Provider: evergreen.ProviderNameDocker}
	mps, err := MountPointsForDistro(d, "")
	require.NoError(t, err)
	require.Nil(t, mps)
}

func TestMountPointsForDistro_WithMountPoints(t *testing.T) {
	settings := struct {
		Region      string `bson:"region"`
		MountPoints []struct {
			VolumeType string `bson:"volume_type"`
			Throughput int32  `bson:"throughput"`
			Size       int32  `bson:"size"`
		} `bson:"mount_points"`
	}{
		Region: evergreen.DefaultEC2Region,
		MountPoints: []struct {
			VolumeType string `bson:"volume_type"`
			Throughput int32  `bson:"throughput"`
			Size       int32  `bson:"size"`
		}{
			{VolumeType: evergreen.VolumeTypeGp3, Throughput: 125, Size: 200},
		},
	}
	raw, err := bson.Marshal(settings)
	require.NoError(t, err)
	doc := &birch.Document{}
	require.NoError(t, doc.UnmarshalBSON(raw))

	d := &distro.Distro{
		Provider:             evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{doc},
	}
	mps, err := MountPointsForDistro(d, evergreen.DefaultEC2Region)
	require.NoError(t, err)
	require.Len(t, mps, 1)
	require.Equal(t, evergreen.VolumeTypeGp3, mps[0].VolumeType)
	require.EqualValues(t, 125, mps[0].Throughput)
	require.EqualValues(t, 200, mps[0].Size)
}

func TestMountPointsForDistro_RegionFallback(t *testing.T) {
	makeDoc := func(region string, throughput int32) *birch.Document {
		settings := struct {
			Region      string `bson:"region"`
			MountPoints []struct {
				VolumeType string `bson:"volume_type"`
				Throughput int32  `bson:"throughput"`
			} `bson:"mount_points"`
		}{
			Region: region,
			MountPoints: []struct {
				VolumeType string `bson:"volume_type"`
				Throughput int32  `bson:"throughput"`
			}{
				{VolumeType: evergreen.VolumeTypeGp3, Throughput: throughput},
			},
		}
		raw, err := bson.Marshal(settings)
		require.NoError(t, err)
		doc := &birch.Document{}
		require.NoError(t, doc.UnmarshalBSON(raw))
		return doc
	}

	d := &distro.Distro{
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			makeDoc(evergreen.DefaultEC2Region, 100),
			makeDoc("us-east-2", 200),
		},
	}
	mps, err := MountPointsForDistro(d, "us-east-2")
	require.NoError(t, err)
	require.Len(t, mps, 1)
	require.EqualValues(t, 200, mps[0].Throughput)
}

func TestMountPointsForDistro_NilDistro(t *testing.T) {
	mps, err := MountPointsForDistro(nil, "")
	require.NoError(t, err)
	require.Nil(t, mps)
}
