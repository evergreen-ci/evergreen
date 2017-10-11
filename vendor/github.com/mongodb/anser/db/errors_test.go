package db

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	mgo "gopkg.in/mgo.v2"
)

func TestResultsPredicate(t *testing.T) {
	assert := assert.New(t)

	assert.False(ResultsNotFound(errors.New("foo")))
	assert.False(ResultsNotFound(nil))
	assert.False(ResultsNotFound(errors.New("not found")))
	assert.True(ResultsNotFound(mgo.ErrNotFound))
}
