package main

import (
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"os"
	"path/filepath"
)

func debGen(input []string) error {
	build := debgen.NewBuildParams()
	fs := InitBuildFlags(cmdName+" "+TaskGenDeb, build)
/*
	fs.StringVar(&build.SourcesGlob, "sources-glob", "**.go", "Glob pattern for sources.")
	fs.StringVar(&build.SourceDir, "sources-dir", ".", "source dir")
	fs.StringVar(&build.SourcesRelativeTo, "sources-relative-to", "", "Sources relative to (it will assume relevant gopath element, unless you specify this)")
	fs.StringVar(&build.Bin386Glob, "bin-386", "*386/*", "Glob pattern for binaries for the 386 platform.")
	fs.StringVar(&build.BinArmhfGlob, "bin-armhf", "*armhf/*", "Glob pattern for binaries for the armhf platform.")
	fs.StringVar(&build.BinAmd64Glob, "bin-amd64", "*amd64/*", "Glob pattern for binaries for the amd64 platform.")
	fs.StringVar(&build.BinAnyGlob, "bin-any", "*any/*", "Glob pattern for binaries for *any* platform.")
*/
	//fs.StringVar(&build.sourcesDest, "sources-dest", debgen.DevGoPathDefault+"/src", "directory containing sources.")
	arches := ""
	fs.StringVar(&arches, "arch-filter", "", "Filter by Architecture. Comma-separated [386,armhf,amd64,all]")

//	fs.StringVar(&build.ResourcesGlob, "resources", "", "directory containing resources for this platform")
	fs.StringVar(&build.Version, "version", "", "Package version")
	err := fs.Parse(os.Args[2:])
	if err != nil {
		return fmt.Errorf("Error parsing flags: %v", err)
	}
	build.Arches, err = deb.ResolveArches(arches)
	if err != nil {
		return fmt.Errorf("Error resolving architecture: %v", err)
	}
	//Read control data
	fi, err := os.Open(filepath.Join(build.DebianDir, "control"))
	if err != nil {
		return fmt.Errorf("Error reading control data: %v", err)
	}
	cfr := deb.NewControlFileReader(fi)
	ctrl, err := cfr.Parse()
	if err != nil {
		return fmt.Errorf("Error reading control file: %v", err)
	}
	dgens, err := debgen.PrepareBasicDebGen(ctrl, build)
	if err != nil {
		return fmt.Errorf("Error preparing 'deb generators': %v", err)
	}
	for _, dgen := range dgens {
		err = dgen.GenerateAllDefault()
		if err != nil {
			return fmt.Errorf("Error building deb: %v", err)
		}
	}
	return nil
}
