package debgen_test

import (
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"log"
)

func Example_genSourcePackage() {
	ctrl := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)

	build := debgen.NewBuildParams()
	build.IsRmtemp = false
	debgen.ApplyGoDefaults(ctrl)
	spkg := deb.NewSourcePackage(ctrl)
	err := build.Init()
	if err != nil {
		log.Fatalf("Error initializing dirs: %v", err)
	}
	spgen := debgen.NewSourcePackageGenerator(spkg, build)
	spgen.ApplyDefaultsPureGo()
	sourcesDestinationDir := ctrl.Get(deb.SourceFName) + "_" + ctrl.Get(deb.VersionFName)
	sourceDir := ".."
	sourcesRelativeTo := debgen.GetGoPathElement(sourceDir)
	spgen.OrigFiles, err = debgen.GlobForSources(sourcesRelativeTo, sourceDir, debgen.GlobGoSources, sourcesDestinationDir, []string{build.TmpDir, build.DestDir})
	if err != nil {
		log.Fatalf("Error resolving sources: %v", err)
	}
	err = spgen.GenerateAllDefault()

	if err != nil {
		log.Fatalf("Error building source: %v", err)
	}

	// Output:
	//
}
