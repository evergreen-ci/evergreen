package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespace(t *testing.T) {
	assert := assert.New(t)

	ns := Namespace{}
	assert.Equal("", ns.DB)
	assert.Equal("", ns.Collection)
	assert.False(ns.IsValid())

	ns.DB = "foo"
	assert.False(ns.IsValid())
	for i := 0; i < 72; i++ {
		ns.DB += "0123456789"
	}
	ns.Collection = "bar"
	assert.False(ns.IsValid())
	ns.DB = "foo"

	assert.Equal("foo.bar", ns.String())
	assert.True(ns.IsValid())
}
