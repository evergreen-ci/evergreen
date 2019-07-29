package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLI(t *testing.T) {
	opts := &ServiceOptions{}
	cmd := Amboy(opts)
	assert.True(t, cmd.HasName("amboy"))
	assert.False(t, cmd.HasName("cli"))
}
