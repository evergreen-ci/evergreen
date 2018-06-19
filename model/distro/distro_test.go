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
	assert := assert.New(t)
	require := require.New(t)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	require.NoError(db.Clear(Collection))

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

func TestComputeParentsToDecommission(t *testing.T) {
	assert := assert.New(t)

	d1 := &Distro{
		Id:            "d1",
		MaxContainers: 100,
	}

	d2 := &Distro{
		Id:            "d2",
		MaxContainers: 0,
	}

	// No containers --> decommission all parents
	c1, err := d1.ComputeParentsToDecommission(5, 0)
	assert.NoError(err)
	assert.Equal(c1, 5)

	// Max containers --> decommission no parents
	c2, err := d1.ComputeParentsToDecommission(5, 500)
	assert.NoError(err)
	assert.Equal(c2, 0)

	// Some containers --> decommission excess parents
	c3, err := d1.ComputeParentsToDecommission(5, 250)
	assert.NoError(err)
	assert.Equal(c3, 2)

	// MaxContainers is zero --> throw error (cannot divide by 0)
	_, err = d2.ComputeParentsToDecommission(5, 10)
	assert.EqualError(err, "Distro does not support containers")
}
