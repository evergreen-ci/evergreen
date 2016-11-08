package main

import (
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

var cmdName = "debber"

func copyrightInitTask(input []string) error {
	return fmt.Errorf("copyright:init not implemented yet")
}
func copyrightInit(ctrl *deb.Control, build *debgen.BuildParams, license string) error {
	filename := filepath.Join(build.DebianDir, "copyright")
	templateVars := debgen.NewTemplateData(ctrl)
	templateVars.ExtraData["License"] = license
	templateVars.ExtraData["Year"] = time.Now().Format("2006")
	err := os.MkdirAll(filepath.Join(build.ResourcesDir, "debian"), 0777)
	if err != nil {
		return fmt.Errorf("Error making dirs: %v", err)
	}
	var f io.WriteCloser
	tpl, err := template.New("copyright").Parse(debgen.TemplateCopyrightBasic)
	if err != nil {
		return fmt.Errorf("Error parsing template: %v", err)
	}
	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		//create ..
		f, err = os.OpenFile(filename, os.O_WRONLY | os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("Error creating file: %v", err)
		}
		defer f.Close()
	} else if err != nil {
		return fmt.Errorf("Error reading existing changelog: %v", err)
	} else {
		//append..
		f, err := os.OpenFile(filename, os.O_WRONLY | os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("Error opening file: %v", err)
		}
		defer f.Close()
	}
	err = tpl.Execute(f, templateVars)
	if err != nil {
		return fmt.Errorf("Error executing template: %v", err)
	}
	err = f.Close()
	if err != nil {
		return fmt.Errorf("Error closing written file: %v", err)
	}
	return nil

}

func controlInitTask(input []string) error {

	return fmt.Errorf("controlt:init not implemented yet")
}

func rulesInitTask(input []string) error {

	return fmt.Errorf("rules:init not implemented yet")
}

