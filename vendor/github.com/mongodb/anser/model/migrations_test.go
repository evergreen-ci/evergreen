package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMigrationMetadata(t *testing.T) {
	assert := assert.New(t)

	meta := &MigrationMetadata{}
	assert.False(meta.Completed)
	assert.False(meta.HasErrors)
	assert.False(meta.Satisfied())

	meta.Completed = true
	assert.True(meta.Satisfied())

	meta.HasErrors = true
	assert.False(meta.Satisfied())
}
