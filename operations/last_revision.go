package operations

import (
	"context"
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
	const (
		regexpVariantsFlagName       = "regex-variants"
		minSuccessProportionFlagName = "min-success"
		successfulTasks              = "successful-tasks"
	)
	return cli.Command{
		Name:  "last-revision",
		Usage: "return the latest revision for a version that matches a set of criteria",
		Flags: addProjectFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(regexpVariantsFlagName, "rv"),
				Usage: "regexps for build variant names to check",
			},
			cli.Float64Flag{
				Name:  joinFlagNames(minSuccessProportionFlagName),
				Usage: "minimum proportion of successful tasks (between 0 and 1 inclusive) in a build for it to be considered a match",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(successfulTasks, "t"),
				Usage: "names of tasks that, if present in the builds, must have succeeded",
			},
		),
		Before: mergeBeforeFuncs(autoUpdateCLI, setPlainLogger, func(c *cli.Context) error {
			if len(c.StringSlice(regexpVariantsFlagName)) == 0 {
				return errors.New("must specify at least one build variant regexp")
			}
			return nil
		}),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			projectID := c.String(projectFlagName)
			regexpBVs := c.StringSlice(regexpVariantsFlagName)
			minSuccessProp := c.Float64(minSuccessProportionFlagName)
			successfulTasks := c.StringSlice(successfulTasks)

			criteria, err := newLastRevisionCriteria(projectID, regexpBVs, minSuccessProp, successfulTasks)
			if err != nil {
				return errors.Wrap(err, "building last revision options")
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			latestVersions, err := client.GetRecentVersionsForProject(ctx, c.String(projectFlagName), evergreen.RepotrackerVersionRequester, defaultLastRevisionLookbackLimit)
			if err != nil {
				return errors.Wrap(err, "getting latest versions for project")
			}

			matchingVersion, err := findLatestMatchingVersion(ctx, client, latestVersions, *criteria)
			if err != nil {
				return errors.Wrap(err, "finding latest matching revision")
			}
			if matchingVersion == nil {
				return errors.New("no matching version found")
			}

			grip.Infof("Latest version that matches criteria: %s\nRevision: %s\n", utility.FromStringPtr(matchingVersion.Id), utility.FromStringPtr(matchingVersion.Revision))

			return nil
		},
	}
}

// lastRevisionBuildInfo includes information needed to determine if a build
// passes a set of criteria.
type lastRevisionBuildInfo struct {
	// buildID is the ID of the build.
	buildID string
	// versionID is the ID of the version that this build belongs to.
	versionID string
	// buildVariant is the name of the build variant for this build.
	buildVariant string
	// allTasks is the set of all tasks in the build.
	allTasks []model.APITask
	// numSuccessfulTasks is the number of tasks in the build that succeeded.
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
		buildID:            utility.FromStringPtr(b.Id),
		buildVariant:       utility.FromStringPtr(b.BuildVariant),
		versionID:          utility.FromStringPtr(b.Version),
		allTasks:           buildTasks,
		numSuccessfulTasks: numSuccessfulTasks,
	}
}

// successProportion calculates the proportion of successful tasks out of all
// the tasks in the build.
func (i *lastRevisionBuildInfo) successProportion() float64 {
	return float64(i.numSuccessfulTasks) / float64(len(i.allTasks))
}

// lastRevisionCriteria defines the user criteria for selecting a suitable last
// revision.
type lastRevisionCriteria struct {
	// project is the project ID or identifier.
	project string
	// buildVariantRegexp is a list of regular expressions of matching build
	// variant names. This determines which particular build variants the
	// criteria should apply to.
	buildVariantRegexp []regexp.Regexp
	// minSuccessProportion is a criterion for the minimum proportion of tasks
	// in a matching build that must succeed.
	minSuccessProportion float64
	// successfulTasks is a criterion for the list of task names that, if
	// present in the build, must have succeeded. If the task is not present in
	// the build, then this criterion does not apply.
	successfulTasks []string
}

