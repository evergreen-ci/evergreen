package service

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/stretchr/testify/assert"
)

func TestSameModules(t *testing.T) {
	assert := assert.New(t)

	projModules := []model.Module{
		{Name: "enterprise", Repo: "git@github.com:something/enterprise.git", Branch: "master"},
		{Name: "wt", Repo: "git@github.com:else/wt.git", Branch: "develop"},
	}

	manifest1 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.True(sameModules(manifest1, projModules))

	manifest2 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "different branch", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(sameModules(manifest2, projModules))

	manifest3 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
			"extra":      &manifest.Module{Branch: "master", Repo: "repo", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(sameModules(manifest3, projModules))

	manifest4 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt": &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
		},
	}
	assert.False(sameModules(manifest4, projModules))
}
