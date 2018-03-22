package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindDefaultAlias(t *testing.T) {
	assert := assert.New(t)

	// Find the default alias for a project when one is present
	conf1 := ClientProjectConf{
		Name:     "test",
		Default:  true,
		Alias:    "testAlias",
		Variants: []string{},
		Tasks:    []string{},
	}

	client1 := &ClientSettings{
		APIServerHost: "apiserverhost",
		UIServerHost:  "uiserverhost",
		APIKey:        "apikey",
		User:          "user",
		Projects:      []ClientProjectConf{conf1},
		LoadedFrom:    "-",
	}

	p1 := &patchParams{
		Project:     "test",
		Variants:    conf1.Variants,
		Tasks:       conf1.Tasks,
		Description: "",
		Alias:       "",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	assert.Equal(client1.FindDefaultAlias(p1.Project), "testAlias")

	// Find an empty string when no project with the given alias exists
	p2 := &patchParams{
		Project:     "test2",
		Variants:    conf1.Variants,
		Tasks:       conf1.Tasks,
		Description: "",
		Alias:       "testAlias2",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	assert.Empty(client1.FindDefaultAlias(p2.Project))
}

func TestSetDefaultAlias(t *testing.T) {
	assert := assert.New(t)

	// Set the default alias for a project
	conf3 := ClientProjectConf{
		Name:     "test",
		Default:  true,
		Alias:    "",
		Variants: []string{},
		Tasks:    []string{},
	}

	client3 := &ClientSettings{
		APIServerHost: "apiserverhost",
		UIServerHost:  "uiserverhost",
		APIKey:        "apikey",
		User:          "user",
		Projects:      []ClientProjectConf{conf3},
		LoadedFrom:    "-",
	}

	p3 := &patchParams{
		Project:     "test",
		Variants:    conf3.Variants,
		Tasks:       conf3.Tasks,
		Description: "",
		Alias:       "defaultAlias",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	client3.SetDefaultAlias(p3.Project, p3.Alias)
	assert.Len(client3.Projects, 1)
	assert.Equal(client3.Projects[0].Alias, "defaultAlias")

	// If no project is present with the given name, add a new project with the requested alias
	p4 := &patchParams{
		Project:     "test2",
		Variants:    conf3.Variants,
		Tasks:       conf3.Tasks,
		Description: "",
		Alias:       "newDefaultAlias",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	client3.SetDefaultAlias(p4.Project, p4.Alias)
	assert.Len(client3.Projects, 2)
	assert.Equal(client3.Projects[0].Alias, "defaultAlias")
	assert.Equal(client3.Projects[1].Alias, "newDefaultAlias")
}
