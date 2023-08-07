package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

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