func newLastRevisionCriteria(project string, bvRegexpsAsStr []string, minSuccessProportion float64, successfulTasks []string) (*lastRevisionCriteria, error) {
	if len(bvRegexpsAsStr) == 0 {
		return nil, errors.New("must specify at least one build variant regexp for criteria")
	}
	if minSuccessProportion < 0 || minSuccessProportion > 1 {
		return nil, errors.New("minimum success proportion must be between 0 and 1 inclusive")
	}
	if project == "" {
		return nil, errors.New("must specify a project")
	}

	bvRegexps := make([]regexp.Regexp, 0, len(bvRegexpsAsStr))
	for _, bvRegexpStr := range bvRegexpsAsStr {
		bvRegexp, err := regexp.Compile(bvRegexpStr)
		if err != nil {
			return nil, errors.Wrapf(err, "compiling build variant regexp '%s'", bvRegexpStr)
		}
		bvRegexps = append(bvRegexps, *bvRegexp)
	}

	return &lastRevisionCriteria{
		project:              project,
		successfulTasks:      successfulTasks,
		buildVariantRegexp:   bvRegexps,
		minSuccessProportion: minSuccessProportion,
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

// check returns whether the the criteria applies to the build and if so, if it
// passes all the criteria. This returns true if the criteria does not apply.
func (c *lastRevisionCriteria) check(info lastRevisionBuildInfo) bool {
	if !c.shouldApply(info.buildVariant) {
		// The criteria does not apply to this build variant, so it
		// automatically passes checks.
		return true
	}

	if info.successProportion() < c.minSuccessProportion {
		grip.Debug(message.Fields{
			"message":                "build does not meet minimum success proportion",
			"version_id":             info.versionID,
			"build_id":               info.buildID,
			"build_variant":          info.buildVariant,
			"min_success_proportion": c.minSuccessProportion,
			"success_proportion":     info.successProportion(),
		})
		return false
	}

	allTasksSet := make(map[string]model.APITask, len(info.allTasks))
	for _, t := range info.allTasks {
		allTasksSet[utility.FromStringPtr(t.DisplayName)] = t
	}
	for _, taskName := range c.successfulTasks {
		tsk, ok := allTasksSet[taskName]
		if !ok {
			// The task does not run in this build, so the criteria does not
			// apply.
			continue
		}
		if status := utility.FromStringPtr(tsk.Status); status != evergreen.TaskSucceeded {
			grip.Debug(message.Fields{
				"message":                  "build has required task but it was not successful",
				"version_id":               info.versionID,
				"build_id":                 info.buildID,
				"build_variant":            info.buildVariant,
				"required_successful_task": taskName,
				"task_status":              status,
			})
			return false
		}
	}

	return true
}

// findLatestMatchingVersion iterates through the latest versions and finds the
// first one that matches the criteria. It returns nil version if no matching
// version is found.
func findLatestMatchingVersion(ctx context.Context, c client.Communicator, latestVersions []model.APIVersion, criteria lastRevisionCriteria) (*model.APIVersion, error) {
	for _, v := range latestVersions {
		grip.Debug(message.Fields{
			"message":    "checking version",
			"version_id": utility.FromStringPtr(v.Id),
			"revision":   utility.FromStringPtr(v.Revision),
			"project":    utility.FromStringPtr(v.Project),
		})

		builds, err := c.GetBuildsForVersion(ctx, utility.FromStringPtr(v.Id))
		if err != nil {
			return nil, errors.Wrapf(err, "getting builds for version '%s'", utility.FromStringPtr(v.Id))
		}

		passesCriteria, err := checkBuildsPassCriteria(ctx, c, builds, criteria)
		if err != nil {
			return nil, err
		}
		if !passesCriteria {
			continue
		}

		return &v, nil
	}

	return nil, nil
}

// checkBuildsPassCriteria checks if all the provided builds pass the criteria.
func checkBuildsPassCriteria(ctx context.Context, c client.Communicator, builds []model.APIBuild, criteria lastRevisionCriteria) (passesCriteria bool, err error) {
	type buildResult struct {
		passesCriteria bool
		err            error
	}

	buildResults := make(chan buildResult, len(builds))
	wg := sync.WaitGroup{}
	for _, b := range builds {
		wg.Add(1)

		go func() {
			defer wg.Done()

			res := buildResult{}
			res.passesCriteria, res.err = checkBuildPassesCriteria(ctx, c, b, criteria)
			select {
			case <-ctx.Done():
			case buildResults <- res:
			}
		}()
	}

	wg.Wait()
	close(buildResults)

	catcher := grip.NewBasicCatcher()
	allBuildsPassedCriteria := true
	for res := range buildResults {
		if res.err != nil {
			catcher.Add(res.err)
		}
		if !res.passesCriteria {
			allBuildsPassedCriteria = false
		}
	}
	return allBuildsPassedCriteria, catcher.Resolve()
}

// checkBuildPassesCriteria checks if a single build passes the criteria.
func checkBuildPassesCriteria(ctx context.Context, c client.Communicator, b model.APIBuild, criteria lastRevisionCriteria) (passesCriteria bool, err error) {
	if !criteria.shouldApply(utility.FromStringPtr(b.BuildVariant)) {
		return true, nil
	}

	grip.Debug(message.Fields{
		"message":       "checking build for last revision criteria",
		"build_id":      utility.FromStringPtr(b.Id),
		"build_variant": utility.FromStringPtr(b.BuildVariant),
		"version":       utility.FromStringPtr(b.Version),
	})

	tasks, err := c.GetTasksForBuild(ctx, utility.FromStringPtr(b.Id))
	if err != nil {
		return false, errors.Wrapf(err, "getting tasks for build '%s'", utility.FromStringPtr(b.Id))
	}

	buildInfo := newLastRevisionBuildInfo(b, tasks)

	return criteria.check(buildInfo), nil
}
