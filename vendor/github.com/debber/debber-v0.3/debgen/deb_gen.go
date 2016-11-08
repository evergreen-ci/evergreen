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
	"compress/gzip"
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/targz"
	"log"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//DebGenerator generates source packages using templates and some overrideable behaviours
type DebGenerator struct {
	DebWriter              *deb.Writer
	BuildParams            *BuildParams
	DefaultTemplateStrings map[string]string
	DataFiles              map[string]string
}


//NewDebGenerator is a factory for SourcePackageGenerator.
func NewDebGenerator(debWriter *deb.Writer, buildParams *BuildParams) *DebGenerator {
	dgen := &DebGenerator{DebWriter: debWriter, BuildParams: buildParams,
		DefaultTemplateStrings: map[string]string{}, DataFiles: map[string]string{}}
	return dgen
}

// GenerateAllDefault applies the default build process.
// First it writes each file, then adds them to the .deb as a separate io operation.
func (dgen *DebGenerator) GenerateAllDefault() error {
	if dgen.BuildParams.IsVerbose {
		log.Printf("trying to write control file for %s", dgen.DebWriter.Architecture)
	}
	err := dgen.GenControlArchive()
	if err != nil {
		return err
	}
	err = dgen.GenDataArchive()
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("trying to write .deb file for %s", dgen.DebWriter.Architecture)
	}
	//TODO switch this around
	err = dgen.DebWriter.Build(dgen.BuildParams.TmpDir, dgen.BuildParams.DestDir)
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Closed deb")
	}
	return err
}

//GenControlArchive generates control archive, using a system of templates or files.
//
//First it attempts to find the file inside BuildParams.Resources.
//If that doesn't exist, it attempts to find a template in templateDir
//Finally, it attempts to use a string-based template.
func (dgen *DebGenerator) GenControlArchive() error {
	archiveFilename := filepath.Join(dgen.BuildParams.TmpDir, dgen.DebWriter.ControlArchive)
	controlTgzw, err := targz.NewWriterFromFile(archiveFilename)
	if err != nil {
		return err
	}
	templateVars := &TemplateData{Package: dgen.DebWriter.Control, Deb: dgen.DebWriter}
	//templateVars.Deb = dgen.DebWriter

	cwh := NewTarWriterHelper(controlTgzw.Writer)
	err = dgen.GenControlFile(cwh, templateVars)
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Wrote control file to control archive")
	}
	// This is where you include Postrm/Postinst etc
	for _, scriptName := range deb.MaintainerScripts {
		resourcePath := filepath.Join(dgen.BuildParams.ResourcesDir, DebianDir, scriptName)
		_, err = os.Stat(resourcePath)
		if err == nil {
			err = cwh.AddFile(resourcePath, scriptName)
			if err != nil {
				return err
			}
		} else {
			templatePath := filepath.Join(dgen.BuildParams.TemplateDir, DebianDir, scriptName+TplExtension)
			_, err = os.Stat(templatePath)
			//TODO handle non-EOF errors
			if err == nil {
				scriptData, err := TemplateFile(templatePath, templateVars)
				if err != nil {
					return err
				}
				err = cwh.AddBytes(scriptData, scriptName, 0755)
				if err != nil {
					return err
				}
			}
		}
	}

	err = controlTgzw.Close()
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Closed control archive")
	}
	return err
}

// GenDataArchive generates the archive from files on the file system.
func (dgen *DebGenerator) GenDataArchive() error {
	archiveFilename := filepath.Join(dgen.BuildParams.TmpDir, dgen.DebWriter.DataArchive)
	dataTgzw, err := targz.NewWriterFromFile(archiveFilename)
	if err != nil {
		return err
	}
	dwh := NewTarWriterHelper(dataTgzw.Writer)
	//NOTE: files only for now
	err = dwh.AddFilesOrDirs(dgen.DataFiles)
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Added executables")
	}
	/*
		// TODO add README.debian automatically
		err = TarAddFiles(dataTgzw.Writer, dgen.DebWriter.Package.MappedFiles)
		if err != nil {
			return err
		}
	*/
	if dgen.BuildParams.IsVerbose {
		log.Printf("Added resources")
	}
	err = dataTgzw.Close()
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Closed data archive")
	}
	return err
}

//Generates the control file based on a template
func (dgen *DebGenerator) GenControlFile(cwh *TarWriterHelper, templateVars *TemplateData) error {
	resourcePath := filepath.Join(dgen.BuildParams.ResourcesDir, "debian", "control")
	_, err := os.Stat(resourcePath)
	if err == nil {
		err = cwh.AddFile(resourcePath, "control")
		return err
	}
	//try template or use a string
	controlData, err := TemplateFileOrString(filepath.Join(dgen.BuildParams.TemplateDir, "control.tpl"), TemplateBinarydebControl, templateVars)
	if err != nil {
		return err
	}
	if dgen.BuildParams.IsVerbose {
		log.Printf("Control file:\n%s", string(controlData))
	}
	err = cwh.AddBytes(controlData, "control", 0644)
	return err
}

