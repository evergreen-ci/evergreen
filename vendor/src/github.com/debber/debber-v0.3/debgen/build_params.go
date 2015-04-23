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

package debgen

import (
	"github.com/debber/debber-v0.3/deb"
	"os"
)

// BuildParams provides information about a particular build
type BuildParams struct {
	// Properties below are mainly for build-related properties rather than metadata

	IsVerbose  bool   // Whether to log debug information
	TmpDir     string // Directory in-which to generate intermediate files & archives
	IsRmtemp   bool   // Delete tmp dir after execution?
	DestDir    string // Where to generate .deb files and source debs (.dsc files etc)
	WorkingDir string // This is the root from which to find .go files, templates, resources, etc
	DebianDir  string // This is the debian dir which stores 'control', 'changelog' and 'rules' files

	TemplateDir  string // Optional. Only required if you're using templates
	ResourcesDir string // Optional. Only if debgo packages your resources automatically.

	//TemplateStringsSource map[string]string //Populate this to fulfil templates for the different control files.
/*
	BinFinders        map[deb.Architecture][]FileFinder
/*	BinArmhfGlob      FileFinder
	BinArmelGlob      FileFinder
	BinAmd64Glob      FileFinder
	BinAnyGlob        FileFinder
	ResourcesDir      string
	ResourcesGlob     string
*/

/*
	SourceIncludeDir  string //In this case it's used as 
	SourcesGlob       string
	SourcesRelativeTo string //special variable to ensure that e.g. GOPATH is properly set up.
	
	ResourceFinders []FileFinder
	SourceFinders   []FileFinder
*/
	Version           string
	Arches            []deb.Architecture

}

type FileFinder struct {
	BaseDir string //Usually set this, except for Go sources, where debber will find the base for you based on the IncludeDir
	IncludeDir string //Usually leave blank, except for Go sources.
	Glob string //'Glob' pattern for finding files.
	Target string //Target dir in eventual Deb file
}


//Factory for BuildParams. Populates defaults.
func NewBuildParams() *BuildParams {
	bp := &BuildParams{IsVerbose: false}
	bp.TmpDir = deb.TempDirDefault
	bp.IsRmtemp = true
	bp.DestDir = deb.DistDirDefault
	bp.WorkingDir = deb.WorkingDirDefault
	bp.DebianDir = deb.DebianDir
	bp.TemplateDir = deb.TemplateDirDefault
	bp.ResourcesDir = deb.ResourcesDirDefault
//	bp.BinFinders = map[deb.Architecture][]FileFinder{}
//	bp.ResourceFinders = []FileFinder{ {BaseDir: ".",Glob:"Readme.*", Target: "/usr/share/doc/.{{Package}}/"} }

	//Go only ...
//	bp.SourceFinders = []FileFinder{ {IncludeDir: ".",Glob:"**.go", Target: "/usr/share/gocode/src/"} }
	return bp
}

//Initialise build directories (make Temp and Dest directories)
func (bp *BuildParams) Init() error {
	//make tmpDir if set
	if bp.TmpDir != "" {
		err := os.MkdirAll(bp.TmpDir, 0755)
		if err != nil {
			return err
		}
	}
	//make destDir if set
	if bp.DestDir != "" {
		err := os.MkdirAll(bp.DestDir, 0755)
		if err != nil {
			return err
		}
	}
	//make debianDir if set
	if bp.DebianDir != "" {
		err := os.MkdirAll(bp.DebianDir, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}
