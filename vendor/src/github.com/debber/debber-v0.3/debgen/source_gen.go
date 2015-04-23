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
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/targz"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//SourcePackageGenerator generates source packages using templates and some overrideable behaviours
type SourcePackageGenerator struct {
	SourcePackage   *deb.SourcePackage
	BuildParams     *BuildParams
	TemplateStrings map[string]string
	//DebianFiles map[string]string
	OrigFiles map[string]string
}

//NewSourcePackageGenerator is a factory for SourcePackageGenerator.
func NewSourcePackageGenerator(sourcePackage *deb.SourcePackage, buildParams *BuildParams) *SourcePackageGenerator {
	spgen := &SourcePackageGenerator{SourcePackage: sourcePackage, BuildParams: buildParams}
	spgen.TemplateStrings = defaultTemplateStrings()
	return spgen
}

// ApplyDefaultsPureGo overrides some template variables for pure-Go packages
func (spgen *SourcePackageGenerator) ApplyDefaultsPureGo() {
	spgen.TemplateStrings["rules"] = TemplateDebianRulesForGo
}

// Get the default templates for source packages
func defaultTemplateStrings() map[string]string {
	//defensive copy
	templateStringsSource := map[string]string{}
	for k, v := range TemplateStringsSourceDefault {
		templateStringsSource[k] = v
	}
	return templateStringsSource
}

// GenerateAll builds all the artifacts using the default behaviour.
// Note that we could implement alternative methods in future (e.g. using a GenDiffArchive)
func (spgen *SourcePackageGenerator) GenerateAllDefault() error {
	//1. Build orig archive.
	err := spgen.GenOrigArchive()
	if err != nil {
		return err
	}
	//2. Build debian archive.
	err = spgen.GenDebianArchive()
	if err != nil {
		return err
	}

	//3. Calculate checksums
	checksums, err := spgen.CalcChecksums()

	if err != nil {
		return err
	}
	//4. Build dsc file
	err = spgen.GenDscFile(checksums)
	if err != nil {
		return err
	}
	return err
}

// GenOrigArchive builds <package>.orig.tar.gz
// This contains the original upstream source code and data.
func (spgen *SourcePackageGenerator) GenOrigArchive() error {
	//TODO add/exclude resources to /usr/share
	origFilePath := filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.OrigFileName)
	tgzw, err := targz.NewWriterFromFile(origFilePath)
	defer tgzw.Close()
	if err != nil {
		return err
	}
	twh := NewTarWriterHelper(tgzw.Writer)
	err = twh.AddFilesOrDirs(spgen.OrigFiles)
	if err != nil {
		return err
	}
	err = tgzw.Close()
	if err != nil {
		return err
	}
	if spgen.BuildParams.IsVerbose {
		log.Printf("Created %s", origFilePath)
	}
	return nil
}

// GenDebianArchive builds <package>.debian.tar.gz
// This contains all the control data, changelog, rules, etc
func (spgen *SourcePackageGenerator) GenDebianArchive() error {
	//set up template
	templateVars := NewTemplateData(spgen.SourcePackage.Package)

	// generate .debian.tar.gz (just containing debian/ directory)
	tgzw, err := targz.NewWriterFromFile(filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.DebianFileName))
			if err != nil {
				return err
			}
	defer tgzw.Close()
	twh := NewTarWriterHelper(tgzw.Writer)
	resourceDir := spgen.BuildParams.DebianDir
	templateDir := filepath.Join(spgen.BuildParams.TemplateDir, "source", DebianDir)

	//TODO change this to iterate over specified list of files.
	for debianFile, defaultTemplateStr := range spgen.TemplateStrings {
		debianFilePath := strings.Replace(debianFile, "/", string(os.PathSeparator), -1) //fixing source/options, source/format for local files
		resourcePath := filepath.Join(resourceDir, debianFilePath)
		_, err = os.Stat(resourcePath)
		if err == nil {
			err = twh.AddFile(resourcePath, DebianDir+"/"+debianFile)
			if err != nil {
				return err
			}
		} else {
			controlData, err := TemplateFileOrString(filepath.Join(templateDir, debianFilePath+TplExtension), defaultTemplateStr, templateVars)
			if err != nil {
				return err
			}
			err = twh.AddBytes(controlData, DebianDir+"/"+debianFile, int64(0644))
			if err != nil {
				return err
			}
		}
	}

	// postrm/postinst etc from main store
	for _, scriptName := range deb.MaintainerScripts {
		resourcePath := filepath.Join(spgen.BuildParams.DebianDir, scriptName)
		_, err = os.Stat(resourcePath)
		if err == nil {
			err = twh.AddFile(resourcePath, DebianDir+"/"+scriptName)
			if err != nil {
				return err
			}
		} else {
			templatePath := filepath.Join(spgen.BuildParams.TemplateDir, "source", DebianDir, scriptName+TplExtension)
			_, err = os.Stat(templatePath)
			//TODO handle non-EOF errors
			if err == nil {
				scriptData, err := TemplateFile(templatePath, templateVars)
				if err != nil {
					return err
				}
				err = twh.AddBytes(scriptData, DebianDir+"/"+scriptName, 0755)
				if err != nil {
					return err
				}
			}
		}
	}
	err = tgzw.Close()
	if err != nil {
		return err
	}

	if spgen.BuildParams.IsVerbose {
		log.Printf("Created %s", filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.DebianFileName))
	}
	return nil
}

