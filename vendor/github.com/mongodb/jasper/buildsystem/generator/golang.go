package generator

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/shrub"
	"github.com/evergreen-ci/utility"

	// "github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Golang represents a configuration for generating an evergreen configuration
// from a project that uses golang.
type Golang struct {
	// Environment defines any environment variables. GOPATH and GOROOT are
	// required. If the working directory is specified, GOPATH must be specified
	// as a subdirectory of the working directory.
	Environment map[string]string `yaml:"environment"`
	// RootPackage is the name of the root package for the project (e.g.
	// github.com/mongodb/jasper).
	RootPackage string `yaml:"root_package"`
	// Packages explicitly sets options for packages that should be tested.
	Packages []GolangPackage `yaml:"packages,omitempty"`
	// Variants describe the mapping between packages and distros to run them
	// on.
	Variants []GolangVariant `yaml:"variants"`

	// WorkingDirectory is the absolute path to the  base directory where config
	// generation should be performed.
	WorkingDirectory string `yaml:"-"`
}

func NewGolang(file, workingDir string) (*Golang, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrap(err, "reading configuration file")
	}

	g := Golang{
		WorkingDirectory: workingDir,
	}
	if err := yaml.UnmarshalStrict(b, &g); err != nil {
		return nil, errors.Wrap(err, "unmarshalling configuration file from YAML")
	}

	if err := g.DiscoverPackages(); err != nil {
		return nil, errors.Wrap(err, "automatically discovering test packages")
	}

	if err := g.Validate(); err != nil {
		return nil, errors.Wrap(err, "golang generator configuration")
	}

	return &g, nil
}

func (g *Golang) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(g.RootPackage == "", "must specify the import path of the root package of the project")
	catcher.Wrap(g.validatePackages(), "invalid package definitions")
	catcher.Wrap(g.validateEnvVars(), "invalid environment variables")
	catcher.Wrap(g.validateVariants(), "invalid variant definitions")

	return catcher.Resolve()
}

func (g *Golang) validatePackages() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(g.Packages) == 0, "must have at least one package to test")
	pkgNames := map[string]struct{}{}
	unnamedPkgPaths := map[string]struct{}{}
	for _, pkg := range g.Packages {
		catcher.Wrapf(pkg.Validate(), "invalid package definition '%s'", pkg.Path)

		if pkg.Name == "" {
			if _, ok := unnamedPkgPaths[pkg.Path]; ok {
				catcher.Errorf("cannot have duplicate unnamed package definitions for path '%s'", pkg.Path)
			}
			unnamedPkgPaths[pkg.Path] = struct{}{}
			continue
		}
		if _, ok := pkgNames[pkg.Name]; ok {
			catcher.Errorf("cannot have duplicate package named '%s'", pkg.Name)
		}
		pkgNames[pkg.Name] = struct{}{}
	}
	return catcher.Resolve()
}

func (g *Golang) validateEnvVars() error {
	catcher := grip.NewBasicCatcher()
	for _, name := range []string{"GOPATH", "GOROOT"} {
		if val, ok := g.Environment[name]; ok && val != "" {
			continue
		}
		if val := os.Getenv(name); val != "" {
			g.Environment[name] = val
			continue
		}
		catcher.Errorf("environment variable '%s' must be explicitly defined or already present in the environment", name)
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	// According to the semantics of the generator's GOPATH, it must be relative
	// to the working directory (if specified).
	relGopath, err := g.relGopath()
	if err != nil {
		catcher.Wrap(err, "converting GOPATH to relative path")
	} else {
		g.Environment["GOPATH"] = relGopath
	}

	return catcher.Resolve()
}

// relGopath returns the GOPATH in the environment relative to the working
// directory (if it is defined).
func (g *Golang) relGopath() (string, error) {
	gopath := g.Environment["GOPATH"]
	if g.WorkingDirectory != "" && strings.HasPrefix(gopath, g.WorkingDirectory) {
		return filepath.Rel(g.WorkingDirectory, gopath)
	}
	if filepath.IsAbs(gopath) {
		return "", errors.New("GOPATH is absolute path, but needs to be relative path")
	}
	return gopath, nil
}

// absGopath converts the relative GOPATH in the environment into an absolute
// path based on the working directory.
func (g *Golang) absGopath() (string, error) {
	gopath := g.Environment["GOPATH"]
	if g.WorkingDirectory != "" && !strings.HasPrefix(gopath, g.WorkingDirectory) {
		return filepath.Join(g.WorkingDirectory, gopath), nil
	}
	if !filepath.IsAbs(gopath) {
		return "", errors.New("GOPATH is relative path, but needs to be absolute path")
	}
	return gopath, nil
}

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

		for _, vp := range gv.Packages {
			catcher.Wrapf(g.validatePackageRef(vp), "invalid package reference(s) in variant '%s'", gv.Name)
		}
	}
	return catcher.Resolve()
}