func initDebber(input []string) error {
	//set to nothing
	ctrl := deb.NewControlEmpty()
	build := debgen.NewBuildParams()
	build.DestDir = "debian"
	fs := InitBuildFlags(cmdName+" init", build)
	var entry string
	var overwrite bool
	var flavour string
	var pkgName, maintainerName, maintainerEmail, shortDescription, longDescription, architecture string
	var license string
	fs.StringVar(&pkgName, "name", "", "Package name [required]")
	fs.StringVar(&maintainerName, "maintainer", "", "Maintainer name [required]")
	fs.StringVar(&maintainerEmail, "maintainer-email", "", "Maintainer's email address [required]")
	fs.StringVar(&shortDescription, "desc", "", "Package description [required]")
	fs.StringVar(&longDescription, "long-desc", "", "Package Long description")
	fs.StringVar(&entry, "entry", "Initial project import", "Changelog entry data")
	fs.BoolVar(&overwrite, "overwrite", false, "Overwrite existing files")
	fs.StringVar(&architecture, "architecture", "any", "Package Architecture (any)")
	fs.StringVar(&license, "license", "BSD-3-clause", "License")
	fs.StringVar(&flavour, "flav", "go:exe", "'flavour' implies a set of defaults - currently, one of 'go:exe', 'go:pkg', 'dev' or ''")
	//TODO flavour
	err := fs.Parse(os.Args[2:])
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	if pkgName == "" || maintainerName == "" || maintainerEmail == "" || shortDescription == "" || longDescription == "" {
		return fmt.Errorf("Required fields: --name, --maintainer, --maintainer-email, --desc, --long-desc")
	}
	//handle spaces in longDescription. TODO: utility function
	longDescriptions := strings.Split(longDescription, "\n")
	longDescription = ""
	for _, ldl := range longDescriptions {
		if longDescription != "" {
			longDescription += "\n"
		}
		longDescription += " " + strings.TrimSpace(ldl)
	}
	(*ctrl)[0].Set(deb.SourceFName, pkgName)
	(*ctrl)[0].Set(deb.MaintainerFName, fmt.Sprintf("%s <%s>", maintainerName, maintainerEmail))
	(*ctrl)[0].Set(deb.DescriptionFName, fmt.Sprintf("%s\n%s", shortDescription, longDescription))
	(*ctrl)[0].Set(deb.ArchitectureFName, architecture)
	if len(*ctrl) < 2 {
		*ctrl = append(*ctrl, deb.NewPackage())
	}
	(*ctrl)[1].Set(deb.PackageFName, pkgName)
	(*ctrl)[1].Set(deb.DescriptionFName, fmt.Sprintf("%s\n%s", shortDescription, longDescription))
	(*ctrl)[1].Set(deb.ArchitectureFName, architecture)
	//-dev package. Optional somehow?
	if len(*ctrl) < 3 {
		*ctrl = append(*ctrl, deb.NewPackage())
	}
	(*ctrl)[2].Set(deb.PackageFName, pkgName+"-dev")
	(*ctrl)[2].Set(deb.ArchitectureFName, "all")
	(*ctrl)[2].Set(deb.DescriptionFName, fmt.Sprintf("%s - development package\n%s", shortDescription, longDescription))
	deb.SetDefaults(ctrl)
	if strings.HasPrefix(flavour, "go:") {
		debgen.ApplyGoDefaults(ctrl)
	} else {
		debgen.ApplyBasicDefaults(ctrl)
	}
	if err == nil {
		//validation ...
		err = deb.ValidateControl(ctrl)
		if err != nil {
			println("")
			fmt.Fprintf(os.Stderr, "Usage:\n")
			fs.PrintDefaults()
			println("")
		}
	}
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	err = build.Init()
	if err != nil {
		return fmt.Errorf("Error initialising build: %v", err)
	}

	spkg := deb.NewSourcePackage(ctrl)
	spgen := debgen.NewSourcePackageGenerator(spkg, build)
	if flavour == "go:exe" {
		spgen.ApplyDefaultsPureGo()
	}

	//create control file
	filename := filepath.Join(build.DebianDir, "control")
	_, err = os.Stat(filename)
	if os.IsNotExist(err) || overwrite {
		err = spgen.GenSourceControlFile()
		if err != nil {
			return fmt.Errorf("Error generating control file: %v", err)
		}
	} else if err != nil {
		return fmt.Errorf("Error generating control file: %v", err)
	} else {
		log.Printf("%s already exists", filename)
	}

	//changelog file
	filename = filepath.Join(build.DebianDir, "changelog")
	_, err = os.Stat(filename)
	if os.IsNotExist(err) || overwrite {
		changelogAddEntry(ctrl, build, entry)
	} else if err != nil {
		return fmt.Errorf("Error reading existing changelog: %v", err)
	} else {
		log.Printf("%s already exists", filename)
	}

	//copyright file
	filename = filepath.Join(build.DebianDir, "copyright")
	_, err = os.Stat(filename)
	if os.IsNotExist(err) || overwrite {
		err = copyrightInit(ctrl, build, license)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("Error reading existing copyright file: %v", err)
	} else {
		log.Printf("%s already exists", filename)
	}

	//rules file ...
	filename = filepath.Join(build.DebianDir, "rules")
	_, err = os.Stat(filename)
	if os.IsNotExist(err) || overwrite {
		templateVars := debgen.NewTemplateData(ctrl)
		tpl, err := template.New("template").Parse(spgen.TemplateStrings["rules"])
		if err != nil {
			return fmt.Errorf("Error parsing template: %v", err)
		}
		//create ..
		f, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("Error creating file: %v", err)
		}
		defer f.Close()
		err = tpl.Execute(f, templateVars)
		if err != nil {
			return fmt.Errorf("Error executing template: %v", err)
		}
		err = f.Close()
		if err != nil {
			return fmt.Errorf("Error closing written file: %v", err)
		}
	} else if err != nil {
		return fmt.Errorf("Error reading existing rules file: %v", err)
	} else {
		log.Printf("%s already exists", filename)
	}
	return nil
}