func (spgen *SourcePackageGenerator) CalcChecksums() (*deb.Checksums, error) {
	cs := new(deb.Checksums)
	err := cs.Add(filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.OrigFileName), spgen.SourcePackage.OrigFileName)
	if err != nil {
		return nil, err
	}
	err = cs.Add(filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.DebianFileName), spgen.SourcePackage.DebianFileName)
	if err != nil {
		return nil, err
	}
	return cs, nil
}

func (spgen *SourcePackageGenerator) GenDscFile(checksums *deb.Checksums) error {
	//set up template
	templateVars := NewTemplateData(spgen.SourcePackage.Package)
	templateVars.Checksums = checksums
	dscData, err := TemplateFileOrString(filepath.Join(spgen.BuildParams.TemplateDir, "source", "dsc.tpl"), TemplateDebianDsc, templateVars)
	if err != nil {
		return err
	}
	dscFilePath := filepath.Join(spgen.BuildParams.DestDir, spgen.SourcePackage.DscFileName)
	err = ioutil.WriteFile(dscFilePath, dscData, 0644)
	if err == nil {
		if spgen.BuildParams.IsVerbose {
			log.Printf("Wrote %s", dscFilePath)
		}
	}
	return err
}

func (spgen *SourcePackageGenerator) GenSourceControlFile() error {
	//set up template
	templateVars := NewTemplateData(spgen.SourcePackage.Package)
	dscData, err := TemplateFileOrString(filepath.Join(spgen.BuildParams.TemplateDir, "source", "control"), TemplateSourcedebControl, templateVars)
	if err != nil {
		return err
	}
	dscFilePath := filepath.Join(spgen.BuildParams.DestDir, "control")
	err = ioutil.WriteFile(dscFilePath, dscData, 0644)
	if err == nil {
		if spgen.BuildParams.IsVerbose {
			log.Printf("Wrote %s", dscFilePath)
		}
	}
	return err
}


func PrepareSourceDebGenerator(ctrl *deb.Control, build *BuildParams) (*SourcePackageGenerator, error) {

	err := build.Init()
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}
	sourcePara := ctrl.SourceParas()[0]
	//sourcePara := deb.CopyPara(sp)
	sourcePara.Set(deb.VersionFName, build.Version)
	sourcePara.Set(deb.FormatFName, deb.FormatDefault)

	//Build ...
	spkg := deb.NewSourcePackage(ctrl)
	spgen := NewSourcePackageGenerator(spkg, build)
/*
	sourcesDestinationDir := sourcePara.Get(deb.SourceFName) + "_" + sourcePara.Get(deb.VersionFName)
	for _, sourceFinder := range build.SourceFinders {
		sourcesRelativeTo := sourceFinder.BaseDir
		var sourceDir string
		if sourceFinder.IncludeDir != "" {
			sourceDir = sourceFinder.IncludeDir
		} else {
			sourceDir = sourceFinder.BaseDir
		}
		spgen.OrigFiles, err = GlobForSources(sourcesRelativeTo, sourceDir, sourceFinder.Glob, sourceFinder.Target + sourcesDestinationDir, []string{build.TmpDir, build.DestDir})
		if err != nil {
			return nil, fmt.Errorf("Error resolving sources: %v", err)
		}
	}
*/
/*
	spgen.OrigFiles, err = GlobForSources(build.SourcesRelativeTo, build.SourceDir, build.SourcesGlob, sourcesDestinationDir, []string{build.TmpDir, build.DestDir})
	if err != nil {
		return nil, fmt.Errorf("Error resolving sources: %v", err)
	}
*/
	return spgen, nil
}
