package model

import (
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/utility"
)

type APIManifest struct {
	Id              *string             `json:"id" extensions:"!x-nullable"`
	Revision        *string             `json:"revision" extensions:"!x-nullable"`
	ProjectName     *string             `json:"project" extensions:"!x-nullable"`
	Branch          *string             `json:"branch" extensions:"!x-nullable"`
	IsBase          bool                `json:"is_base"`
	Modules         []APIManifestModule `json:"modules" extensions:"x-nullable"`
	PreviousVersion *string             `json:"previous_version,omitempty"`
	ChangedModules  []string            `json:"changed_modules,omitempty"`
}

func (m *APIManifest) BuildFromService(mfst *manifest.Manifest) {
	m.Id = utility.ToStringPtr(mfst.Id)
	m.Revision = utility.ToStringPtr(mfst.Revision)
	m.ProjectName = utility.ToStringPtr(mfst.ProjectName)
	m.Branch = utility.ToStringPtr(mfst.Branch)
	m.IsBase = mfst.IsBase
	if mfst.PreviousVersion != "" {
		m.PreviousVersion = utility.ToStringPtr(mfst.PreviousVersion)
	}
	m.ChangedModules = mfst.ChangedModules
	for modName, mod := range mfst.Modules {
		apiMod := APIManifestModule{}
		apiMod.BuildFromService(modName, mod)
		m.Modules = append(m.Modules, apiMod)
	}
}

type APIManifestModule struct {
	Name          *string `json:"name" extensions:"!x-nullable"`
	Owner         *string `json:"owner" extensions:"!x-nullable"`
	Repo          *string `json:"repo" extensions:"!x-nullable"`
	Branch        *string `json:"branch" extensions:"!x-nullable"`
	Revision      *string `json:"revision" extensions:"!x-nullable"`
	URL           *string `json:"url" extensions:"!x-nullable"`
	SelectionRule *string `json:"selection_rule,omitempty"`
}

func (m *APIManifestModule) BuildFromService(modName string, mod *manifest.Module) {
	m.Name = utility.ToStringPtr(modName)
	m.Branch = utility.ToStringPtr(mod.Branch)
	m.Repo = utility.ToStringPtr(mod.Repo)
	m.Revision = utility.ToStringPtr(mod.Revision)
	m.Owner = utility.ToStringPtr(mod.Owner)
	m.URL = utility.ToStringPtr(mod.URL)
	if mod.SelectionRule != "" {
		m.SelectionRule = utility.ToStringPtr(string(mod.SelectionRule))
	}
}
