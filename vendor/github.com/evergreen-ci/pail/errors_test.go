package pail

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyNotFoundError(t *testing.T) {
	assert.False(t, IsKeyNotFoundError(errors.New("err")))
	assert.False(t, IsKeyNotFoundError(nil))
	assert.True(t, IsKeyNotFoundError(NewKeyNotFoundError("err")))
	assert.True(t, IsKeyNotFoundError(NewKeyNotFoundErrorf("err")))
	assert.True(t, IsKeyNotFoundError(NewKeyNotFoundErrorf("err %s", "err")))
	assert.True(t, IsKeyNotFoundError(MakeKeyNotFoundError(errors.New("err"))))
}
