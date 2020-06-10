package model

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

// Golang represents a configuration for generating an evergreen configuration
// from a project that uses golang.
type Golang struct {
	GolangGeneralConfig `yaml:",inline"`
	// Packages define packages that should be tested. They can be either
	// explicitly defined via configuration or automatically discovered.
	Packages []GolangPackage `yaml:"packages,omitempty"`
	// Variants describe the mapping between packages and distros to run them
	// on.
	Variants []GolangVariant `yaml:"variants"`
}

// GolangGeneralConfig defines general top-level configuration for Golang.
type GolangGeneralConfig struct {
	// GeneralConfig defines generic top-level configuration.
	// WorkingDirectory is the absolute path to the base directory where the
	// GOPATH directory is located.
	// Environment requires that GOPATH and GOROOT be defined. If the working
	// directory is specified, GOPATH must be specified as a subdirectory of the
	// working directory.
	// DefaultTags are applied to all packages (discovered or explicitly
	// defined) unless explicitly excluded.
	GeneralConfig `yaml:",inline"`
	// RootPackage is the name of the root package for the project (e.g.
	// github.com/mongodb/jasper).
	RootPackage string `yaml:"root_package"`
}

func (ggc *GolangGeneralConfig) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(ggc.RootPackage == "", "must specify the import path of the root package of the project")
	catcher.Wrap(ggc.validateEnvVars(), "invalid environment variables")
	return catcher.Resolve()
}

