/*
   Copyright 2013 Am Laher

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package deb

import (
	"fmt"
)

// Control is the base unit for this library.
// A *Control contains one or more paragraphs
type Control []*Package

// NewControlEmpty returns a package with one empty paragraph and an empty map of ExtraData
func NewControlEmpty() *Control {
	ctrl := &Control{NewPackage()}
	return ctrl
}

// NewControlDefault is a factory for a Control. Name, Version, Maintainer and Description are mandatory.
func NewControlDefault(name, maintainerName, maintainerEmail, shortDescription, longDescription string, addDevPackage bool) *Control {
	ctrl := NewControlEmpty()
	sourcePackage := (*ctrl)[0]
	sourcePackage.Set(SourceFName, name)
	sourcePackage.Set(MaintainerFName, fmt.Sprintf("%s <%s>", maintainerName, maintainerEmail))
	sourcePackage.Set(DescriptionFName, fmt.Sprintf("%s\n%s", shortDescription, longDescription))
	//BuildDepends is empty...
	//add binary package
	binPackage := NewPackage()
	*ctrl = append(*ctrl, binPackage)
	binPackage.Set(PackageFName, name)
	binPackage.Set(DescriptionFName, fmt.Sprintf("%s\n%s", shortDescription, longDescription))
	//depends is empty
	if addDevPackage {
		devPackage := NewPackage()
		*ctrl = append(*ctrl, devPackage)
		devPackage.Set(PackageFName, name+"-dev")
		devPackage.Set(ArchitectureFName, "all")
		devPackage.Set(DescriptionFName, fmt.Sprintf("%s - development package\n%s", shortDescription, longDescription))
	}
	SetDefaults(ctrl)
	return ctrl
}

/*
func (ctrl *Control) NewDevPackage() *Package {
	name := ctrl.Get(PackageFName)
	desc := ctrl.Get(DescriptionFName)
	devPara := NewPackage()
	devPara.Set(PackageFName, name+"-dev")
	devPara.Set(ArchitectureFName, "all")
	sp := strings.SplitN(desc, "\n", 2)
	shortDescription := sp[0]
	longDescription := ""
	if len(sp) > 1 {
		longDescription = sp[1]
	}
	devPara.Set(DescriptionFName, fmt.Sprintf("%s - development package\n%s", shortDescription, longDescription))
	return devPara
}
*/

// SetDefaults sets fields which can be initialised appropriately
// note that Source and Binary packages are detected by the presence of a Source or Package field, respectively.
func SetDefaults(ctrl *Control) {
	for _, pkg := range ctrl.SourceParas() {
		pkg.Set(PriorityFName, PriorityDefault)
		pkg.Set(StandardsVersionFName, StandardsVersionDefault)
		pkg.Set(SectionFName, SectionDefault)
		pkg.Set(FormatFName, FormatDefault)
		pkg.Set(StatusFName, StatusDefault)
	}
	for _, pkg := range ctrl.BinaryParas() {
		if pkg.Get(ArchitectureFName) == "" {
			pkg.Set(ArchitectureFName, "any") //default ...
		}
	}
}

// GetArches resolves architecture(s) and return as a slice
func (ctrl *Control) GetArches() ([]Architecture, error) {
	for _, pkg := range *ctrl {
		_, arch, exists := pkg.GetExtended(ArchitectureFName)
		if exists {
			arches, err := ResolveArches(arch)
			return arches, err
		}
	}
	return nil, fmt.Errorf("Architecture field not set")
}

//Get finds the first occurence of the specified value, checking each paragraph in turn
func (ctrl *Control) Get(key string) string {
	for _, paragraph := range *ctrl {
		_, val, exists := paragraph.GetExtended(key)
		if exists {
			return val
		}
	}
	//not found
	return ""
}

// Copy all fields
func Copy(ctrl *Control) *Control {
	nctrl := &Control{}
	for _, para := range *ctrl {
		*nctrl = append(*nctrl, CopyPara(para))
	}
	return nctrl
}

//BinaryParas returns all paragraphs containing a 'Package' field
func (ctrl *Control) BinaryParas() []*Package {
	paras := []*Package{}
	for _, para := range *ctrl {
		v := para.Get(PackageFName)
		if v != "" {
			paras = append(paras, para)
		}
	}
	return paras
}

//SourceParas returns all paragraphs containing a 'Source' field
func (ctrl *Control) SourceParas() []*Package {
	paras := []*Package{}
	for _, para := range *ctrl {
		v := para.Get(SourceFName)
		if v != "" {
			paras = append(paras, para)
		}
	}
	return paras
}

//GetParasByField finds paragraphs containing a given field
func (ctrl *Control) GetParasByField(key string, val string) []*Package {
	paras := []*Package{}
	nkey := NormaliseFieldKey(key)
	for _, para := range *ctrl {
		v := para.Get(nkey)
		if val == v {
			paras = append(paras, para)
		}
	}
	return paras
}
