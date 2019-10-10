package cloud

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestRDPPasswordValidation(t *testing.T) {
	assert := assert.New(t)

	goodPasswords := []string{
		"地火風水心CP!",
		"V3ryStr0ng!",
		`Aaa\aa\`,
	}
	badPasswords := []string{"", "weak", "stilltooweak1", "火火火1"}

	for _, password := range goodPasswords {
		assert.True(host.ValidateRDPPassword(password), password)
	}

	for _, password := range badPasswords {
		assert.False(host.ValidateRDPPassword(password), password)
	}
}

func TestMakeExtendedHostExpiration(t *testing.T) {
	assert := assert.New(t)

	h := host.Host{
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := MakeExtendedSpawnHostExpiration(&h, time.Hour)
	assert.NotZero(expTime)
	assert.NoError(err, expTime.Format(time.RFC3339))
}

func TestMakeExtendedHostExpirationFailsBeyondOneWeek(t *testing.T) {
	assert := assert.New(t)

	h := host.Host{
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}

	expTime, err := MakeExtendedSpawnHostExpiration(&h, 24*14*time.Hour)
	assert.Zero(expTime)
	assert.Error(err, expTime.Format(time.RFC3339))
}
