package main

import (
	"flag"
	"github.com/debber/debber-v0.3/debgen"
)

func InitFlagsBasic(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return fs
}

func InitBuildFlags(name string, build *debgen.BuildParams) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&build.IsRmtemp, "rmtemp", false, "Remove 'temp' dirs")
	fs.BoolVar(&build.IsVerbose, "verbose", false, "Show log messages")
	fs.StringVar(&build.WorkingDir, "working-dir", build.WorkingDir, "Working directory")
	fs.StringVar(&build.TemplateDir, "template-dir", build.TemplateDir, "Template directory")
	fs.StringVar(&build.ResourcesDir, "resources-dir", build.ResourcesDir, "Resources directory")
	fs.StringVar(&build.DebianDir, "debian-dir", build.DebianDir, "'debian' dir (contains control file, changelog, postinst, etc)")
	return fs
}
