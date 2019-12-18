package validator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	unauthorizedDistroCharacters = "|"
)

type distroValidator func(context.Context, *distro.Distro, *evergreen.Settings) ValidationErrors

// Functions used to validate the syntax of a distro object.
var distroSyntaxValidators = []distroValidator{
	ensureHasNonZeroID,
	ensureHasRequiredFields,
	ensureValidSSHOptions,
	ensureValidExpansions,
	ensureStaticHostsAreNotSpawnable,
	ensureValidContainerPool,
	ensureValidArch,
	ensureValidBootstrapSettings,
	ensureValidStaticBootstrapSettings,
	ensureValidCloneMethod,
	ensureHasNoUnauthorizedCharacters,
	ensureHasValidHostAllocatorSettings,
	ensureHasValidPlannerSettings,
	ensureHasValidFinderSettings,
	ensureHasValidDispatcherSettings,
}

// CheckDistro checks if the distro configuration syntax is valid. Returns
// a slice of any validation errors found.
func CheckDistro(ctx context.Context, d *distro.Distro, s *evergreen.Settings, newDistro bool) (ValidationErrors, error) {
	validationErrs := ValidationErrors{}
	distroIds := []string{}
	var err error
	if newDistro || len(d.Aliases) > 0 {
		distroIds, err = getDistroIds()
		if err != nil {
			return nil, err
		}
	}
	if newDistro {
		validationErrs = append(validationErrs, ensureUniqueId(d, distroIds)...)
	}
	if len(d.Aliases) > 0 {
		validationErrs = append(validationErrs, ensureValidAliases(d, distroIds)...)
	}

	for _, v := range distroSyntaxValidators {
		validationErrs = append(validationErrs, v(ctx, d, s)...)
	}
	return validationErrs, nil
}

// ensureStaticHostsAreNotSpawnable makes sure that any static distro cannot also be spawnable.
func ensureStaticHostsAreNotSpawnable(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if d.SpawnAllowed && d.Provider == evergreen.ProviderNameStatic {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("static distro %s cannot be spawnable", d.Id),
				Level:   Error,
			},
		}
	}

	return nil
}

// ensureHasRequiredFields check that the distro configuration has all the required fields
func ensureHasRequiredFields(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	errs := ValidationErrors{}

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
		return append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.ProviderKey),
			Level:   Error,
		})
	}

	mgrOpts, err := cloud.GetManagerOptions(*d)
	if err != nil {
		return append(errs, ValidationError{
			Message: err.Error(),
			Level:   Error,
		})
	}
	mgr, err := cloud.GetManager(ctx, evergreen.GetEnvironment(), mgrOpts)
	if err != nil {
		return append(errs, ValidationError{
			Message: err.Error(),
			Level:   Error,
		})
	}

	settings := mgr.GetSettings()

	if d.ProviderSettings != nil {
		if err = mapstructure.Decode(d.ProviderSettings, settings); err != nil {
			return append(errs, ValidationError{
				Message: fmt.Sprintf("distro '%v' decode error: %v", distro.ProviderSettingsKey, err),
				Level:   Error,
			})
		}
	}

	if err := settings.Validate(); err != nil {
		errs = append(errs, ValidationError{Error, err.Error()})
	}

	return errs
}

// ensureUniqueId checks that the distro's id does not collide with an existing id.
func ensureUniqueId(d *distro.Distro, distroIds []string) ValidationErrors {
	if util.StringSliceContains(distroIds, d.Id) {
		return ValidationErrors{{Error, fmt.Sprintf("distro '%v' uses an existing identifier", d.Id)}}
	}
	return nil
}

func ensureValidAliases(d *distro.Distro, distroIDs []string) ValidationErrors {
	errs := ValidationErrors{}

	for _, a := range d.Aliases {
		if d.Id == a {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("'%s' cannot be an distro alias of itself", a),
			})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ensureValidExpansions checks that no expansion option key is blank.
func ensureValidExpansions(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	for _, e := range d.Expansions {
		if e.Key == "" {
			return ValidationErrors{{Error, fmt.Sprintf("distro cannot be blank expansion key")}}
		}
	}
	return nil
}

// ensureValidSSHOptions checks that no SSH option key is blank.
func ensureValidSSHOptions(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	for _, o := range d.SSHOptions {
		if o == "" {
			return ValidationErrors{{Error, fmt.Sprintf("distro cannot be blank SSH option")}}
		}
	}
	return nil
}

// ensureValidArch checks that the architecture is one of the supported
// architectures.
func ensureValidArch(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if err := distro.ValidateArch(d.Arch); err != nil {
		return ValidationErrors{{Level: Error, Message: errors.Wrap(err, "error validating arch").Error()}}
	}
	return nil
}

// ensureValidBootstrapSettings checks that the bootstrap method
// is one of the supported methods, the communication method is one of the
// supported methods, and the two together form a valid combination.
func ensureValidBootstrapSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if err := d.ValidateBootstrapSettings(); err != nil {
		return ValidationErrors{{Level: Error, Message: err.Error()}}
	}
	return nil
}

// ensureValidBootstrapSettingsForStaticDistro checks that static hosts are
// bootstrapped with one of the allowed methods.
func ensureValidStaticBootstrapSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if d.Provider == evergreen.ProviderNameStatic && d.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("static distro %s cannot be bootstrapped with user data", d.Id),
				Level:   Error,
			},
		}
	}
	return nil
}

