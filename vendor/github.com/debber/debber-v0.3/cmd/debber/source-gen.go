package main

import (
	"fmt"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"os"
	"path/filepath"
)


func sourceGen(input []string) error {
	build := debgen.NewBuildParams()
	fs := InitBuildFlags(cmdName+" "+TaskGenSource, build)
	var sourceDir string
	fs.StringVar(&sourceDir, "sources", ".", "source dir")
	var isDiscoverGoPath bool
	fs.BoolVar(&isDiscoverGoPath, "discover-gopath", true, "Use Go-style path (try to discover base relative to GOPATH element)")
	var sourcesGlob string
	fs.StringVar(&sourcesGlob, "sources-glob", debgen.GlobGoSources, "Glob for inclusion of sources")
	var destinationDir string
	fs.StringVar(&destinationDir, "destination-dir", debgen.DevGoPathDefault, "Directory name to store sources in archive")
	var sourcesRelativeTo string
	fs.StringVar(&sourcesRelativeTo, "sources-relative-to", ".", "Sources relative to (this is ignored when -discover-gopath is selected)")
	fs.StringVar(&build.Version, "version", "", "Package version")

	// parse and validate flags
	err := fs.Parse(os.Args[2:])
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	if build.Version == "" {
		return fmt.Errorf("Error: --version is a required flag")
	}
	if isDiscoverGoPath {
		sourcesRelativeTo = debgen.GetGoPathElement(sourceDir)
	}
	mappedSources, err := debgen.GlobForSources(sourcesRelativeTo, sourceDir, sourcesGlob, destinationDir, []string{build.TmpDir, build.DebianDir})
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	fi, err := os.Open(filepath.Join(build.DebianDir, "control"))
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	cfr := deb.NewControlFileReader(fi)
	ctrl, err := cfr.Parse()
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	spgen, err := debgen.PrepareSourceDebGenerator(ctrl, build)
	for k, v := range mappedSources {
		spgen.OrigFiles[k] = v
	}
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	err = spgen.GenerateAllDefault()
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	return err
}

