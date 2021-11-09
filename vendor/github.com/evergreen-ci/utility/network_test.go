package utility

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkFinder(t *testing.T) {
	addr, err := GetPublicIP()
	require.NoError(t, err)
	require.NotZero(t, addr)

	assert.True(t, !strings.HasPrefix(addr, "127"))
	assert.True(t, !strings.HasPrefix(addr, "172.16"))
	assert.True(t, len(addr) <= 15)
}
