package distro

import (
	"context"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func FindApplicableDistroIDs(ctx context.Context, id string) ([]string, error) {
	d, err := FindOne(ctx, ById(id), options.FindOne().SetProjection(bson.M{AliasesKey: 1}))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if d == nil {
		return nil, errors.Errorf("error finding distro '%s'", id)
	}

	out := []string{id}
	out = append(out, d.Aliases...)

	return out, nil
}

type byPoolSize []Distro

func (ps byPoolSize) Len() int      { return len(ps) }
func (ps byPoolSize) Swap(i, j int) { ps[i], ps[j] = ps[j], ps[i] }
func (ps byPoolSize) Less(i, j int) bool {
	first := ps[i]
	second := ps[j]

	if first.Provider != second.Provider {
		// always prefer docker hosts
		if utility.StringSliceContains(evergreen.ProviderContainer, first.Provider) {
			return true
		}
		if utility.StringSliceContains(evergreen.ProviderContainer, second.Provider) {
			return false
		}

		// always prefer not static providers
		if !first.IsEphemeral() {
			return false
		}

		if !second.IsEphemeral() {
			return true
		}
	}

	return first.GetPoolSize() > second.GetPoolSize()
}

type AliasLookupTable map[string][]string

func NewDistroAliasesLookupTable(ctx context.Context) (AliasLookupTable, error) {
	all, err := AllDistros(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding all distros")
	}

	return buildCache(all), nil
}

func (da AliasLookupTable) Expand(in []string) []string {
	expanded := []string{}
	for _, id := range in {
		exp := da[id]
		if len(exp) == 0 {
			expanded = append(expanded, id)
		} else {
			expanded = append(expanded, exp...)
		}
	}

	seen := map[string]struct{}{}
	out := []string{}
	for _, id := range expanded {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	return out
}

func buildCache(all []Distro) map[string][]string {
	cache := map[string][]Distro{}

	for _, d := range all {
		name := append(cache[d.Id], d)
		cache[d.Id] = name

		for _, a := range d.Aliases {
			aliases := append(cache[a], d)
			cache[a] = aliases
		}
	}

	out := map[string][]string{}
	for k, v := range cache {
		sorted := byPoolSize(v)
		sort.Sort(sorted)
		aliases := []string{}
		for _, it := range sorted {
			aliases = append(aliases, it.Id)
		}
		out[k] = aliases
	}

	return out
}