func (g *Golang) validatePackageRef(vp VariantPackage) error {
	catcher := grip.NewBasicCatcher()
	if vp.Name != "" {
		if _, err := g.GetPackageByName(vp.Name); err != nil {
			catcher.Errorf("package named '%s' is undefined", vp.Name)
		}
	}
	if vp.Path != "" {
		if _, err := g.GetPackageByPath(vp.Path); err != nil {
			catcher.Errorf("package with path '%s' is undefined", vp.Path)
		}
	}
	if vp.Tag != "" {
		if pkgs := g.GetPackagesByTag(vp.Tag); len(pkgs) == 0 {
			catcher.Errorf("no packages matched tag '%s'", vp.Tag)
		}
	}
	return catcher.Resolve()
}

// golangTestFileSuffix is the suffix indicating that a golang file is meant to
// be run as a test.
const (
	golangTestFileSuffix = "_test.go"
	golangVendorDir      = "vendor"
)

// DiscoverPackages discovers directories containing tests in the local file
// system and adds them if they are not already defined.
func (g *Golang) DiscoverPackages() error {
	if err := g.validateEnvVars(); err != nil {
		return errors.Wrap(err, "invalid environment variables")
	}

	projectPath, err := g.absProjectPath()
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
		// If package has already been added, skip adding it.
		if _, err = g.GetPackageByPath(dir); err == nil {
			return nil
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

const (
	// minTasksForTaskGroup is the minimum number of tasks that have to be in a
	// task group in order for it to be worthwhile to create a task group. Since
	// max hosts must can be at most half the number of tasks and we don't want
	// to use single-host task groups, we must have at least four tasks in the
	// group to make a multi-host task group.
	minTasksForTaskGroup = 4
)

// Generate creates the evergreen configuration from the given golang build
// configuration.
func (g *Golang) Generate() (*shrub.Configuration, error) {
	conf, err := shrub.BuildConfiguration(func(c *shrub.Configuration) {
		for _, gv := range g.Variants {
			variant := c.Variant(gv.Name)
			variant.DistroRunOn = gv.Distros

			var tasksForVariant []*shrub.Task
			// Make one task per package in this variant. We cannot make one
			// task per package, because we have to account for variant-level
			// options possibly overriding package-level options, which requires
			// making separate tasks with different commands.
			for _, vp := range gv.Packages {
				var pkgs []GolangPackage
				var pkgRef string
				pkgs, pkgRef, err := g.getPackagesAndRef(vp)
				if err != nil {
					panic(errors.Wrapf(err, "package definition for variant '%s'", gv.Name))
				}

				newTasks, err := g.generateVariantTasksForPackageRef(c, gv, pkgs, pkgRef)
				if err != nil {
					panic(errors.Wrapf(err, "generating task for package ref '%s' in variant '%s'", pkgRef, gv.Name))
				}
				tasksForVariant = append(tasksForVariant, newTasks...)
			}

			projectPath, err := g.relProjectPath()
			if err != nil {
				panic(errors.Wrap(err, "getting project path as a relative path"))
			}
			getProjectCmd := shrub.CmdGetProject{
				Directory: projectPath,
			}

			// Only use a task group for this variant if it meets the threshold
			// number of tasks. Otherwise, just run regular tasks for this
			// variant.
			if len(variant.TaskSpecs) >= minTasksForTaskGroup {
				tg := c.TaskGroup(gv.Name + "_group").SetMaxHosts(len(variant.TaskSpecs) / 2)
				tg.SetupTask = shrub.CommandSequence{getProjectCmd.Resolve()}

				for _, task := range variant.TaskSpecs {
					tg.Task(task.Name)
				}
				_ = variant.AddTasks(tg.GroupName)
			} else {
				for _, task := range tasksForVariant {
					task.Commands = append([]*shrub.CommandDefinition{getProjectCmd.Resolve()}, task.Commands...)
					_ = variant.AddTasks(task.Name)
				}
			}
		}
	})

	if err != nil {
		return nil, errors.Wrap(err, "generating evergreen configuration")
	}

	return conf, nil
}

func (g *Golang) getPackagesAndRef(vp VariantPackage) (pkgs []GolangPackage, pkgRef string, err error) {
	if vp.Tag != "" {
		pkgs = g.GetPackagesByTag(vp.Tag)
		if len(pkgs) == 0 {
			return nil, "", errors.Errorf("no packages matched tag '%s'", vp.Tag)
		}
		return pkgs, vp.Tag, nil
	}

	var pkg *GolangPackage
	if vp.Name != "" {
		pkg, err = g.GetPackageByName(vp.Name)
		if err != nil {
			return nil, "", errors.Wrapf(err, "finding definition for package named '%s'", vp.Name)
		}
		pkgRef = vp.Name
	} else if vp.Path != "" {
		pkg, err = g.GetPackageByPath(vp.Path)
		if err != nil {
			return nil, "", errors.Wrapf(err, "finding definition for package path '%s'", vp.Path)
		}
		pkgRef = vp.Path
	} else {
		return nil, "", errors.New("empty package reference")
	}

	return []GolangPackage{*pkg}, pkgRef, nil
}

func (g *Golang) generateVariantTasksForPackageRef(c *shrub.Configuration, gv GolangVariant, pkgs []GolangPackage, pkgRef string) ([]*shrub.Task, error) {
	var tasks []*shrub.Task

	for _, pkg := range pkgs {
		scriptCmd, err := g.subprocessScriptingCmd(gv, pkg)
		if err != nil {
			return nil, errors.Wrapf(err, "generating %s command for package '%s' in variant '%s'", shrub.CmdSubprocessScripting{}.Name(), pkg.Path, gv.Name)
		}
		var taskName string
		if len(pkgs) > 1 {
			taskName = getTaskName(gv.Name, pkgRef, pkg.Path)
		} else {
			taskName = getTaskName(gv.Name, pkgRef)
		}
		tasks = append(tasks, c.Task(taskName).Command(scriptCmd))
	}

	return tasks, nil
}

func (g *Golang) subprocessScriptingCmd(gv GolangVariant, pkg GolangPackage) (*shrub.CmdSubprocessScripting, error) {
	gopath, err := g.relGopath()
	if err != nil {
		return nil, errors.Wrap(err, "getting GOPATH as a relative path")
	}
	projectPath, err := g.relProjectPath()
	if err != nil {
		return nil, errors.Wrap(err, "getting project path as a relative path")
	}

	testOpts := pkg.Options
	if gv.Options != nil {
		testOpts = testOpts.Merge(*gv.Options)
	}

	relPath := pkg.Path
	if relPath != "." && !strings.HasPrefix(relPath, "./") {
		relPath = "./" + relPath
	}
	testOpts = append(testOpts, relPath)

	return &shrub.CmdSubprocessScripting{
		Harness:     "golang",
		WorkingDir:  g.WorkingDirectory,
		HarnessPath: gopath,
		// It is not a problem for the environment to set the
		// GOPATH here (relative to the working directory),
		// which conflicts with the actual GOPATH (an absolute
		// path). The GOPATH in the environment will be
		// overridden when subprocess.scripting runs to be an
		// absolute path relative to the working directory.
		Env:     g.Environment,
		TestDir: projectPath,
		TestOptions: &shrub.ScriptingTestOptions{
			Args: testOpts,
		},
	}, nil
}

func getTaskName(parts ...string) string {
	return strings.Join(parts, "-")
}

func (g *Golang) relProjectPath() (string, error) {
	gopath, err := g.relGopath()
	if err != nil {
		return "", errors.Wrap(err, "getting GOPATH as a relative path")
	}
	return filepath.Join(gopath, "src", g.RootPackage), nil
}

func (g *Golang) absProjectPath() (string, error) {
	gopath, err := g.absGopath()
	if err != nil {
		return "", errors.Wrap(err, "getting GOPATH as an absolute path")
	}
	return filepath.Join(gopath, "src", g.RootPackage), nil
}

// writeConfig writes the evergreen configuration to the given output as JSON.
// TODO: probably will be moved to a different package since we don't write the
// config JSON here.
//nolint: deadcode
func writeConfig(conf *shrub.Configuration, output io.Writer) error {
	data, err := json.Marshal(conf)
	if err != nil {
		return errors.Wrap(err, "marshalling configuration as JSON")
	}

	if _, err = io.Copy(output, bytes.NewBuffer(data)); err != nil {
		return errors.Wrapf(err, "writing configuration to output")
	}

	return nil
}

func (g *Golang) GetPackageByName(name string) (*GolangPackage, error) {
	for _, pkg := range g.Packages {
		if pkg.Name == name {
			return &pkg, nil
		}
	}
	return nil, errors.Errorf("package with name '%s' not found", name)
}

func (g *Golang) GetPackageByPath(path string) (*GolangPackage, error) {
	for _, pkg := range g.Packages {
		if pkg.Path == path {
			return &pkg, nil
		}
	}
	return nil, errors.Errorf("package with path '%s' not found", path)
}

func (g *Golang) GetPackagesByTag(tag string) []GolangPackage {
	var pkgs []GolangPackage
	for _, pkg := range g.Packages {
		if utility.StringSliceContains(pkg.Tags, tag) {
			pkgs = append(pkgs, pkg)
			continue
		}
	}
	return pkgs
}

type GolangPackage struct {
	// Name is an alias for the package.
	Name string `yaml:"name,omitempty"`
	// Path is the path of the package relative to the root package. For
	// example, "." refers to the root package while "util" refers to a
	// subpackage called "util" within the root package.
	Path string `yaml:"path"`
	// Tags are labels that allow you to logically group related packages.
	Tags    []string             `yaml:"tags,omitempty"`
	Options GolangRuntimeOptions `yaml:"options,omitempty"`
}

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
	catcher.Wrap(gp.Options.Validate(), "invalid golang options")
	return catcher.Resolve()
}

// GolangVariant defines a mapping between distros that run packages and the
// golang packages to run.
type GolangVariant struct {
	Name    string   `yaml:"name"`
	Distros []string `yaml:"distros"`
	// Packages lists a package name, path or or tagged group of packages
	// relative to the root package.
	Packages []VariantPackage `yaml:"packages"`
	// Options are variant-specific options that modify test execution.
	// Explicitly setting these values will override options specified under the
	// definitions of packages.
	Options *GolangRuntimeOptions `yaml:"options,omitempty"`
}

func (gv *GolangVariant) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(gv.Name == "", "missing variant name")
	catcher.NewWhen(len(gv.Distros) == 0, "need to specify at least one distro")
	catcher.NewWhen(len(gv.Packages) == 0, "need to specify at least one package")
	pkgPaths := map[string]struct{}{}
	pkgNames := map[string]struct{}{}
	pkgTags := map[string]struct{}{}
	for _, pkg := range gv.Packages {
		catcher.Wrap(pkg.Validate(), "invalid package reference")
		if path := pkg.Path; path != "" {
			if _, ok := pkgPaths[path]; ok {
				catcher.Errorf("duplicate reference to package path '%s'", path)
			}
			pkgPaths[path] = struct{}{}
		}
		if name := pkg.Name; name != "" {
			if _, ok := pkgNames[name]; ok {
				catcher.Errorf("duplicate reference to package name '%s'", name)
			}
			pkgNames[name] = struct{}{}
		}
		if tag := pkg.Tag; tag != "" {
			if _, ok := pkgTags[tag]; ok {
				catcher.Errorf("duplicate reference to package tag '%s'", tag)
			}
			pkgTags[tag] = struct{}{}
		}
	}
	if gv.Options != nil {
		catcher.Wrap(gv.Options.Validate(), "invalid runtime options")
	}
	return catcher.Resolve()
}

