package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestFindExpansionsForVariant(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ParserProjectCollection))
	pp := ParserProject{
		Id: "v1",
		BuildVariants: []parserBV{
			{
				Name:       "myBV",
				Expansions: util.Expansions{"hello": "world", "goodbye": "mars"},
			},
			{
				Name:       "yourBV",
				Expansions: util.Expansions{"milky": "way"},
			},
		},
	}

	v := &Version{Id: "v1"}
	assert.NoError(t, pp.TryUpsert())
	expansions, err := FindExpansionsForVariant(v, "myBV")
	assert.NoError(t, err)
	assert.Equal(t, expansions["hello"], "world")
	assert.Equal(t, expansions["goodbye"], "mars")
	assert.Empty(t, expansions["milky"])
}