// ensureValidCloneMethod checks that the clone method is one of the supported
// methods.
func ensureValidCloneMethod(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if err := distro.ValidateCloneMethod(d.CloneMethod); err != nil {
		return ValidationErrors{{Level: Error, Message: err.Error()}}
	}
	return nil
}

func ensureHasNonZeroID(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if d == nil {
		return ValidationErrors{{Error, "distro cannot be nil"}}
	}

	if d.Id == "" {
		return ValidationErrors{{Error, "distro must specify id"}}
	}

	return nil
}

// ensureHasNoUnauthorizedCharacters checks that the distro name does not contain any unauthorized character.
func ensureHasNoUnauthorizedCharacters(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if strings.ContainsAny(d.Id, unauthorizedDistroCharacters) {
		message := fmt.Sprintf("distro '%v' contains unauthorized characters (%v)", d.Id, unauthorizedDistroCharacters)
		return ValidationErrors{{Error, message}}
	}
	return nil
}

// ensureValidContainerPool checks that a distro's container pool exists and
// has a valid distro capable of hosting containers
func ensureValidContainerPool(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if d.ContainerPool != "" {
		// check if container pool exists
		pool := s.ContainerPools.GetContainerPool(d.ContainerPool)
		if pool == nil {
			return ValidationErrors{{Error, "distro container pool does not exist"}}
		}
		// warn if container pool exists without valid distro
		err := distro.ValidateContainerPoolDistros(s)
		if err != nil {
			return ValidationErrors{{Error, "error in container pool settings: " + err.Error()}}
		}
	}
	return nil
}

// ensureHasValidHostAllocatorSettings checks that the distro's HostAllocatorSettings are valid
func ensureHasValidHostAllocatorSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	errs := ValidationErrors{}
	settings := d.HostAllocatorSettings

	if !util.StringSliceContains(evergreen.ValidHostAllocators, settings.Version) {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.version '%s' for distro '%s'", settings.Version, d.Id),
			Level:   Error,
		})
	}
	if settings.MinimumHosts < 0 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.minimum_hosts value of %d for distro '%s' - its value must be a non-negative integer", settings.MinimumHosts, d.Id),
			Level:   Error,
		})
	}
	if settings.MaximumHosts < 0 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.maximum_hosts value of %d for distro '%s' - its value must be a non-negative integer", settings.MaximumHosts, d.Id),
			Level:   Error,
		})
	}
	if settings.AcceptableHostIdleTime < 0 {
		ms := settings.AcceptableHostIdleTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.acceptable_host_idle_time value of %dms for distro '%s' - its value must be a non-negative integer", ms, d.Id),
			Level:   Error,
		})
	} else if settings.AcceptableHostIdleTime != 0 && (settings.AcceptableHostIdleTime < time.Second) {
		ms := settings.AcceptableHostIdleTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.acceptable_host_idle_time value of %dms for distro '%s' - its millisecond value must convert directly to units of seconds", ms, d.Id),
			Level:   Error,
		})
	} else if settings.AcceptableHostIdleTime%time.Second != 0 {
		ms := settings.AcceptableHostIdleTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.acceptable_host_idle_time value of %dms for distro '%s' - its millisecond value must convert directly to units of seconds", ms, d.Id),
			Level:   Error,
		})
	}

	return errs
}

// ensureHasValidPlannerSettings checks that the distro's PlannerSettings are valid
func ensureHasValidPlannerSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	errs := ValidationErrors{}
	settings := d.PlannerSettings

	if !util.StringSliceContains(evergreen.ValidTaskPlannerVersions, settings.Version) {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.version '%s' for distro '%s'", settings.Version, d.Id),
			Level:   Error,
		})
	}
	if settings.TargetTime < 0 {
		ms := settings.TargetTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.target_time value of %dms for distro '%s' - its value must be a non-negative integer", ms, d.Id),
			Level:   Error,
		})
	} else if settings.TargetTime != 0 && (settings.TargetTime < time.Second) {
		ms := settings.TargetTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.target_time value of %dms for distro '%s' - its millisecond value must convert directly to units of seconds", ms, d.Id),
			Level:   Error,
		})
	} else if settings.TargetTime%time.Second != 0 {
		ms := settings.TargetTime / time.Millisecond
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.target_time value of %dms for distro '%s' - its value must convert directly to units of seconds", ms, d.Id),
			Level:   Error,
		})
	}
	if settings.PatchFactor < 0 || settings.PatchFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.patch_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.PatchFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.TimeInQueueFactor < 0 || settings.TimeInQueueFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.time_in_queue_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.TimeInQueueFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.ExpectedRuntimeFactor < 0 || settings.ExpectedRuntimeFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.expected_runtime_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.ExpectedRuntimeFactor, d.Id),
			Level:   Error,
		})
	}

	return errs
}

// ensureHasValidFinderSettings checks that the distro's FinderSettings are valid
func ensureHasValidFinderSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if !util.StringSliceContains(evergreen.ValidTaskFinderVersions, d.FinderSettings.Version) {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("invalid finder_settings.version '%s' for distro '%s'", d.FinderSettings.Version, d.Id),
				Level:   Error,
			},
		}
	}

	return nil
}

// ensureHasValidDispatcherSettings checks that the distro's DispatcherSettings are valid
func ensureHasValidDispatcherSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if !util.StringSliceContains(evergreen.ValidTaskDispatcherVersions, d.DispatcherSettings.Version) {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("invalid dispatcher_settings.version '%s' for distro '%s'", d.DispatcherSettings.Version, d.Id),
				Level:   Error,
			},
		}
	}

	return nil
}
