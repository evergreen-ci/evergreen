package bsonutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTag(t *testing.T) {
	assert := assert.New(t)

	var err error
	var tagVal string

	// fetching the bson tag for a missing struct field should return an error

	type sOne struct {
	}

	_, err = Tag(sOne{}, "FieldOne")
	assert.Error(err)

	// fetching the bson tag for a struct field without the tag should return the empty string, and no error
	type sTwo struct {
		FieldOne string
	}

	tagVal, err = Tag(sTwo{}, "FieldOne")
	assert.NoError(err)
	assert.Equal(tagVal, "")

	// fetching the bson tag for a struct field with a specified tag should return the tag value"
	type sThree struct {
		FieldOne string `bson:"tag1"`
		FieldTwo string `bson:"tag2"`
	}

	tagVal, err = Tag(sThree{}, "FieldOne")
	assert.NoError(err)
	assert.Equal(tagVal, "tag1")
	tagVal, err = Tag(sThree{}, "FieldTwo")
	assert.NoError(err)
	assert.Equal(tagVal, "tag2")

	// if there are extra modifiers such as omitempty, they should be ignored",
	type sFour struct {
		FieldOne string `bson:"tag1,omitempty"`
	}

	tagVal, err = Tag(sFour{}, "FieldOne")
	assert.NoError(err)
	assert.Equal(tagVal, "tag1")
}