func (ggc *GolangGeneralConfig) validateEnvVars() error {
	catcher := grip.NewBasicCatcher()
	for _, name := range []string{"GOPATH", "GOROOT"} {
		if val, ok := ggc.Environment[name]; ok && val != "" {
			ggc.Environment[name] = util.ConsistentFilepath(val)
			continue
		}
		if val := os.Getenv(name); val != "" {
			ggc.Environment[name] = util.ConsistentFilepath(val)
			continue
		}
		catcher.Errorf("environment variable '%s' must be explicitly defined or already present in the environment", name)
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	// According to the semantics of the generator's GOPATH, it must be relative
	// to the working directory (if specified).
	relGopath, err := ggc.RelGopath()
	if err != nil {
		catcher.Wrap(err, "converting GOPATH to relative path")
	} else {
		ggc.Environment["GOPATH"] = relGopath
	}

	return catcher.Resolve()
}

// AbsGopath converts the relative GOPATH in the environment into an absolute
// path based on the working directory.
func (ggc *GolangGeneralConfig) AbsGopath() (string, error) {
	gopath := util.ConsistentFilepath(ggc.Environment["GOPATH"])
	workingDir := util.ConsistentFilepath(ggc.WorkingDirectory)
	if workingDir != "" && !strings.HasPrefix(gopath, workingDir) {
		return util.ConsistentFilepath(workingDir, gopath), nil
	}
	if !filepath.IsAbs(gopath) {
		return "", errors.New("GOPATH is relative path, but needs to be absolute path")
	}
	return gopath, nil
}

// RelGopath returns the GOPATH in the environment relative to the working
// directory (if it is defined).
func (ggc *GolangGeneralConfig) RelGopath() (string, error) {
	gopath := util.ConsistentFilepath(ggc.Environment["GOPATH"])
	workingDir := util.ConsistentFilepath(ggc.WorkingDirectory)
	if workingDir != "" && strings.HasPrefix(gopath, workingDir) {
		relGopath, err := filepath.Rel(workingDir, gopath)
		if err != nil {
			return "", errors.Wrap(err, "making GOPATH relative")
		}
		return util.ConsistentFilepath(relGopath), nil
	}
	if filepath.IsAbs(gopath) {
		return "", errors.New("GOPATH is absolute path, but needs to be relative path")
	}
	return gopath, nil
}

// AbsProjectPath returns the absolute path to the project.
func (ggc *GolangGeneralConfig) AbsProjectPath() (string, error) {
	gopath, err := ggc.AbsGopath()
	if err != nil {
		return "", errors.Wrap(err, "getting GOPATH as an absolute path")
	}
	return util.ConsistentFilepath(gopath, "src", ggc.RootPackage), nil
}

// RelProjectPath returns the path to the project relative to the working
// directory.
func (ggc *GolangGeneralConfig) RelProjectPath() (string, error) {
	gopath, err := ggc.RelGopath()
	if err != nil {
		return "", errors.Wrap(err, "getting GOPATH as a relative path")
	}
	return util.ConsistentFilepath(gopath, "src", ggc.RootPackage), nil
}

// NewGolang returns a model of a Golang build configuration from a single file
// and working directory where the GOPATH directory is located.
func NewGolang(file, workingDir string) (*Golang, error) {
	gv := struct {
		Golang           `yaml:",inline"`
		VariablesSection `yaml:",inline"`
	}{}
	if err := utility.ReadYAMLFileStrict(file, &gv); err != nil {
		return nil, errors.Wrap(err, "unmarshalling from YAML file")
	}
	g := gv.Golang
	g.WorkingDirectory = workingDir

	if err := g.DiscoverPackages(); err != nil {
		return nil, errors.Wrap(err, "automatically discovering test packages")
	}

	g.ApplyDefaultTags()

	if err := g.Validate(); err != nil {
		return nil, errors.Wrap(err, "golang generator configuration")
	}

	return &g, nil
}

// ApplyDefaultTags applies all the default tags to the existing tasks, subject
// to package-level exclusion rules.
func (g *Golang) ApplyDefaultTags() {
	for _, tag := range g.DefaultTags {
		for i, gp := range g.Packages {
			if !utility.StringSliceContains(gp.Tags, tag) && !utility.StringSliceContains(gp.ExcludeTags, tag) {
				g.Packages[i].Tags = append(g.Packages[i].Tags, tag)
			}
		}
	}
}

// MergeTasks merges task definitions with the existing ones by either package
// name or package path. For a given package name or path, existing tasks are
// overwritten if they are already defined.
func (g *Golang) MergePackages(gps ...GolangPackage) {
	for _, gp := range gps {
		if gp.Name != "" {
			if _, i, err := g.GetPackageIndexByName(gp.Name); err == nil {
				g.Packages[i] = gp
			} else {
				g.Packages = append(g.Packages, gp)
			}
		} else if gp.Path != "" {
			if _, i, err := g.GetUnnamedPackageIndexByPath(gp.Path); err == nil {
				g.Packages[i] = gp
			} else {
				g.Packages = append(g.Packages, gp)
			}
		}
	}
}

// MergeVariants merges variants with the existing ones by variant name. For a
// given variant name, existing variants are overwritten if they are already
// defined.
func (g *Golang) MergeVariants(vs ...GolangVariant) {
	for _, v := range vs {
		if _, i, err := g.GetVariantIndexByName(v.Name); err == nil {
			g.Variants[i] = v
		} else {
			g.Variants = append(g.Variants, v)
		}
	}
}

// Validate checks that the entire Golang build configuration is valid.
func (g *Golang) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Wrap(g.GolangGeneralConfig.Validate(), "invalid top-level configuration")
	catcher.Wrap(g.validatePackages(), "invalid package definition(s)")
	catcher.Wrap(g.validateVariants(), "invalid variant definition(s)")

	return catcher.Resolve()
}

// validatePackages checks that:
// - Packages are defined.
// - Each package has a unique name. If it's unnamed, it must be the only
//   unnamed package with its path. Furthermore, no package can be named the
//   same as the path of an unnamed package.
// - Each package definition is valid.
func (g *Golang) validatePackages() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(g.Packages) == 0, "must have at least one package to test")
	pkgNames := map[string]struct{}{}
	pkgPaths := map[string]struct{}{}
	unnamedPkgPaths := map[string]struct{}{}
	for _, pkg := range g.Packages {
		catcher.Wrapf(pkg.Validate(), "invalid package definition for package named '%s' and path '%s'", pkg.Name, pkg.Path)

		if pkg.Name != "" {
			if _, ok := pkgNames[pkg.Name]; ok {
				catcher.Errorf("duplicate package named '%s'", pkg.Name)
			}
			pkgNames[pkg.Name] = struct{}{}
		} else {
			if _, ok := unnamedPkgPaths[pkg.Path]; ok {
				catcher.Errorf("duplicate unnamed package definitions for path '%s'", pkg.Path)
			}
			unnamedPkgPaths[pkg.Path] = struct{}{}
		}
		pkgPaths[pkg.Path] = struct{}{}
	}
	// Don't allow packages with a name that matches an unnamed package
	// containing a path.
	for pkgPath := range unnamedPkgPaths {
		if _, ok := pkgNames[pkgPath]; ok {
			catcher.Errorf("cannot have package named '%s' because it would be ambiguous with unnamed package with path '%s'", pkgPath, pkgPath)
		}
	}

	return catcher.Resolve()
}

