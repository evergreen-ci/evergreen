package distro

import (
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistroAliases(t *testing.T) {
	t.Run("OrderAliases", func(t *testing.T) {
		t.Run("Containers", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "three", Provider: evergreen.ProviderNameStatic},
				{Id: "two", Provider: evergreen.ProviderNameEc2Auto},
				{Id: "one", Provider: evergreen.ProviderNameDocker},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
		})
		t.Run("Ephemeral", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "two", Provider: evergreen.ProviderNameStatic},
				{Id: "one", Provider: evergreen.ProviderNameEc2Auto},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
		})
		t.Run("PoolSize", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "three", PoolSize: 10},
				{Id: "two", PoolSize: 20},
				{Id: "one", PoolSize: 100},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
			assert.Equal(t, "three", distros[2].Id)
		})
	})
	t.Run("AliasLookupTable", func(t *testing.T) {
		t.Run("Constructor", func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			t.Run("Empty", func(t *testing.T) {
				lt, err := NewDistroAliasesLookupTable()
				assert.NoError(t, err)
				assert.NotNil(t, lt)
			})
			t.Run("Simple", func(t *testing.T) {
				require.NoError(t, (&Distro{Id: "one", Aliases: []string{"foo", "bar"}}).Insert())
				require.NoError(t, (&Distro{Id: "two", Aliases: []string{"baz", "bar"}}).Insert())
				lt, err := NewDistroAliasesLookupTable()
				require.NoError(t, err)
				require.NotNil(t, lt)

				// three unique plus two distros:
				assert.Len(t, lt, 5)
			})
		})
		t.Run("Expansion", func(t *testing.T) {
			lt := AliasLookupTable{
				"aliasOne":  []string{"foo", "bar"},
				"aliasTwo":  []string{"baz", "bar"},
				"distroOne": []string{},
			}

			t.Run("SimpleExpansion", func(t *testing.T) {
				out := lt.Expand([]string{"aliasOne", "distroOne"})
				assert.Equal(t, []string{"foo", "bar", "distroOne"}, out)
			})
			t.Run("RemovesDuplicates", func(t *testing.T) {
				out := lt.Expand([]string{"aliasOne", "aliasTwo"})
				assert.Equal(t, []string{"foo", "bar", "baz"}, out)
			})
		})
	})
}
