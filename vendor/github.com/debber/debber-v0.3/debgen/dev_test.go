package debgen_test

import (
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"log"
)

func Example_genDevPackage() {
	ctrl := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)

	spara := ctrl.GetParasByField(deb.SourceFName, "testpkg")
	bpara := ctrl.GetParasByField(deb.PackageFName, "testpkg-dev")
	nctrl := &deb.Control{spara[0], bpara[0]}

	build := debgen.NewBuildParams()
	build.IsRmtemp = false
	build.Init()
	var err error
	mappedFiles, err := debgen.GlobForGoSources(".", []string{build.TmpDir, build.DestDir})
	if err != nil {
		log.Fatalf("Error building -dev: %v", err)
	}

	artifacts, err := deb.NewWriters(nctrl)
	if err != nil {
		log.Fatalf("Error building -dev: %v", err)
	}
	for _, artifact := range artifacts {
		dgen := debgen.NewDebGenerator(artifact, build)
		dgen.DataFiles = mappedFiles
		err = dgen.GenerateAllDefault()
		if err != nil {
			log.Fatalf("Error building -dev: %v", err)
		}
	}

	// Output:
	//
}