// validateEnvVars checks that:
// - GOROOT is defined.
// - GOPATH is defined and can be converted to a path relative to the working
//   directory.
// validateVariants checks that:
// - Variants are defined.
// - Each variant name is unique.
// - Each variant definition is valid.
// - Each package referenced in a variant has a defined package.
// - Each variant does not specify a duplicate package.
func (g *Golang) validateVariants() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(g.Variants) == 0, "must specify at least one variant")
	varNames := map[string]struct{}{}
	for _, gv := range g.Variants {
		catcher.Wrapf(gv.Validate(), "invalid definition for variant '%s'", gv.Name)

		if _, ok := varNames[gv.Name]; ok {
			catcher.Errorf("cannot have duplicate variant name '%s'", gv.Name)
		}
		varNames[gv.Name] = struct{}{}

		pkgNames := map[string]struct{}{}
		pkgPaths := map[string]struct{}{}
		for _, gvp := range gv.Packages {
			pkgs, _, err := g.GetPackagesAndRef(gvp)
			if err != nil {
				catcher.Wrapf(err, "invalid package reference in variant '%s'", gv.Name)
				continue
			}
			for _, pkg := range pkgs {
				if pkg.Name != "" {
					if _, ok := pkgNames[pkg.Name]; ok {
						catcher.Errorf("duplicate reference to package name '%s' in variant '%s'", pkg.Name, gv.Name)
					}
					pkgNames[pkg.Name] = struct{}{}
				} else if pkg.Path != "" {
					if _, ok := pkgPaths[pkg.Path]; ok {
						catcher.Errorf("duplicate reference to package path '%s' in variant '%s'", pkg.Path, gv.Name)
					}
					pkgPaths[pkg.Path] = struct{}{}
				}
			}
		}
	}
	return catcher.Resolve()
}

const (
	// golangTestFileSuffix is the suffix indicating that a golang file is meant to
	// be run as a test.
	golangTestFileSuffix = "_test.go"
	// golangVendorDir is the special vendor directory for vendoring
	// dependencies.
	golangVendorDir = "vendor"
	// golangTestDataDir is the special data directory for tests.
	golangTestDataDir = "testdata"
)

// DiscoverPackages discovers directories containing tests in the local file
// system and adds them if they are not already defined.
func (g *Golang) DiscoverPackages() error {
	if err := g.validateEnvVars(); err != nil {
		return errors.Wrap(err, "invalid environment variables")
	}

	projectPath, err := g.AbsProjectPath()
	if err != nil {
		return errors.Wrap(err, "getting project path as an absolute path")
	}

	if err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		fileName := filepath.Base(info.Name())
		if fileName == golangVendorDir {
			return filepath.SkipDir
		}
		if fileName == golangTestDataDir {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if !strings.Contains(fileName, golangTestFileSuffix) {
			return nil
		}
		dir := filepath.Dir(path)
		dir, err = filepath.Rel(projectPath, dir)
		if err != nil {
			return errors.Wrapf(err, "making package path '%s' relative to root package", path)
		}
		dir = util.ConsistentFilepath(dir)
		// If package has already been defined, skip adding it.
		for _, gp := range g.Packages {
			if gp.Path == dir {
				return nil
			}
		}

		pkg := GolangPackage{
			Path: dir,
		}

		g.Packages = append(g.Packages, pkg)

		return nil
	}); err != nil {
		return errors.Wrapf(err, "walking the file system tree starting from path '%s'", projectPath)
	}

	return nil
}

