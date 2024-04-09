package validator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	ensureStaticHasAuthorizedKeysFile,
	ensureValidExpansions,
	ensureStaticHostsAreNotSpawnable,
	ensureValidContainerPool,
	ensureValidArch,
	ensureValidBootstrapSettings,
	ensureValidStaticBootstrapSettings,
	ensureHasNoUnauthorizedCharacters,
	ensureHasValidHostAllocatorSettings,
	ensureHasValidPlannerSettings,
	ensureHasValidFinderSettings,
	ensureHasValidDispatcherSettings,
	ensureHasValidVirtualWorkstationSettings,
}

// CheckDistro checks if the distro configuration syntax is valid. Returns
// a slice of any validation errors found.
func CheckDistro(ctx context.Context, d *distro.Distro, s *evergreen.Settings, newDistro bool) (ValidationErrors, error) {
	validationErrs := ValidationErrors{}
	var allDistroIDs, allDistroAliases []string
	var err error
	if newDistro || len(d.Aliases) > 0 {
		allDistroIDs, allDistroAliases, err = getDistros(ctx)
		if err != nil {
			return nil, err
		}
	}

	if newDistro {
		validationErrs = append(validationErrs, ensureUniqueId(d, allDistroIDs)...)
	}

	validationErrs = append(validationErrs, validateAliases(d, allDistroAliases)...)

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
func ensureHasRequiredFields(ctx context.Context, d *distro.Distro, _ *evergreen.Settings) ValidationErrors {
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

	if d.Provider == "" {
		return append(errs, ValidationError{
			Message: fmt.Sprintf("distro '%v' cannot be blank", distro.ProviderKey),
			Level:   Error,
		})
	}
	if evergreen.IsEc2Provider(d.Provider) && len(d.ProviderSettingsList) > 1 {
		return append(errs, validateMultipleProviderSettings(d)...)
	} else if err := validateSingleProviderSettings(d); err != nil {
		errs = append(errs, ValidationError{
			Message: err.Error(),
			Level:   Error,
		})
	}
	return errs
}

func validateMultipleProviderSettings(d *distro.Distro) ValidationErrors {
	errs := ValidationErrors{}
	definedRegions := map[string]bool{}
	for _, doc := range d.ProviderSettingsList {
		region, ok := doc.Lookup("region").StringValueOK()
		if !ok {
			region = evergreen.DefaultEC2Region
		}
		if definedRegions[region] {
			errs = append(errs, ValidationError{
				Message: fmt.Sprintf("defined region %s more than once", region),
				Level:   Error,
			})
			continue
		}
		definedRegions[region] = true
		bytes, err := doc.MarshalBSON()
		if err != nil {
			errs = append(errs, ValidationError{
				Message: errors.Wrap(err, "error marshalling provider setting into bson").Error(),
				Level:   Error,
			})
			continue
		}

		settings := &cloud.EC2ProviderSettings{}
		if err := bson.Unmarshal(bytes, settings); err != nil {
			errs = append(errs, ValidationError{
				Message: errors.Wrap(err, "error unmarshalling bson into provider settings").Error(),
				Level:   Error,
			})
			continue
		}
		if err := settings.Validate(); err != nil {

			errs = append(errs, ValidationError{
				Message: errors.Wrapf(err, "error validating settings for region '%s'", region).Error(),
				Level:   Error,
			})
		}
	}
	return errs
}

func validateSingleProviderSettings(d *distro.Distro) error {
	settings, err := cloud.GetSettings(d.Provider)
	if err != nil {
		return errors.WithStack(err)
	}
	if err = settings.FromDistroSettings(*d, ""); err != nil {
		return errors.Wrapf(err, "distro '%v' decode error", distro.ProviderSettingsListKey)
	}

	if err := settings.Validate(); err != nil {
		return errors.Wrap(err, "error validating settings")
	}
	return nil
}

// ensureUniqueId checks that the distro's id does not collide with an existing id.
func ensureUniqueId(d *distro.Distro, distroIds []string) ValidationErrors {
	if utility.StringSliceContains(distroIds, d.Id) {
		return ValidationErrors{{Error, fmt.Sprintf("distro '%v' uses an existing identifier", d.Id)}}
	}
	return nil
}

func ensureValidAliases(d *distro.Distro) ValidationErrors {
	var errs ValidationErrors
	for _, a := range d.Aliases {
		if d.Id == a {
			errs = append(errs, ValidationError{
				Level:   Error,
				Message: fmt.Sprintf("'%s' cannot be an distro alias of itself", a),
			})
		}
	}
	return errs
}

func ensureNoAliases(d *distro.Distro, distroAliases []string) ValidationErrors {
	var errs ValidationErrors
	if len(d.Aliases) != 0 {
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("'%s' cannot have aliases", d.Id),
		})
	}
	if utility.StringSliceContains(distroAliases, d.Id) {
		errs = append(errs, ValidationError{
			Level:   Error,
			Message: fmt.Sprintf("cannot have alias that resolves to '%s'", d.Id),
		})
	}
	return errs
}

// ensureValidExpansions checks that no expansion option key is blank.
func ensureValidExpansions(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	for _, e := range d.Expansions {
		if e.Key == "" {
			return ValidationErrors{{Error, "distro cannot be blank expansion key"}}
		}
	}
	return nil
}

