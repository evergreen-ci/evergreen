package main

import (
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"os"
	"path/filepath"
	"text/template"
)

//Adds an entry to the changelog
func changelogAddEntryTask(args []string) error {
	build := debgen.NewBuildParams()
	fs := InitBuildFlags(cmdName+" "+TaskChangelogAdd, build)
	var version string
	fs.StringVar(&version, "version", "", "Package Version")
	var architecture string
	fs.StringVar(&architecture, "arch", "all", "Architectures [any,386,armhf,amd64,all]")
	var distribution string
	fs.StringVar(&distribution, "distribution", "unstable", "Distribution (unstable is recommended until Debian accept the package into testing/stable)")
	var entry string
	fs.StringVar(&entry, "entry", "", "Changelog entry data")

	err := fs.Parse(os.Args[2:])
	if err != nil {
		return fmt.Errorf("Error parsing %v", err)
	}
	if version == "" {
		return fmt.Errorf("Error: --version is a required flag")
	}
	if entry == "" {
		return fmt.Errorf("Error: --entry is a required flag")
	}
	ctrl, err := changelogPrepareCtrl(version, distribution, architecture, build)
	if err != nil {
		return err
	}
	return changelogAddEntry(ctrl, build, entry)
}

func changelogPrepareCtrl(version, distribution, architecture string, build *debgen.BuildParams) (*deb.Control, error) {
	controlFilename := filepath.Join(build.DebianDir, "control")
	fi, err := os.Open(controlFilename)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("file 'control' not found in debian-dir %s: %v", build.DebianDir, err)
	}
	defer fi.Close()
	cfr := deb.NewControlFileReader(fi)
	ctrl, err := cfr.Parse()
	if err != nil {
		return nil, fmt.Errorf("Control file parse error: %v", err)
	}
	if len(ctrl.SourceParas()) < 1 {
		return nil, fmt.Errorf("Control file does not have any 'Source' paragraphs")
	}
	ctrl.SourceParas()[0].Set("Version", version)
	ctrl.SourceParas()[0].Set("Distribution", distribution)
	return ctrl, nil
}

func changelogAddEntry(ctrl *deb.Control, build *debgen.BuildParams, entry string) error {
	filename := filepath.Join(build.DebianDir, "changelog")
	templateVars := debgen.NewTemplateData(ctrl)
	templateVars.ChangelogEntry = entry
	err := os.MkdirAll(filepath.Join(build.ResourcesDir, "debian"), 0777)
	if err != nil {
		return fmt.Errorf("Error making dirs: %v", err)
	}
	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		tpl, err := template.New("changelog-new").Parse(debgen.TemplateChangelogInitial)
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
		return fmt.Errorf("Error reading existing changelog: %v", err)
	} else {
		tpl, err := template.New("changelog-add").Parse(debgen.TemplateChangelogAdditionalEntry)
		if err != nil {
			return fmt.Errorf("Error parsing template: %v", err)
		}
		//append..
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("Error opening file: %v", err)
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
	}
	return nil
}
