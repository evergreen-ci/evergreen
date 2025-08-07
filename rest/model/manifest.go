package model

import (
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/utility"
)

type APIManifest struct {
	Id          *string             `json:"id"`
	Revision    *string             `json:"revision"`
	ProjectName *string             `json:"project"`
	Branch      *string             `json:"branch"`
	Modules     []APIManifestModule `json:"modules"`
}

func (m *APIManifest) BuildFromService(mfst *manifest.Manifest) {
	m.Id = utility.ToStringPtr(mfst.Id)
	m.Revision = utility.ToStringPtr(mfst.Revision)
	m.ProjectName = utility.ToStringPtr(mfst.ProjectName)
	m.Branch = utility.ToStringPtr(mfst.Branch)
	for modName, mod := range mfst.Modules {
		apiMod := APIManifestModule{}
		apiMod.BuildFromService(modName, mod)
		m.Modules = append(m.Modules, apiMod)
	}
}

type APIManifestModule struct {
	Name     *string `json:"name"`
	Owner    *string `json:"owner"`
	Repo     *string `json:"repo"`
	Branch   *string `json:"branch"`
	Revision *string `json:"revision"`
	URL      *string `json:"url"`
}

func (m *APIManifestModule) BuildFromService(modName string, mod *manifest.Module) {
	m.Name = utility.ToStringPtr(modName)
	m.Branch = utility.ToStringPtr(mod.Branch)
	m.Repo = utility.ToStringPtr(mod.Repo)
	m.Revision = utility.ToStringPtr(mod.Revision)
	m.Owner = utility.ToStringPtr(mod.Owner)
	m.URL = utility.ToStringPtr(mod.URL)
}
