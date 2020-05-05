package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMostRecentlyAddedDevice(t *testing.T) {
	lsblkOutput := []string{
		"NAME        MAJ:MIN RM  SIZE RO TYPE MOUNTPOINT                  UUID",
		"loop0         7:0    0 93.8M  1 loop /snap/core/8935             ",
		"nvme0n1     259:0    0   64G  0 disk                             ",
		"`-nvme0n1p1 259:1    0   64G  0 part /                           6156ec80-9446-4eb1-95e0-9ae6b7a46187",
		"nvme1n1     259:2    0    1G  0 disk /user_home                  fee4e1cc-1b86-4cee-8dd3-96f52f5b3ecb",
	}

	deviceName, deviceUUID, err := getMostRecentlyAddedDevice(lsblkOutput)
	assert.NoError(t, err)
	assert.Equal(t, "/dev/nvme1n1", deviceName)
	assert.Equal(t, "fee4e1cc-1b86-4cee-8dd3-96f52f5b3ecb", deviceUUID)
}