// GetPackagesAndRef returns all packages that match the reference specified in
// the given GolangVariantPackage and the reference string from the
// GolangVariantPackage.
func (g *Golang) GetPackagesAndRef(gvp GolangVariantPackage) ([]GolangPackage, string, error) {
	if gvp.Tag != "" {
		pkgs := g.GetPackagesByTag(gvp.Tag)
		if len(pkgs) == 0 {
			return nil, "", errors.Errorf("no packages matched tag '%s'", gvp.Tag)
		}
		return pkgs, gvp.Tag, nil
	}

	if gvp.Name != "" {
		pkg, _, err := g.GetPackageIndexByName(gvp.Name)
		if err != nil {
			return nil, "", errors.Wrapf(err, "finding definition for package named '%s'", gvp.Name)
		}
		return []GolangPackage{*pkg}, gvp.Name, nil
	} else if gvp.Path != "" {
		pkg, _, err := g.GetUnnamedPackageIndexByPath(gvp.Path)
		if err != nil {
			return nil, "", errors.Wrapf(err, "finding definition for package path '%s'", gvp.Path)
		}
		return []GolangPackage{*pkg}, gvp.Path, nil
	}

	return nil, "", errors.New("empty package reference")
}

// GetPackageIndexByName finds the package by name and returns the task
// definition and its index.
func (g *Golang) GetPackageIndexByName(name string) (gp *GolangPackage, index int, err error) {
	for i, p := range g.Packages {
		if p.Name == name {
			return &p, i, nil
		}
	}
	return nil, -1, errors.Errorf("package with name '%s' not found", name)
}

// GetUnnamedPackageIndexByPath finds the unnamed package by path and returns
// the task definition and its index.
func (g *Golang) GetUnnamedPackageIndexByPath(path string) (gp *GolangPackage, index int, err error) {
	for i, p := range g.Packages {
		if p.Name == "" && p.Path == path {
			return &p, i, nil
		}
	}
	return nil, -1, errors.Errorf("unnamed package with path '%s' not found", path)
}

// GetPackagesByTag returns the packages that match the tag.
func (g *Golang) GetPackagesByTag(tag string) []GolangPackage {
	var pkgs []GolangPackage
	for _, pkg := range g.Packages {
		if utility.StringSliceContains(pkg.Tags, tag) {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// GetVariantIndexByName finds the variant by name and returns the variant
// definition and its index.
func (g *Golang) GetVariantIndexByName(name string) (gv *GolangVariant, index int, err error) {
	for i, v := range g.Variants {
		if v.Name == name {
			return &v, i, nil
		}
	}
	return nil, -1, errors.Errorf("variant with name '%s' not found", name)
}

type GolangPackage struct {
	// Name is an alias for the package.
	Name string `yaml:"name,omitempty"`
	// Path is the path of the package relative to the root package. For
	// example, "." refers to the root package while "util" refers to a
	// subpackage called "util" within the root package.
	Path string `yaml:"path"`
	// Tags are labels that allow you to logically group related packages.
	Tags []string `yaml:"tags,omitempty"`
	// ExcludeTags allows you to specify tags that should not be applied to the
	// task. This can be useful for excluding a package from the default tags.
	ExcludeTags []string `yaml:"exclude_tags,omitempty"`
	// Flags are package-specific Golang flags that modify runtime execution.
	Flags GolangFlags `yaml:"flags,omitempty"`
}

// Validate ensures that for each package:
// - It specifies a package path.
// - There are no duplicate tags.
// - The flags are valid.
func (gp *GolangPackage) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(gp.Path == "", "need to specify package path")
	tags := map[string]struct{}{}
	for _, tag := range gp.Tags {
		if _, ok := tags[tag]; ok {
			catcher.Errorf("duplicate tag '%s'", tag)
		}
		tags[tag] = struct{}{}
	}
	catcher.Wrap(gp.Flags.Validate(), "invalid flag(s)")
	return catcher.Resolve()
}

// GolangVariant defines a mapping between distros that run packages and the
// golang packages to run.
type GolangVariant struct {
	VariantDistro `yaml:",inline"`
	// Packages lists a package name, path or or tagged group of packages
	// relative to the root package.
	Packages []GolangVariantPackage `yaml:"packages"`
	// Golang are variant-specific flags that modify runtime execution.
	// Explicitly setting these values will override any flags specified under
	// the package definitions.
	Flags *GolangFlags `yaml:"flags,omitempty"`
}

// Validate checks that the variant-distro mapping and the Golang-specific
// parameters are valid.
func (gv *GolangVariant) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(gv.VariantDistro.Validate())
	catcher.Add(gv.validatePackages())

	if gv.Flags != nil {
		catcher.Wrap(gv.Flags.Validate(), "invalid flag(s)")
	}

	return catcher.Resolve()
}

func (gv *GolangVariant) validatePackages() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(gv.Packages) == 0, "need to specify at least one package")
	pkgPaths := map[string]struct{}{}
	pkgNames := map[string]struct{}{}
	pkgTags := map[string]struct{}{}
	for _, gvp := range gv.Packages {
		catcher.Wrap(gvp.Validate(), "invalid package reference")
		if path := gvp.Path; path != "" {
			if _, ok := pkgPaths[path]; ok {
				catcher.Errorf("duplicate reference to package path '%s'", path)
			}
			pkgPaths[path] = struct{}{}
		}
		if name := gvp.Name; name != "" {
			if _, ok := pkgNames[name]; ok {
				catcher.Errorf("duplicate reference to package name '%s'", name)
			}
			pkgNames[name] = struct{}{}
		}
		if tag := gvp.Tag; tag != "" {
			if _, ok := pkgTags[tag]; ok {
				catcher.Errorf("duplicate reference to package tag '%s'", tag)
			}
			pkgTags[tag] = struct{}{}

		}
	}
	return catcher.Resolve()
}

