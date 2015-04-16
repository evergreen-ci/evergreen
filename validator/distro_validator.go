package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/cloud/providers/static"
	"10gen.com/mci/model/distro"
	_ "10gen.com/mci/plugin/config"
	"10gen.com/mci/util"
	"fmt"
	"github.com/mitchellh/mapstructure"
)

type distroValidator func(*distro.Distro, *mci.MCISettings) []ValidationError

// Functions used to validate the syntax of a distro object.
var distroSyntaxValidators = []distroValidator{
	ensureUniqueId,
	ensureHasRequiredFields,
	ensureValidSSHOptions,
	ensureValidExpansions,
}

// CheckDistro checks if the distro configuration syntax is valid. Returns
// a slice of any validation errors found.
func CheckDistro(d *distro.Distro, s *mci.MCISettings, newDistro bool) []ValidationError {
	// skip _id check for existing distro modifications.
	if newDistro {
		if err := populateDistroIds(); err != nil {
			return []ValidationError{*err}
		}
	} else {
		distroIds = []string{}
	}

	validationErrs := []ValidationError{}
	for _, v := range distroSyntaxValidators {
		validationErrs = append(validationErrs, v(d, s)...)
	}
	return validationErrs
}

// ensureHasRequiredFields check that the distro configuration has all the required fields
func ensureHasRequiredFields(d *distro.Distro, s *mci.MCISettings) []ValidationError {
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

	if d.SSHKey == "" && d.Provider != static.ProviderName {
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
func ensureUniqueId(d *distro.Distro, s *mci.MCISettings) []ValidationError {
	if util.SliceContains(distroIds, d.Id) {
		return []ValidationError{{Error, fmt.Sprintf("distro '%v' uses an existing identifier", d.Id)}}
	}
	return nil
}

// ensureValidExpansions checks that no expansion option key is blank.
func ensureValidExpansions(d *distro.Distro, s *mci.MCISettings) []ValidationError {
	for _, e := range d.Expansions {
		if e.Key == "" {
			return []ValidationError{{Error, fmt.Sprintf("distro cannot be blank expansion key")}}
		}
	}
	return nil
}

// ensureValidSSHOptions checks that no SSH option key is blank.
func ensureValidSSHOptions(d *distro.Distro, s *mci.MCISettings) []ValidationError {
	for _, o := range d.SSHOptions {
		if o == "" {
			return []ValidationError{{Error, fmt.Sprintf("distro cannot be blank SSH option")}}
		}
	}
	return nil
}
