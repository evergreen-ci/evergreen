package units

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMostRecentlyAddedDevice(t *testing.T) {
	lsblkOutput := []string{
		"NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINT",
		"loop0     7:0    0 88.5M  1 loop ...",
		"xvda    202:0    0   64G  0 disk",
		"`-xvda1 202:1    0   64G  0 part /",
		"xvdc    202:160  0   10G  0 disk",
	}

	output, err := getMostRecentlyAddedDevice(lsblkOutput)
	assert.NoError(t, err)
	assert.Equal(t, "/dev/xvdc", output)

	lsblkOutput = []string{
		"NAME        MAJ:MIN RM SIZE RO TYPE MOUNTPOINT",
		"nvme0n1     259:1    0  40G  0 disk",
		"`-nvme0n1p1 259:2    0  40G  0 part /",
		"nvme1n1     259:0    0  40G  0 disk /data",
		"nvme2n1     259:3    0  10G  0 disk",
	}

	output, err = getMostRecentlyAddedDevice(lsblkOutput)
	assert.NoError(t, err)
	assert.Equal(t, "/dev/nvme2n1", output)
}
