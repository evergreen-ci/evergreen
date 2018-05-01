package distro

import (
	"regexp"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateName(t *testing.T) {
	assert := assert.New(t)

	d := Distro{
		Provider: evergreen.ProviderNameStatic,
	}
	assert.Equal("static", d.Provider)

	d.Provider = evergreen.ProviderNameDocker
	match, err := regexp.MatchString("container-[0-9]+", d.GenerateName())
	assert.NoError(err)
	assert.True(match)

	d.Id = "test"
	d.Provider = "somethingcompletelydifferent"
	match, err = regexp.MatchString("evg-test-[0-9]+-[0-9]+", d.GenerateName())
	assert.NoError(err)
	assert.True(match)
}

func TestGenerateGceName(t *testing.T) {
	assert := assert.New(t)

	r, err := regexp.Compile("(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)")
	assert.NoError(err)
	d := Distro{Id: "name"}

	nameA := d.GenerateName()
	nameB := d.GenerateName()
	assert.True(r.Match([]byte(nameA)))
	assert.True(r.Match([]byte(nameB)))
	assert.NotEqual(nameA, nameB)

	d.Id = "!nv@lid N@m3*"
	invalidChars := d.GenerateName()
	assert.True(r.Match([]byte(invalidChars)))

	d.Id = strings.Repeat("abc", 10)
	tooManyChars := d.GenerateName()
	assert.True(r.Match([]byte(tooManyChars)))
}

func TestFindActive(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	db.Clear(Collection)
	assert := assert.New(t)
	require := require.New(t)

	active, err := FindActive()
	assert.Error(err)
	assert.Len(active, 0)

	d := Distro{
		Id: "foo",
	}
	require.NoError(d.Insert())
	active, err = FindActive()
	assert.NoError(err)
	assert.Len(active, 1)

	d = Distro{
		Id:       "bar",
		Disabled: false,
	}
	require.NoError(d.Insert())
	active, err = FindActive()
	assert.NoError(err)
	assert.Len(active, 2)

	d = Distro{
		Id:       "baz",
		Disabled: true,
	}
	require.NoError(d.Insert())
	active, err = FindActive()
	assert.NoError(err)
	assert.Len(active, 2)

	d = Distro{
		Id:       "qux",
		Disabled: true,
	}
	require.NoError(d.Insert())
	active, err = FindActive()
	assert.NoError(err)
	assert.Len(active, 2)
}
