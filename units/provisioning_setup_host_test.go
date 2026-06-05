package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestAttachVolumePassesHostTagToCreateVolume(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection))
	})

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))

	h := &host.Host{
		Id:  "i-ec2-instance-id",
		Tag: "ht_intent_id",
		Distro: distro.Distro{
			Arch:     "linux/amd64",
			Provider: evergreen.ProviderNameMock,
			BootstrapSettings: distro.BootstrapSettings{
				Communication: distro.CommunicationMethodSSH,
			},
		},
		HomeVolumeSize: 10,
		Zone:           "us-east-1a",
	}
	require.NoError(t, h.Insert(t.Context()))

	// AttachVolume fails because the mock instance isn't registered, so SetHost
	// never runs and the volume retains the Host value set at creation.
	err := attachVolume(t.Context(), env, h)
	require.Error(t, err)
	require.NotEmpty(t, h.HomeVolumeID, "volume must have been created before the attach error")

	volume, err := host.FindVolumeByID(t.Context(), h.HomeVolumeID)
	require.NoError(t, err)
	require.NotNil(t, volume)
	assert.Equal(t, h.Tag, volume.Host, "CreateVolume should be called with Host set to the evergreen intent host ID (h.Tag)")
}

func TestGetMostRecentlyAddedDevice(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	lsblkOutput := `{
	"blockdevices": [
		{"name": "loop0", "fstype": "squashfs", "label": null, "uuid": null, "mountpoint": "/snap/core/8935"},
		{"name": "loop1", "fstype": "squashfs", "label": null, "uuid": null, "mountpoint": "/snap/amazon-ssm-agent/1566"},
		{"name": "loop2", "fstype": "squashfs", "label": null, "uuid": null, "mountpoint": "/snap/core/9066"},
		{"name": "nvme0n1", "fstype": null, "label": null, "uuid": null, "mountpoint": null,
			"children": [
				{"name": "nvme0n1p1", "fstype": "ext4", "label": "cloudimg-rootfs", "uuid": "6156ec80-9446-4eb1-95e0-9ae6b7a46187", "mountpoint": "/"}
			]
		},
		{"name": "nvme1n1", "fstype": "xfs", "label": null, "uuid": "fee4e1cc-1b86-4cee-8dd3-96f52f5b3ecb", "mountpoint": "/user_home"}
	]
}`

	devices, err := parseLsblkOutput(lsblkOutput)
	assert.NoError(t, err)
	assert.Len(t, devices, 5)
	assert.Equal(t, "nvme1n1", devices[len(devices)-1].Name)
	assert.Equal(t, "fee4e1cc-1b86-4cee-8dd3-96f52f5b3ecb", devices[len(devices)-1].UUID)
	assert.Equal(t, "xfs", devices[len(devices)-1].FSType)
	assert.Equal(t, "/user_home", devices[len(devices)-1].MountPoint)
}