type VariantPackage struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path,omitempty"`
	Tag  string `yaml:"tag,omitempty"`
}

func (vp *VariantPackage) Validate() error {
	var numRefs int
	for _, ref := range []string{vp.Name, vp.Path, vp.Tag} {
		if ref != "" {
			numRefs++
		}
	}
	if numRefs != 1 {
		return errors.New("must specify exactly one of the following: name, path, or tag")
	}
	return nil
}

// GolangRuntimeOptions specify additional options to modify behavior of runtime
// execution.
type GolangRuntimeOptions []string

// Validate ensures that options to the scripting environment (i.e. the go
// binary) do not contain duplicate flags.
func (gro GolangRuntimeOptions) Validate() error {
	seen := map[string]struct{}{}
	catcher := grip.NewBasicCatcher()
	for _, flag := range gro {
		flag = cleanupFlag(flag)
		// Don't allow the verbose flag because the scripting harness sets
		// verbose.
		if flagIsVerbose(flag) {
			catcher.New("verbose flag is already specified")
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

// golangTestPrefix is the optiona prefix that each golang test flag can have.
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

// Merge returns the merged set of GolangRuntimeOptions. Unique flags between
// the two flag sets are added together. Duplicate flags are handled by
// overwriting conflicting flags with overwrite's flags.
func (gro GolangRuntimeOptions) Merge(overwrite GolangRuntimeOptions) GolangRuntimeOptions {
	merged := gro
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
func (gro GolangRuntimeOptions) flagIndex(flag string) int {
	flag = cleanupFlag(flag)
	for i, f := range gro {
		f = cleanupFlag(f)
		if f == flag {
			return i
		}
	}
	return -1
}
