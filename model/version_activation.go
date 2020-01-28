package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func DoProjectActivation(id string) error {
	// fetch the most recent, non-ignored version to activate
	activateVersion, err := VersionFindOne(VersionByMostRecentNonIgnored(id))
	if err != nil {
		return errors.WithStack(err)
	}
	if activateVersion == nil {
		grip.Info(message.Fields{
			"message":   "no version to activate for repository",
			"project":   id,
			"operation": "project-activation",
		})
		return nil
	}

	if err = ActivateElapsedBuilds(activateVersion); err != nil {
		return errors.WithStack(err)
	}

	return nil

}

// Activates any builds if their BatchTimes have elapsed.
func ActivateElapsedBuilds(v *Version) error {
	hasActivated := false
	now := time.Now()
	for i, status := range v.BuildVariants {
		// last comparison is to check that ActivateAt is actually set
		if !status.Activated && now.After(status.ActivateAt) && !status.ActivateAt.IsZero() {
			grip.Info(message.Fields{
				"message":   "activating revision",
				"variant":   status.BuildVariant,
				"project":   v.Branch,
				"revision":  v.Revision,
				"operation": "project-activation",
			})

			// Go copies the slice value, we want to modify the actual value
			status.Activated = true
			status.ActivateAt = now
			v.BuildVariants[i] = status

			b, err := build.FindOne(build.ById(status.BuildId))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "problem retrieving build",
					"variant":   status.BuildVariant,
					"build":     status.BuildId,
					"project":   v.Branch,
					"operation": "project-activation",
				}))
				continue
			}

			grip.Info(message.Fields{
				"message":   "activating build",
				"operation": "project-activation",
				"variant":   status.BuildVariant,
				"build":     status.BuildId,
				"project":   v.Branch,
			})

			// Don't need to set the version in here since we do it ourselves in a single update
			if err = SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator, false); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"operation": "project-activation",
					"message":   "problem activating build",
					"variant":   status.BuildVariant,
					"build":     status.BuildId,
					"project":   v.Branch,
				}))
				continue
			}
			hasActivated = true
		}
	}

	// If any variants were activated, update the stored version so that we don't
	// attempt to activate them again
	if hasActivated {
		return v.UpdateBuildVariants()
	}
	return nil

}
