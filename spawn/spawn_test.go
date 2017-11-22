package spawn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRDPPasswordValidation(t *testing.T) {
	assert := assert.New(t)

	goodPasswords := []string{
		"地火風水心1!",
		"V3ryStr0ng!",
		"Aaaaa-",
		`Aaaaa\`,
		"Aaaaa(",
		"Aaaaa)",
		"Aaaaa[",
		"Aaaaa]",
		"Aaaaa`",
	}
	badPasswords := []string{"", "weak", "stilltooweak1", "火火火1"}

	for _, password := range goodPasswords {
		assert.True(ValidateRDPPassword(password), password)
	}

	for _, password := range badPasswords {
		assert.False(ValidateRDPPassword(password), password)
	}
}
