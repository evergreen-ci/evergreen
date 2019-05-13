package operations

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNewClientSettings(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "newclientsettings")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	globalTestConfigPath := filepath.Join(tmpdir, ".evergreen.test.yml")
	err = ioutil.WriteFile(globalTestConfigPath,
		[]byte(`api_server_host: https://some.evergreen.api
ui_server_host: https://some.evergreen.ui
api_key: not-a-valid-token
user: myusername
projects:
- name: my-primary-project
  default: true
  tasks: 
    - all
  alias: some-variants`), 0600)
	assert.NoError(t, err)

	clientSettings, err := NewClientSettings(globalTestConfigPath)
	assert.NoError(t, err)
	assert.Equal(t, ClientSettings{
		APIServerHost: "https://some.evergreen.api",
		UIServerHost:  "https://some.evergreen.ui",
		APIKey:        "not-a-valid-token",
		User:          "myusername",
		LoadedFrom:    globalTestConfigPath,
		Projects: []ClientProjectConf{
			{
				Name:    "my-primary-project",
				Default: true,
				Tasks:   []string{"all"},
				Alias:   "some-variants",
			},
		},
	}, *clientSettings)

	err = ioutil.WriteFile(localConfigPath,
		[]byte(`
user: some-other-username
projects:
- name: my-other-project
  default: true
  tasks: 
    - all
  variants: 
    - all`), 0600)
	assert.NoError(t, err)
	defer os.Remove(localConfigPath)

	localClientSettings, err := NewClientSettings(globalTestConfigPath)
	assert.NoError(t, err)
	assert.Equal(t, ClientSettings{
		// from global config
		APIServerHost: "https://some.evergreen.api",
		// from global config
		UIServerHost: "https://some.evergreen.ui",
		// from global config
		APIKey: "not-a-valid-token",
		// from local config
		User: "some-other-username",
		// from global config
		LoadedFrom: globalTestConfigPath,
		// from local config
		Projects: []ClientProjectConf{
			{
				Name:     "my-other-project",
				Default:  true,
				Tasks:    []string{"all"},
				Variants: []string{"all"},
			},
		},
	}, *localClientSettings)
}

func TestLoadWorkingChangesFromFile(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	tmpdir, err := ioutil.TempDir("", "clientsettings")
	assert.NoError(err)
	defer os.RemoveAll(tmpdir)
	globalTestConfigPath := filepath.Join(tmpdir, ".evergreen.test.yml")

	//Uncommitted changes : true
	fileContents := `patch_uncommitted_changes: true`
	require.NoError(ioutil.WriteFile(globalTestConfigPath, []byte(fileContents), 0644))
	conf, err := NewClientSettings(globalTestConfigPath)
	require.NoError(err)

	assert.True(conf.UncommittedChanges)

	// Working tree: false
	fileContents = `projects:
- name: mci
  default: true`
	require.NoError(ioutil.WriteFile(globalTestConfigPath, []byte(fileContents), 0644))
	conf, err = NewClientSettings(globalTestConfigPath)
	require.NoError(err)

	assert.False(conf.UncommittedChanges)
}
