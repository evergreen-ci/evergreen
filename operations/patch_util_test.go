package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadAlias(t *testing.T) {
	assert := assert.New(t)

	// Normal test, use the default alias
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

	mockLoadAlias(p1, client1, false)
	assert.Equal(p1.Alias, "testAlias")

	// Test that, given no default alias, the user can write a passed-in alias as default
	conf2 := ClientProjectConf{
		Name:     "test2",
		Default:  true,
		Alias:    "",
		Variants: []string{},
		Tasks:    []string{},
	}

	client2 := &ClientSettings{
		APIServerHost: "apiserverhost",
		UIServerHost:  "uiserverhost",
		APIKey:        "apikey",
		User:          "user",
		Projects:      []ClientProjectConf{conf2},
		LoadedFrom:    "-",
	}

	p2 := &patchParams{
		Project:     "test2",
		Variants:    conf2.Variants,
		Tasks:       conf2.Tasks,
		Description: "",
		Alias:       "testAlias2",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	mockLoadAlias(p2, client2, true)
	assert.Equal(client2.Projects[0].Alias, "testAlias2")

	// Test that a passed in alias doesn't replace the default
	conf3 := ClientProjectConf{
		Name:     "test3",
		Default:  true,
		Alias:    "defaultAlias",
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
		Project:     "test3",
		Variants:    conf3.Variants,
		Tasks:       conf3.Tasks,
		Description: "",
		Alias:       "newAlias",
		SkipConfirm: false,
		Finalize:    false,
		Large:       false,
		ShowSummary: false,
	}

	mockLoadAlias(p3, client3, true)
	assert.Equal(p3.Alias, "newAlias")
	assert.Equal(client3.Projects[0].Alias, "defaultAlias")
}

// Mock version of loadAlias that removes the command line prompt and writing to file
func mockLoadAlias(p *patchParams, conf *ClientSettings, isNewDefault bool) {
	if p.Alias != "" {
		defaultAlias := conf.FindDefaultAlias(p.Project)
		if defaultAlias == "" && !p.SkipConfirm && isNewDefault {
			conf.SetDefaultAlias(p.Project, p.Alias)
		}
	} else {
		p.Alias = conf.FindDefaultAlias(p.Project)
	}

	return
}
