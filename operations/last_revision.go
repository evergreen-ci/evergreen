package operations

import (
	"context"
	"regexp"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const defaultLastRevisionLookbackLimit = 50

func LastRevision() cli.Command {
	return cli.Command{
		Name:   "last-revision",
		Usage:  "return the latest revision that shouldApply a set of criteria",
		Flags:  addProjectFlag(addVariantsRegexpFlag()...),
		Before: mergeBeforeFuncs(autoUpdateCLI, requireVariantsFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			criteria, err := newLastRevisionCriteria(c.String(projectFlagName), c.StringSlice(variantsFlagName))
			if err != nil {
				return errors.Wrap(err, "building last revision options")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// kim: TODO: get version and builds, then make a skeleton similar
			// to what git-co-evg-base does.

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			// GET /projects/{project_id}/versions
			latestVersions, err := client.GetRecentVersionsForProject(ctx, c.String(projectFlagName), evergreen.RepotrackerVersionRequester, defaultLastRevisionLookbackLimit)
			if err != nil {
				return errors.Wrap(err, "getting latest versions for project")
			}

			// GET /versions/{version_id}/builds
			// kim: TODO: figure out if latestVersions is ordered most to least
			// recent.
			for _, v := range latestVersions {
				builds, err := client.GetBuildsForVersion(ctx, utility.FromStringPtr(v.Id))
				if err != nil {
					return errors.Wrapf(err, "getting builds for version '%s'", utility.FromStringPtr(v.Id))
				}
				// kim: TODO: eventually make goroutines to parallel check all
				// the builds.
				for _, b := range builds {
					if !criteria.shouldApply(utility.FromStringPtr(b.BuildVariant)) {
						continue
					}

					tasks, err := client.GetTasksForBuild(ctx, utility.FromStringPtr(b.Id))
					if err != nil {
						return errors.Wrapf(err, "getting tasks for build '%s'", utility.FromStringPtr(b.Id))
					}

					buildInfo := newLastRevisionBuildInfo(b, tasks)
					if !criteria.check(buildInfo) {
						continue
					}
					// kim: TODO: handle the builds that do match somehow
				}
			}

			return nil
		},
	}
}

// lastRevisionBuildInfo includes information needed to determine if a build
// passes a set of criteria.
type lastRevisionBuildInfo struct {
	buildVariant       string
	numTasks           int
	numSuccessfulTasks int
}

func newLastRevisionBuildInfo(b model.APIBuild, buildTasks []model.APITask) lastRevisionBuildInfo {
	numSuccessfulTasks := 0
	for _, t := range buildTasks {
		if utility.FromStringPtr(t.Status) == evergreen.TaskSucceeded {
			numSuccessfulTasks++
		}
	}
	return lastRevisionBuildInfo{
		buildVariant:       utility.FromStringPtr(b.BuildVariant),
		numTasks:           len(buildTasks),
		numSuccessfulTasks: numSuccessfulTasks,
	}
}

func (i *lastRevisionBuildInfo) successProportion() float64 {
	return float64(i.numSuccessfulTasks) / float64(i.numTasks)
}

// kim: TODO: use once flags are set up.
// lastRevisionCriteria defines the user criteria for selecting a suitable last
// revision.
type lastRevisionCriteria struct {
	// project is the project ID or identifier.
	project string
	// buildVariantRegexp is a list of regular expressions of build variant
	// names. This determines which particular build variants the criteria
	// should apply to.
	buildVariantRegexp []regexp.Regexp
	// minSuccessProportion is a criterion for the minimum proportion of tasks
	// in the build that must succeed.
	minSuccessProportion float64
}

func newLastRevisionCriteria(project string, bvRegexpsAsStr []string) (*lastRevisionCriteria, error) {
	bvRegexps := make([]regexp.Regexp, 0, len(bvRegexpsAsStr))
	for _, bvRegexpStr := range bvRegexpsAsStr {
		bvRegexp, err := regexp.Compile(bvRegexpStr)
		if err != nil {
			return nil, errors.Wrap(err, "compiling build variant regexp")
		}
		bvRegexps = append(bvRegexps, *bvRegexp)
	}
	return &lastRevisionCriteria{
		project:            project,
		buildVariantRegexp: bvRegexps,
	}, nil
}

// shouldApply returns whether the criteria applies to this build variant.
func (c *lastRevisionCriteria) shouldApply(bv string) bool {
	for _, bvRegexp := range c.buildVariantRegexp {
		if bvRegexp.MatchString(bv) {
			return true
		}
	}
	return false
}

func (c *lastRevisionCriteria) check(info lastRevisionBuildInfo) bool {
	if !c.shouldApply(info.buildVariant) {
		// The criteria does not apply to this build variant, so it
		// automatically passes checks.
		return true
	}

	if info.successProportion() < c.minSuccessProportion {
		return false
	}

	return true
}
