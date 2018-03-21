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
}

// Mock version of loadAlias that removes the command line prompt and writing to file
func mockLoadAlias(p *patchParams, conf *ClientSettings, isNewDefault bool) (err error) {
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
