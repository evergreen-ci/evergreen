package model

import (
	"path/filepath"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

// GolangControl represents a control file which can be used to build a Golang
// generator from multiple files containing the necessary build configuration.
type GolangControl struct {
	GeneralFile  string   `yaml:"general,omitempty"`
	VariantFiles []string `yaml:"variants"`
	PackageFiles []string `yaml:"packages,omitempty"`

	ControlDirectory string `yaml:"-"`
	WorkingDirectory string `yaml:"-"`
}

// NewGolangControl creates a new representation of a Golang control file from
// the given file. The working directory is the directory where the project
// will be cloned.
func NewGolangControl(file, workingDir string) (*GolangControl, error) {
	gcv := struct {
		GolangControl    `yaml:",inline"`
		VariablesSection `yaml:",inline"`
	}{}
	if err := utility.ReadYAMLFileStrict(file, &gcv); err != nil {
		return nil, errors.Wrap(err, "unmarshalling from YAML file")
	}
	gc := gcv.GolangControl
	gc.ControlDirectory = util.ConsistentFilepath(filepath.Dir(file))
	gc.WorkingDirectory = workingDir
	return &gc, nil
}

// Build creates a Golang model from the files referenced in the GolangControl.
func (gc *GolangControl) Build() (*Golang, error) {
	g := Golang{}

	gps, err := gc.buildPackages()
	if err != nil {
		return nil, errors.Wrap(err, "building package definitions")
	}
	g.MergePackages(gps...)

	gvs, err := gc.buildVariants()
	if err != nil {
		return nil, errors.Wrap(err, "building variant definitions")
	}
	g.MergeVariants(gvs...)

	ggc, err := gc.buildGeneral()
	if err != nil {
		return nil, errors.Wrap(err, "building top-level configuration")
	}
	g.GolangGeneralConfig = ggc

	g.WorkingDirectory = gc.WorkingDirectory

	if err := g.DiscoverPackages(); err != nil {
		return nil, errors.Wrap(err, "automatically discovering test packages")
	}

	g.ApplyDefaultTags()

	if err := g.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid build configuration")
	}

	return &g, nil
}

func (gc *GolangControl) buildPackages() ([]GolangPackage, error) {
	var all []GolangPackage
	if err := withMatchingFiles(gc.ControlDirectory, gc.PackageFiles, func(file string) error {
		gpsv := struct {
			Packages         []GolangPackage `yaml:"packages"`
			VariablesSection `yaml:",inline"`
		}{}
		if err := utility.ReadYAMLFileStrict(file, &gpsv); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}
		gps := gpsv.Packages

		catcher := grip.NewBasicCatcher()
		for _, gp := range gps {
			catcher.Wrapf(gp.Validate(), "package '%s'", gp.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid package definitions")
		}

		all = append(all, gps...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (gc *GolangControl) buildVariants() ([]GolangVariant, error) {
	var all []GolangVariant
	if err := withMatchingFiles(gc.ControlDirectory, gc.VariantFiles, func(file string) error {
		gvsv := struct {
			Variants         []GolangVariant `yaml:"variants"`
			VariablesSection `yaml:",inline"`
		}{}
		if err := utility.ReadYAMLFileStrict(file, &gvsv); err != nil {
			return errors.Wrap(err, "unmarshalling from YAML file")
		}
		gvs := gvsv.Variants

		catcher := grip.NewBasicCatcher()
		for _, gv := range gvs {
			catcher.Wrapf(gv.Validate(), "variant '%s'", gv.Name)
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "invalid variant definition(s)")
		}

		all = append(all, gvs...)

		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return all, nil
}

func (gc *GolangControl) buildGeneral() (GolangGeneralConfig, error) {
	ggcv := struct {
		GolangGeneralConfig `yaml:"general"`
		VariablesSection    `yaml:",inline"`
	}{}

	if err := utility.ReadYAMLFileStrict(gc.GeneralFile, &ggcv); err != nil {
		return GolangGeneralConfig{}, errors.Wrap(err, "unmarshalling from YAML file")
	}
	ggc := ggcv.GolangGeneralConfig
	if err := ggc.Validate(); err != nil {
		return GolangGeneralConfig{}, errors.Wrap(err, "invalid top-level configuration")
	}

	return ggc, nil
}