// ensureValidSSHOptions checks that no SSH option key is blank.
func ensureValidSSHOptions(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	for _, o := range d.SSHOptions {
		if o == "" {
			return ValidationErrors{{Error, "distro cannot be blank SSH option"}}
		}
	}
	return nil
}

// ensureStaticHasAuthorizedKeysFile checks that the SSH key name corresponds to an actual
// SSH key.
func ensureStaticHasAuthorizedKeysFile(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if len(s.SSHKeyPairs) != 0 && d.Provider == evergreen.ProviderNameStatic && d.AuthorizedKeysFile == "" {
		return ValidationErrors{
			{
				Message: "authorized keys file was not specified",
				Level:   Error,
			},
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
		err := distro.ValidateContainerPoolDistros(ctx, s)
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

	if !utility.StringSliceContains(evergreen.ValidHostAllocators, settings.Version) {
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
	if !utility.StringSliceContains(evergreen.ValidHostAllocatorRoundingRules, settings.RoundingRule) {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.rounding_rule value of %s for distro '%s' - its value must be %s", settings.RoundingRule, d.Id, evergreen.ValidHostAllocatorRoundingRules),
			Level:   Error,
		})
	}
	if !utility.StringSliceContains(evergreen.ValidHostAllocatorFeedbackRules, settings.FeedbackRule) {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.feedback_rule value of %s for distro '%s' - its value must be %s", settings.FeedbackRule, d.Id, evergreen.ValidHostAllocatorFeedbackRules),
			Level:   Error,
		})
	}
	if !utility.StringSliceContains(evergreen.ValidHostsOverallocatedRules, settings.HostsOverallocatedRule) {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.hosts_overallocated_rule value of %s for distro '%s' - its value must be %s", settings.FeedbackRule, d.Id, evergreen.ValidHostsOverallocatedRules),
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
	if settings.FutureHostFraction < 0 || settings.FutureHostFraction > 1 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid host_allocator_settings.future_host_fraction value of %f for distro '%s' - its value must be a fraction between 0 and 1, inclusive", settings.FutureHostFraction, d.Id),
			Level:   Error,
		})
	}

	return errs
}

// ensureHasValidPlannerSettings checks that the distro's PlannerSettings are valid
func ensureHasValidPlannerSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	errs := ValidationErrors{}
	settings := d.PlannerSettings

	if !utility.StringSliceContains(evergreen.ValidTaskPlannerVersions, settings.Version) {
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
	if settings.PatchTimeInQueueFactor < 0 || settings.PatchTimeInQueueFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.patch_time_in_queue_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.PatchTimeInQueueFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.CommitQueueFactor < 0 || settings.CommitQueueFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.commit_queue_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.CommitQueueFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.MainlineTimeInQueueFactor < 0 || settings.MainlineTimeInQueueFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.mainline_time_in_queue_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.MainlineTimeInQueueFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.ExpectedRuntimeFactor < 0 || settings.ExpectedRuntimeFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.expected_runtime_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.ExpectedRuntimeFactor, d.Id),
			Level:   Error,
		})
	}
	if settings.GenerateTaskFactor < 0 || settings.GenerateTaskFactor > 100 {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("invalid planner_settings.generate_task_factor value of %d for distro '%s' - its value must be a non-negative integer between 0 and 100, inclusive", settings.GenerateTaskFactor, d.Id),
			Level:   Error,
		})
	}

	return errs
}

// ensureHasValidFinderSettings checks that the distro's FinderSettings are valid
func ensureHasValidFinderSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if !utility.StringSliceContains(evergreen.ValidTaskFinderVersions, d.FinderSettings.Version) {
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
	if !utility.StringSliceContains(evergreen.ValidTaskDispatcherVersions, d.DispatcherSettings.Version) {
		return ValidationErrors{
			{
				Message: fmt.Sprintf("invalid dispatcher_settings.version '%s' for distro '%s'", d.DispatcherSettings.Version, d.Id),
				Level:   Error,
			},
		}
	}

	return nil
}

func ensureHasValidVirtualWorkstationSettings(ctx context.Context, d *distro.Distro, s *evergreen.Settings) ValidationErrors {
	if !d.IsVirtualWorkstation {
		return nil
	}
	var errs ValidationErrors
	if d.HomeVolumeSettings.FormatCommand == "" {
		errs = append(errs, ValidationError{
			Message: "missing format command",
			Level:   Error,
		})
	}
	linuxArchs := []string{
		evergreen.ArchLinuxPpc64le,
		evergreen.ArchLinuxS390x,
		evergreen.ArchLinuxArm64,
		evergreen.ArchLinuxAmd64,
	}

	if !utility.StringSliceContains(linuxArchs, d.Arch) {
		errs = append(errs, ValidationError{
			Message: "workstation distros must use Linux with a supported CPU architecture",
			Level:   Error,
		})
	}
	return errs
}

func validateAliases(d *distro.Distro, allDistroAliases []string) ValidationErrors {
	var validationErrs ValidationErrors
	// Parent and container distros do not support aliases.
	if d.ContainerPool != "" || d.Provider == evergreen.ProviderNameDocker {
		validationErrs = append(validationErrs, ensureNoAliases(d, allDistroAliases)...)
	}
	if len(d.Aliases) > 0 {
		validationErrs = append(validationErrs, ensureValidAliases(d)...)
	}
	return validationErrs
}
