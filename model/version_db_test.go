package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestVersionByMostRecentNonIgnored(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))
	ts := time.Now()
	v1 := Version{
		Id:         "v1",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts,
	}
	v2 := Version{
		Id:         "v2",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts.Add(-2 * time.Minute), // created too early
	}
	v3 := Version{
		Id:         "v3",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		CreateTime: ts.Add(time.Second), // created too late
	}

	assert.NoError(t, db.InsertMany(VersionCollection, v1, v2, v3))

	v, err := VersionFindOne(VersionByMostRecentNonIgnored("proj", ts))
	assert.NoError(t, err)
	assert.Equal(t, v.Id, "v1")
}

func TestGetVersionAuthorID(t *testing.T) {
	defer db.ClearCollections(VersionCollection)

	for name, test := range map[string]func(*testing.T){
		"HasAuthorID": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id:       "v0",
				AuthorID: "me",
			}).Insert())
			author, err := GetVersionAuthorID("v0")
			assert.NoError(t, err)
			assert.Equal(t, "me", author)
		},
		"NoVersion": func(t *testing.T) {
			author, err := GetVersionAuthorID("v0")
			assert.Error(t, err)
			assert.Empty(t, author)
		},
		"EmptyAuthorID": func(t *testing.T) {
			assert.NoError(t, (&Version{
				Id: "v0",
			}).Insert())
			author, err := GetVersionAuthorID("v0")
			assert.NoError(t, err)
			assert.Empty(t, author)
		},
	} {
		assert.NoError(t, db.ClearCollections(VersionCollection))
		t.Run(name, test)
	}
}
