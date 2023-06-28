package distro

import (
	"context"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistroAliases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("OrderAliases", func(t *testing.T) {
		t.Run("Containers", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "three", Provider: evergreen.ProviderNameStatic},
				{Id: "two", Provider: evergreen.ProviderNameEc2Fleet},
				{Id: "one", Provider: evergreen.ProviderNameDocker},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
		})
		t.Run("Ephemeral", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "two", Provider: evergreen.ProviderNameStatic},
				{Id: "one", Provider: evergreen.ProviderNameEc2Fleet},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
		})
		t.Run("PoolSize", func(t *testing.T) {
			distros := byPoolSize{
				{Id: "three", HostAllocatorSettings: HostAllocatorSettings{MaximumHosts: 10}},
				{Id: "two", HostAllocatorSettings: HostAllocatorSettings{MaximumHosts: 20}},
				{Id: "one", HostAllocatorSettings: HostAllocatorSettings{MaximumHosts: 100}},
			}
			sort.Sort(distros)
			assert.Equal(t, "one", distros[0].Id)
			assert.Equal(t, "two", distros[1].Id)
			assert.Equal(t, "three", distros[2].Id)
		})
	})
	t.Run("AliasLookupTable", func(t *testing.T) {
		t.Run("Builder", func(t *testing.T) {
			lt := buildCache([]Distro{{Id: "name", Aliases: []string{"al0", "al2"}}})
			assert.Len(t, lt, 3)
			assert.Contains(t, lt, "al0")
			assert.Contains(t, lt, "al2")
			assert.Equal(t, []string{"name"}, lt["al0"])
		})
		t.Run("Constructor", func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			t.Run("Empty", func(t *testing.T) {
				lt, err := NewDistroAliasesLookupTable(ctx)
				assert.NoError(t, err)
				assert.NotNil(t, lt)
			})
			t.Run("Simple", func(t *testing.T) {
				require.NoError(t, (&Distro{Id: "one", Aliases: []string{"foo", "bar"}}).Insert())
				require.NoError(t, (&Distro{Id: "two", Aliases: []string{"baz", "bar"}}).Insert())
				lt, err := NewDistroAliasesLookupTable(ctx)
				require.NoError(t, err)
				require.NotNil(t, lt)

				// three unique plus two distros:
				assert.Len(t, lt, 5)
				assert.Len(t, lt["bar"], 2)
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
			t.Run("Missing", func(t *testing.T) {
				assert.Equal(t, []string{"distroOne"}, lt.Expand([]string{"distroOne"}))
				assert.Equal(t, []string{".DOES-NOT-EXIST"}, lt.Expand([]string{".DOES-NOT-EXIST"}))
			})
		})
	})
}