// GolangVariantPackage is a specifier that references a golang package.
type GolangVariantPackage struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path,omitempty"`
	Tag  string `yaml:"tag,omitempty"`
}

// Validate checks that exactly one kind of reference is specified in a package
// reference for a variant.
func (gvp *GolangVariantPackage) Validate() error {
	var numRefs int
	for _, ref := range []string{gvp.Name, gvp.Path, gvp.Tag} {
		if ref != "" {
			numRefs++
		}
	}
	if numRefs != 1 {
		return errors.New("must specify exactly one of the following: name, path, or tag")
	}
	return nil
}

// GolangFlags specify additional flags to the go binary to modify behavior of
// runtime execution.
type GolangFlags []string

// Validate ensures that flags to the go binary do not contain duplicate flags.
func (gf GolangFlags) Validate() error {
	seen := map[string]struct{}{}
	catcher := grip.NewBasicCatcher()
	for _, flag := range gf {
		flag = cleanupFlag(flag)
		// Don't allow the verbose flag because the scripting harness sets
		// verbose.
		if flagIsVerbose(flag) {
			catcher.New("verbose flag is always specified")
		}
		if _, ok := seen[flag]; ok {
			catcher.Errorf("duplicate flag '%s'", flag)
			continue
		}
		seen[flag] = struct{}{}
	}
	return catcher.Resolve()
}

// flagIsVerbose returns whether or not the flag is the golang flag for verbose
// testing.
func flagIsVerbose(flag string) bool {
	flag = cleanupFlag(flag)
	return flag == "v"
}

// golangTestPrefix is the optional prefix that each golang test flag can have.
// Flags with this prefix have identical meaning to their non-prefixed flag.
// (e.g. "test.v" and "v" are identical).
const golangTestPrefix = "test."

// cleanupFlag cleans up the golang-style flag and returns just the name of the
// flag. Golang flags have the form -<flag_name>[=value].
func cleanupFlag(flag string) string {
	flag = strings.TrimPrefix(flag, "-")
	flag = strings.TrimPrefix(flag, golangTestPrefix)

	// We only care about the flag name, not its set value.
	flagAndValue := strings.Split(flag, "=")
	flag = flagAndValue[0]

	return flag
}

// Merge returns the merged set of GolangFlags. Unique flags between
// the two flag sets are added together. Duplicate flags are handled by
// overwriting conflicting flags with overwrite's flags.
func (gf GolangFlags) Merge(overwrite GolangFlags) GolangFlags {
	merged := gf
	for _, flag := range overwrite {
		if i := merged.flagIndex(flag); i != -1 {
			merged = append(merged[:i], merged[i+1:]...)
			merged = append(merged, flag)
		} else {
			merged = append(merged, flag)
		}
	}

	return merged
}

// flagIndex returns the index where the flag is set if it is present. If it is
// absent, this returns -1.
func (gf GolangFlags) flagIndex(flag string) int {
	flag = cleanupFlag(flag)
	for i, f := range gf {
		f = cleanupFlag(f)
		if f == flag {
			return i
		}
	}
	return -1
}
