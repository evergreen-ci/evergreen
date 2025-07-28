package operations

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const defaultLastRevisionLookbackLimit = 50

func LastRevision() cli.Command {
	return cli.Command{
		Name:   "last-revision",
		Usage:  "return the latest revision that shouldApply a set of criteria",
		Flags:  addProjectFlag(addVariantsRegexpFlag()...),
		Before: mergeBeforeFuncs(autoUpdateCLI, setPlainLogger, requireVariantsFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			criteria, err := newLastRevisionCriteria(c.String(projectFlagName), c.StringSlice(variantsFlagName))
			if err != nil {
				return errors.Wrap(err, "building last revision options")
			}

			grip.Debugf("Criteria: %s\n", criteria.String())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

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

			revisionInfo, err := findLatestMatchingRevision(ctx, client, latestVersions, *criteria)
			if err != nil {
				return errors.Wrap(err, "finding latest matching revision")
			}

			grip.Infof("Latest revision that matches criteria: %s\n", revisionInfo.revision)

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

func (c *lastRevisionCriteria) String() string {
	bvRegexps := make([]string, 0, len(c.buildVariantRegexp))
	for _, re := range c.buildVariantRegexp {
		bvRegexps = append(bvRegexps, re.String())
	}
	return fmt.Sprintf("Project: %s\nBuild Variant Regexps: %v\nMin Success Proportion: %f", c.project, bvRegexps, c.minSuccessProportion)
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
		grip.Debug(message.Fields{
			"message":                "build does not meet minimum success proportion",
			"build_variant":          info.buildVariant,
			"min_success_proportion": c.minSuccessProportion,
			"success_proportion":     info.successProportion(),
		})
		return false
	}

	return true
}

type lastRevisionInfo struct {
	// revision is the latest revision that matches the criteria.
	revision string
}

func findLatestMatchingRevision(ctx context.Context, c client.Communicator, latestVersions []model.APIVersion, criteria lastRevisionCriteria) (*lastRevisionInfo, error) {
	// GET /versions/{version_id}/builds
	// kim: TODO: figure out if latestVersions is ordered most to least
	// recent.
	for _, v := range latestVersions {
		builds, err := c.GetBuildsForVersion(ctx, utility.FromStringPtr(v.Id))
		if err != nil {
			return nil, errors.Wrapf(err, "getting builds for version '%s'", utility.FromStringPtr(v.Id))
		}
		// kim: TODO: eventually make goroutines to parallel check all
		// the builds.

		type buildResult struct {
			passesCriteria bool
			err            error
		}

		buildResults := make(chan buildResult, len(builds))
		wg := sync.WaitGroup{}
		for _, b := range builds {
			if !criteria.shouldApply(utility.FromStringPtr(b.BuildVariant)) {
				continue
			}

			grip.Debug(message.Fields{
				"message":       "checking build for last revision criteria",
				"build_id":      b.Id,
				"build_variant": b.BuildVariant,
				"version":       b.Version,
			})

			wg.Add(1)

			go func() {
				defer wg.Done()

				res := buildResult{}
				res.passesCriteria, res.err = checkBuildMatchesCriteria(ctx, c, b, criteria)
				select {
				case <-ctx.Done():
				case buildResults <- res:
				}
			}()
		}

		wg.Wait()

		catcher := grip.NewBasicCatcher()
		allBuildsPassedCriteria := true
		for res := range buildResults {
			if res.err != nil {
				catcher.Add(errors.Wrapf(res.err, "checking build '%s' for last revision criteria", utility.FromStringPtr(v.Id)))
			}
			if !res.passesCriteria {
				allBuildsPassedCriteria = false
			}
		}
		if catcher.HasErrors() {
			return nil, catcher.Resolve()
		}
		if !allBuildsPassedCriteria {
			continue
		}

		return &lastRevisionInfo{
			revision: utility.FromStringPtr(v.Revision),
		}, nil
	}

	return nil, errors.New("no matching revision found")
}

func checkBuildMatchesCriteria(ctx context.Context, c client.Communicator, b model.APIBuild, criteria lastRevisionCriteria) (passesCriteria bool, err error) {
	if !criteria.shouldApply(utility.FromStringPtr(b.BuildVariant)) {
		return false, nil // criteria does not apply to this build variant
	}

	tasks, err := c.GetTasksForBuild(ctx, utility.FromStringPtr(b.Id))
	if err != nil {
		return false, errors.Wrapf(err, "getting tasks for build '%s'", utility.FromStringPtr(b.Id))
	}

	buildInfo := newLastRevisionBuildInfo(b, tasks)

	return criteria.check(buildInfo), nil
}