//Default flow for generating a deb
func PrepareBasicDebGen(ctrl *deb.Control, build *BuildParams) ([]*DebGenerator, error) {
	if build.Version == "" {
		return nil, fmt.Errorf("Error: 'version' is a required field")
	}
	// Initiate build
	err := build.Init()
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	dgens := []*DebGenerator{}
	//Build ... assume just one source package
	sourcePara := ctrl.SourceParas()[0]
	for _, binPara := range ctrl.BinaryParas() {
		//TODO check -dev package here ...
		debpara := deb.CopyPara(binPara)
		debpara.Set(deb.VersionFName, build.Version)
		debpara.Set(deb.SectionFName, sourcePara.Get(deb.SectionFName))
		debpara.Set(deb.MaintainerFName, sourcePara.Get(deb.MaintainerFName))
		debpara.Set(deb.PriorityFName, sourcePara.Get(deb.PriorityFName))
		//log.Printf("debPara: %+v", debpara)
		
		dataFiles := map[string]string{}
		//source package. TODO: find a better way to identify a sources-only .deb package.
		if strings.HasSuffix(binPara.Get(deb.PackageFName), "-dev") {
			/*
				if build.sourcesGlob != "" {
					sources, err = filepath.Glob(build.sourcesGlob)
					if err != nil {
						return fmt.Errorf("%v", err)
					}
					log.Printf("sources matching glob: %+v", sources)
				}*/
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
				sources, err := GlobForSources(sourcesRelativeTo, sourceDir, sourceFinder.Glob, sourceFinder.Target + sourcesDestinationDir, []string{build.TmpDir, build.DestDir})
				if err != nil {
					return nil, fmt.Errorf("Error resolving sources: %v", err)
				}
				for k, v := range sources {
					dataFiles[k] = v
				}
			}
*/
		} else {
			// no sources
		}
		//Add changelog
		cl := filepath.Join(build.DebianDir, "changelog")
		cf, err := os.Open(cl)
		if os.IsNotExist(err) {
		} else if err != nil {
			return nil, err
		} else {
			defer cf.Close()
			changelogGz := filepath.Join(build.TmpDir, "changelog.gz")
			gzf, err := os.OpenFile(changelogGz, os.O_CREATE | os.O_WRONLY, 0644)
			defer gzf.Close()
			if err != nil {
				return nil, fmt.Errorf("Error creating temp gzip file for changelog: %v", err)
			}
			gzw, err := gzip.NewWriterLevel(gzf, gzip.BestCompression)
			if err != nil {
				return nil, fmt.Errorf("Error gzipping changelog: %v", err)
			}
			_, err = io.Copy(gzw, cf)
			if err != nil {
				return nil, fmt.Errorf("Error gzipping changelog: %v", err)
			}
			err = gzw.Close()
			if err != nil {
				return nil, fmt.Errorf("Error gzipping changelog: %v", err)
			}
			err = cf.Close()
			if err != nil {
				return nil, fmt.Errorf("Error gzipping changelog: %v", err)
			}
			err = gzf.Close()
			if err != nil {
				return nil, fmt.Errorf("Error gzipping changelog: %v", err)
			}
			//exists. Add it.
			dataFiles["/usr/share/doc/"+binPara.Get(deb.PackageFName)+"/changelog.gz"] = changelogGz
		}
		//Add copyright
		cr := filepath.Join(build.DebianDir, "copyright")
		_, err = os.Stat(cr)
		if os.IsNotExist(err) {
		} else if err != nil {
			return nil, err
		} else {
			//exists. Add it.
			dataFiles["/usr/share/doc/"+binPara.Get(deb.PackageFName)+"/copyright"] = cr
		}
		//log.Printf("Resources: %v", build.Resources)
		// TODO determine this platform
		//err = bpkg.Build(build, debgen.GenBinaryArtifact)
		artifacts, err := deb.NewWriters(&deb.Control{debpara})
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		for arch, artifact := range artifacts {
			required := false
			for _, requiredArch := range build.Arches {
				if arch == requiredArch {
					//yes
					required = true
				}
			}
			if !required {
				continue
			}
			dgen := NewDebGenerator(artifact, build)
			dgens = append(dgens, dgen)
			dgen.DataFiles = map[string]string{}
			for k, v := range dataFiles {
				dgen.DataFiles[k] = v
			}
			/*
				for _, source := range sources {
					//NOTE: this should not use filepath.Join because it should always use forward-slash
					dgen.DataFiles[build.sourcesDest+"/"+source] = source
				}
			*/
			//add changelog
			
	

			// add resources ...
			f := func(path string, info os.FileInfo, err2 error) error {
				if info != nil && !info.IsDir() {
					rel, err := filepath.Rel(build.ResourcesDir, path)
					if err == nil {
						dgen.DataFiles[rel] = path
					}
					return err
				}
				return nil
			}
			err = filepath.Walk(build.ResourcesDir, f)

			if err != nil {
				return nil, fmt.Errorf("%v", err)
			}
/*
			binFinders, ok := build.BinFinders[arch]
			if ok {
				for _, binFinder := range binFinders {
					if binFinder.Glob != "" {
						glob := filepath.Join(binFinder.BaseDir, binFinder.Glob)
						bins, err := filepath.Glob(glob)
						if err != nil {
							return nil, fmt.Errorf("%v", err)
						}
						log.Printf("glob: %s, bins: %v", glob, bins)
						//log.Printf("Binaries matching glob for '%s': %+v", arch, bins)
						for _, bin := range bins {
							target := binFinder.Target + "/" + bin
							dgen.DataFiles[target] = bin
						}
					}
				}
			}
*/
//			log.Printf("All data files: %v", dgen.DataFiles)
		}
	}
	return dgens, nil
}

/*
	archBinDir := filepath.Join(binDir, string(arch))
	err = filepath.Walk(archBinDir, func(path string, info os.FileInfo, err2 error) error {
		if info != nil && !info.IsDir() {
			rel, err := filepath.Rel(binDir, path)
			if err == nil {
				dgen.DataFiles[rel] = path
			}
			return err
		}
		return nil
	})
*/
/*

			err = dgen.GenerateAllDefault()
			if err != nil {
				return fmt.Errorf("Error building for '%s': %v", arch, err)
			}
		}
	}
	return nil
}
*/
