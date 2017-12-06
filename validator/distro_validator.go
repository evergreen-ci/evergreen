package validator

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

type distroValidator func(*distro.Distro, *evergreen.Settings) []ValidationError

// Functions used to validate the syntax of a distro object.
var distroSyntaxValidators = []distroValidator{
	ensureHasRequiredFields,
	ensureValidSSHOptions,
	ensureValidExpansions,
	ensureStaticHostsAreNotSpawnable,
}

// CheckDistro checks if the distro configuration syntax is valid. Returns
// a slice of any validation errors found.
func CheckDistro(d *distro.Distro, s *evergreen.Settings, newDistro bool) ([]ValidationError, error) {
	validationErrs := []ValidationError{}
	distroIds := []string{}
	var err error
	if newDistro {
		// check ensureUniqueId separately and pass in distroIds list
		distroIds, err = getDistroIds()
		if err != nil {
			return nil, err
		}
	}
	validationErrs = append(validationErrs, ensureUniqueId(d, distroIds)...)

	for _, v := range distroSyntaxValidators {
		validationErrs = append(validationErrs, v(d, s)...)
	}
	return validationErrs, nil
}

// ensureStaticHostsAreNotSpawnable makes sure that any static distro cannot also be spawnable.
func ensureStaticHostsAreNotSpawnable(d *distro.Distro, s *evergreen.Settings) []ValidationError {
	if d.SpawnAllowed && d.Provider == evergreen.ProviderNameStatic {
		return []ValidationError{
			{
				Message: fmt.Sprintf("static distro %s cannot be spawnable", d.Id),
				Level:   Error,
			},
		}
	}

	return nil
}

// ensureHasRequiredFields check that the distro configuration has all the required fields
func ensureHasRequiredFields(d *distro.Distro, s *evergreen.Settings) []ValidationError {
	errs := []ValidationError{}

	if d.Id == "" {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.IdKey),
			Level:   Error,
		})
	}

	if d.Arch == "" {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.ArchKey),
			Level:   Error,
		})
	}

	if d.User == "" {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.UserKey),
			Level:   Error,
		})
	}

	if d.WorkDir == "" {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.WorkDirKey),
			Level:   Error,
		})
	}

	if d.SSHKey == "" && d.Provider != evergreen.ProviderNameStatic {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.SSHKeyKey),
			Level:   Error,
		})
	}

	if d.Provider == "" {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.ProviderKey),
			Level:   Error,
		})
		return errs
	}

	mgr, err := providers.GetCloudManager(d.Provider, s)
	if err != nil {
		errs = append(errs, ValidationError{
			Message: err.Error(),
			Level:   Error,
		})
		return errs
	}

	settings := mgr.GetSettings()

	if err = mapstructure.Decode(d.ProviderSettings, settings); err != nil {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' decode error: %v", distro.ProviderSettingsKey, err),
			Level:   Error,
		})
		return errs
	}

	if err := settings.Validate(); err != nil {
		errs = append(errs, ValidationError{Error, err.Error()})
	}

	return errs
}

// ensureUniqueId checks that the distro's id does not collide with an existing id.
func ensureUniqueId(d *distro.Distro, distroIds []string) []ValidationError {
	if util.StringSliceContains(distroIds, d.Id) {
		return []ValidationError{{Error, fmt.Sprintf("distro '%v' uses an existing identifier", d.Id)}}
	}
	return nil
}

// ensureValidExpansions checks that no expansion option key is blank.
func ensureValidExpansions(d *distro.Distro, s *evergreen.Settings) []ValidationError {
	for _, e := range d.Expansions {
		if e.Key == "" {
			return []ValidationError{{Error, fmt.Sprintf("distro cannot be blank expansion key")}}
		}
	}
	return nil
}

// ensureValidSSHOptions checks that no SSH option key is blank.
func ensureValidSSHOptions(d *distro.Distro, s *evergreen.Settings) []ValidationError {
	for _, o := range d.SSHOptions {
		if o == "" {
			return []ValidationError{{Error, fmt.Sprintf("distro cannot be blank SSH option")}}
		}
	}
	return nil
}
