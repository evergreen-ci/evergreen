package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/cheynewallace/tabby"
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
		regexpVariantsFlagName            = "regex-variants"
		regexpVariantsDisplayNameFlagName = "regex-display-variants"
		minSuccessProportionFlagName      = "min-success"
		minFinishedProportionFlagName     = "min-finished"
		successfulTasks                   = "successful-tasks"
		knownIssuesAreSuccessFlagName     = "known-issues-are-success"
		jsonFlagName                      = "json"
		timeoutFlagName                   = "timeout"
		lookbackLimitFlagName             = "lookback-limit"
		saveFlagName                      = "save"
		reuseFlagName                     = "reuse"
		listFlagName                      = "list"
	)
	return cli.Command{
		Name:  "last-revision",
		Usage: "return the latest revision for a version that matches a set of criteria, along with its modules (if any)",
		Flags: addProjectFlag(
			cli.StringSliceFlag{
				Name:  joinFlagNames(regexpVariantsFlagName, "rv"),
				Usage: "regexps for build variant names to check",
			},
			cli.StringSliceFlag{
				Name:  regexpVariantsDisplayNameFlagName,
				Usage: "regexps for build variant display names to check",
			},
			cli.Float64Flag{
				Name:  minSuccessProportionFlagName,
				Usage: "minimum proportion of successful tasks (between 0 and 1 inclusive) in a build for it to be considered a match",
			},
			cli.StringSliceFlag{
				Name:  joinFlagNames(successfulTasks, "t"),
				Usage: "names of tasks that, if present in the builds, must have succeeded",
			},
			cli.Float64Flag{
				Name:  minFinishedProportionFlagName,
				Usage: "minimum proportion of finished tasks (between 0 and 1 inclusive) in a build for it to be considered a match",
			},
			cli.BoolFlag{
				Name:  knownIssuesAreSuccessFlagName,
				Usage: "treat tasks with known issues as successful when checking the last revision criteria",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(jsonFlagName, "j"),
				Usage: "output the result in JSON format",
			},
			cli.IntFlag{
				Name:  timeoutFlagName,
				Usage: "timeout in seconds to find a revision",
				Value: 300,
			},
			cli.IntFlag{
				Name:  lookbackLimitFlagName,
				Usage: "number of recent versions to consider before giving up",
				Value: defaultLastRevisionLookbackLimit,
			},
			cli.StringFlag{
				Name:  saveFlagName,
				Usage: "instead of searching for a revision, save the last revision criteria for reuse with the given group name. If a set of criteria already exists in the group for the same build variant name/display name regexps, the old criteria will be overwritten.",
			},
			cli.StringFlag{
				Name:  reuseFlagName,
				Usage: "reuse a set of last revision criteria by group name",
			},
			cli.BoolFlag{
				Name:  listFlagName,
				Usage: "list all saved last revision criteria groups",
			},
		),
		Before: mergeBeforeFuncs(setPlainLogger,
			func(c *cli.Context) error {
				if c.Bool(jsonFlagName) {
					// If running with JSON output, don't try to upgrade the CLI
					// because it will produce extraneous non-JSON output.
					return nil
				}
				return autoUpdateCLI(c)
			},
			requireProjectFlag,
			requireAtLeastOneFlag(reuseFlagName, regexpVariantsFlagName, regexpVariantsDisplayNameFlagName),
			func(c *cli.Context) error {
				if c.Float64(minSuccessProportionFlagName) < 0 || c.Float64(minSuccessProportionFlagName) > 1 {
					return errors.New("minimum success proportion must be between 0 and 1 inclusive")
				}
				if c.Float64(minFinishedProportionFlagName) < 0 || c.Float64(minFinishedProportionFlagName) > 1 {
					return errors.New("minimum finished proportion must be between 0 and 1 inclusive")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.Int(timeoutFlagName) < 0 {
					return errors.New("timeout must be a non-negative integer")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.Int(lookbackLimitFlagName) <= 0 {
					return errors.New("lookback limit must be a positive integer")
				}
				return nil
			},
			requireAtLeastOneFlag(reuseFlagName, minSuccessProportionFlagName, minFinishedProportionFlagName, successfulTasks),
			mutuallyExclusiveArgs(false, reuseFlagName, saveFlagName, listFlagName),
			func(c *cli.Context) error {
				reuseCriteria := c.String(reuseFlagName) != ""
				if reuseCriteria && (len(c.StringSlice(regexpVariantsFlagName)) > 0 || len(c.StringSlice(regexpVariantsDisplayNameFlagName)) > 0 ||
					c.Float64(minSuccessProportionFlagName) > 0 || c.Float64(minFinishedProportionFlagName) > 0 || len(c.StringSlice(successfulTasks)) > 0) {
					return errors.New("cannot both reuse criteria and also specify other criteria")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			projectID := c.String(projectFlagName)
			bvRegexps := c.StringSlice(regexpVariantsFlagName)
			bvDisplayNameRegexps := c.StringSlice(regexpVariantsDisplayNameFlagName)
			minSuccessProp := c.Float64(minSuccessProportionFlagName)
			minFinishedProp := c.Float64(minFinishedProportionFlagName)
			successfulTasks := c.StringSlice(successfulTasks)
			knownIssuesAreSuccess := c.Bool(knownIssuesAreSuccessFlagName)
			jsonOutput := c.Bool(jsonFlagName)
			versionLookbackLimit := c.Int(lookbackLimitFlagName)
			saveCriteriaName := c.String(saveFlagName)
			reuseCriteriaName := c.String(reuseFlagName)
			listCriteria := c.Bool(listFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}
			if saveCriteriaName != "" {
				criteria, err := newLastRevisionCriteria(projectID, bvRegexps, bvDisplayNameRegexps, minSuccessProp, minFinishedProp, successfulTasks, knownIssuesAreSuccess)
				if err != nil {
					return errors.Wrap(err, "building last revision options")
				}

				cg, err := saveLastRevisionCriteria(conf, saveCriteriaName, criteria)
				if err != nil {
					return errors.Wrap(err, "saving last revision criteria")
				}
				if err := printCriteriaGroup(cg, jsonOutput); err != nil {
					return errors.Wrap(err, "printing last revision criteria group")
				}
				return nil
			}
			if listCriteria {
				// kim: TODO: implement logic to list all criteria groups.
			}

			var ctx context.Context
			var cancel context.CancelFunc
			timeoutSecs := c.Int(timeoutFlagName)
			if timeoutSecs > 0 {
				ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
				defer cancel()
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()
			}

			client, err := conf.setupRestCommunicator(ctx, !jsonOutput)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			var allCriteria []lastRevisionCriteria
			if reuseCriteriaName != "" {
				allCriteria, err = getLastRevisionCriteria(conf, reuseCriteriaName, projectID, knownIssuesAreSuccess)
				if err != nil {
					return errors.Wrapf(err, "getting last revision criteria with name '%s'", reuseCriteriaName)
				}
			} else {
				criteria, err := newLastRevisionCriteria(projectID, bvRegexps, bvDisplayNameRegexps, minSuccessProp, minFinishedProp, successfulTasks, knownIssuesAreSuccess)
				if err != nil {
					return errors.Wrap(err, "building last revision options")
				}
				allCriteria = []lastRevisionCriteria{*criteria}
			}

			// Search for a suitable version in batches to reduce request times.
			// Otherwise without any batching, the initial time to get recent
			// versions becomes slower as the lookback limit increases.
			const maxVersionBatchSize = 20
			var orderNum int
			var matchingVersion *model.APIVersion
			for numRevisionsSearched := 0; numRevisionsSearched < versionLookbackLimit; numRevisionsSearched += maxVersionBatchSize {
				numRevisionsToSearch := min(maxVersionBatchSize, versionLookbackLimit-numRevisionsSearched)
				latestVersions, err := client.GetRecentVersionsForProject(ctx, c.String(projectFlagName), evergreen.RepotrackerVersionRequester, orderNum, numRevisionsToSearch)
				if err != nil {
					return errors.Wrap(err, "getting latest versions for project")
				}

				matchingVersion, err = findLatestMatchingVersion(ctx, client, latestVersions, allCriteria)
				if err != nil {
					return errors.Wrap(err, "finding latest matching revision")
				}
				if matchingVersion != nil {
					break
				}

				// latestVersions is always sorted from most to least recent
				// version, so the last version in the slice is the first
				// version to start at (exclusive) for the next batch of
				// versions.
				orderNum = latestVersions[len(latestVersions)-1].Order
				if orderNum <= 1 {
					// If the last order number searched is 1, that's the
					// earliest waterfall version, so there's no more versions
					// to search.
					break
				}
			}

			if matchingVersion == nil {
				return errors.New("no matching version found")
			}
			modules, err := getModulesForVersion(ctx, client, utility.FromStringPtr(matchingVersion.Id))
			if err != nil {
				return errors.Wrapf(err, "getting modules for matching version '%s'", utility.FromStringPtr(matchingVersion.Id))
			}
			if err := printLastRevision(matchingVersion, modules, jsonOutput); err != nil {
				return errors.Wrap(err, "printing last revision")
			}

			return nil
		},
	}
}

func printLastRevision(v *model.APIVersion, modules []model.APIManifestModule, jsonOutput bool) error {
	versionID := utility.FromStringPtr(v.Id)
	revision := utility.FromStringPtr(v.Revision)

	if jsonOutput {
		type moduleInfo struct {
			Name     string `json:"name"`
			Revision string `json:"revision"`
		}
		var moduleNameAndRevisions []moduleInfo
		if len(modules) > 0 {
			moduleNameAndRevisions = make([]moduleInfo, 0, len(modules))
			for _, m := range modules {
				moduleNameAndRevisions = append(moduleNameAndRevisions, moduleInfo{
					Name:     utility.FromStringPtr(m.Name),
					Revision: utility.FromStringPtr(m.Revision),
				})
			}
		}

		output, err := json.MarshalIndent(struct {
			VersionID string       `json:"version_id"`
			Revision  string       `json:"revision"`
			Modules   []moduleInfo `json:"modules,omitzero"`
		}{
			VersionID: versionID,
			Revision:  revision,
			Modules:   moduleNameAndRevisions,
		}, "", "\t")
		if err != nil {
			return errors.Wrap(err, "marshalling output to JSON")
		}
		fmt.Println(string(output))
		return nil
	}

	fmt.Printf("Latest version that matches criteria: %s\nRevision: %s\n", utility.FromStringPtr(v.Id), utility.FromStringPtr(v.Revision))
	if len(modules) > 0 {
		fmt.Println("Modules:")
		for _, m := range modules {
			fmt.Printf("- name: %s\n  revision: %s\n", utility.FromStringPtr(m.Name), utility.FromStringPtr(m.Revision))
		}
	} else {
		fmt.Println("No modules found for this version.")
	}
	return nil
}

func printCriteriaGroup(cg *lastRevisionCriteriaGroup, jsonOutput bool) error {
	if jsonOutput {
		output, err := json.MarshalIndent(cg, "", "\t")
		if err != nil {
			return errors.Wrap(err, "marshalling criteria group to JSON")
		}
		fmt.Println(string(output))
		return nil
	}

	t := tabby.New()
	t.AddHeader("Name", "Build Variant Regexps", "Build Variant Display Name Regexps", "Min Success Proportion", "Min Finished Proportion", "Required Successful Tasks")
	for i, c := range cg.Criteria {
		name := cg.Name
		if i > 0 {
			name = ""
		}
		bvRegexps := ""
		if len(c.BVRegexps) > 0 {
			bvRegexps = fmt.Sprint(c.BVRegexps)
		}
		bvDisplayRegexps := ""
		if len(c.BVDisplayRegexps) > 0 {
			bvDisplayRegexps = fmt.Sprint(c.BVDisplayRegexps)
		}
		minSuccessProp := ""
		if c.MinSuccessProportion > 0 {
			minSuccessProp = fmt.Sprintf("%.2f", c.MinSuccessProportion)
		}
		minFinishedProp := ""
		if c.MinFinishedProportion > 0 {
			minFinishedProp = fmt.Sprintf("%.2f", c.MinFinishedProportion)
		}
		successfulTasks := ""
		if len(c.SuccessfulTasks) > 0 {
			successfulTasks = fmt.Sprint(c.SuccessfulTasks)
		}
		t.AddLine(name, bvRegexps, bvDisplayRegexps, minSuccessProp, minFinishedProp, successfulTasks)
	}
	t.Print()
	return nil
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
	// buildVariantDisplayName is the display name of the build variant for this
	// build.
	buildVariantDisplayName string
	// allTasks is the set of all tasks in the build.
	allTasks []model.APITask
	// numSuccessfulTasks is the number of tasks in the build that succeeded.
	numSuccessfulTasks int
	// numFinishedTasks is the number of tasks in the build that finished.
	numFinishedTasks int
}

func newLastRevisionBuildInfo(b model.APIBuild, buildTasks []model.APITask, knownIssuesAreSuccess bool) lastRevisionBuildInfo {
	numFinishedTasks := 0
	numSuccessfulTasks := 0
	for _, t := range buildTasks {
		status := utility.FromStringPtr(t.Status)
		if evergreen.IsFinishedTaskStatus(status) {
			numFinishedTasks++
		}
		if isSuccessfulTask(t, knownIssuesAreSuccess) {
			numSuccessfulTasks++
		}
	}
	return lastRevisionBuildInfo{
		buildID:                 utility.FromStringPtr(b.Id),
		buildVariant:            utility.FromStringPtr(b.BuildVariant),
		buildVariantDisplayName: utility.FromStringPtr(b.DisplayName),
		versionID:               utility.FromStringPtr(b.Version),
		allTasks:                buildTasks,
		numSuccessfulTasks:      numSuccessfulTasks,
		numFinishedTasks:        numFinishedTasks,
	}
}

// successProportion calculates the proportion of successful tasks out of all
// the tasks in the build.
func (i *lastRevisionBuildInfo) successProportion() float64 {
	return float64(i.numSuccessfulTasks) / float64(len(i.allTasks))
}

// finishedProportion calculates the proportion of finished tasks out of all
// the tasks in the build.
func (i *lastRevisionBuildInfo) finishedProportion() float64 {
	return float64(i.numFinishedTasks) / float64(len(i.allTasks))
}

// isSuccessfulTask checks if a task is considered successful for last revision
// criteria, which means either the task succeeded or if knownIssuesAreSuccess
// is true, the task failed with known issues.
func isSuccessfulTask(t model.APITask, knownIssuesAreSuccess bool) bool {
	status := utility.FromStringPtr(t.Status)
	if status == evergreen.TaskSucceeded {
		return true
	}
	return knownIssuesAreSuccess && evergreen.IsFailedTaskStatus(status) && t.HasAnnotations
}

// lastRevisionCriteria defines the user criteria for selecting a suitable last
// revision.
type lastRevisionCriteria struct {
	// project is the project ID or identifier.
	project string
	// buildVariantRegexps is a list of regular expressions of matching build
	// variant names. This determines which particular build variants the
	// criteria should apply to.
	buildVariantRegexps []regexp.Regexp
	// buildVariantDisplayNameRegexps is a list of regular expressions of matching
	// build variant display names. This determines which particular build
	// variants the criteria should apply to.
	buildVariantDisplayNameRegexps []regexp.Regexp
	// minSuccessProportion is a criterion for the minimum proportion of tasks
	// in a matching build that must succeed.
	minSuccessProportion float64
	// minSuccessProportion is a criterion for the minimum proportion of tasks
	// in a matching build that must be finished.
	minFinishedProportion float64
	// successfulTasks is a criterion for the list of task names that, if
	// present in the build, must have succeeded. If the task is not present in
	// the build, then this criterion does not apply.
	successfulTasks []string
	// knownIssuesAreSuccess indicates whether tasks with known issues
	// should be treated as successful when checking the criteria.
	knownIssuesAreSuccess bool
}

func newLastRevisionCriteria(project string, bvRegexpsAsStr, bvDisplayRegexpsAsStr []string, minSuccessProportion, minFinishedProportion float64, successfulTasks []string, knownIssuesAreSuccess bool) (*lastRevisionCriteria, error) {
	bvRegexps := make([]regexp.Regexp, 0, len(bvRegexpsAsStr))
	for _, bvRegexpStr := range bvRegexpsAsStr {
		bvRegexp, err := regexp.Compile(bvRegexpStr)
		if err != nil {
			return nil, errors.Wrapf(err, "compiling build variant regexp '%s'", bvRegexpStr)
		}
		bvRegexps = append(bvRegexps, *bvRegexp)
	}
	bvDisplayRegexps := make([]regexp.Regexp, 0, len(bvDisplayRegexpsAsStr))
	for _, bvDisplayRegexpStr := range bvDisplayRegexpsAsStr {
		bvDisplayRegexp, err := regexp.Compile(bvDisplayRegexpStr)
		if err != nil {
			return nil, errors.Wrapf(err, "compiling build variant display name regexp '%s'", bvDisplayRegexpStr)
		}
		bvDisplayRegexps = append(bvDisplayRegexps, *bvDisplayRegexp)
	}

	return &lastRevisionCriteria{
		project:                        project,
		buildVariantRegexps:            bvRegexps,
		buildVariantDisplayNameRegexps: bvDisplayRegexps,
		minSuccessProportion:           minSuccessProportion,
		minFinishedProportion:          minFinishedProportion,
		successfulTasks:                successfulTasks,
		knownIssuesAreSuccess:          knownIssuesAreSuccess,
	}, nil
}

// shouldApply returns whether the criteria applies to this build variant.
func (c *lastRevisionCriteria) shouldApply(bv, bvDisplayName string) bool {
	for _, bvRegexp := range c.buildVariantRegexps {
		if bvRegexp.MatchString(bv) {
			return true
		}
	}
	for _, bvDisplayNameRegexp := range c.buildVariantDisplayNameRegexps {
		if bvDisplayNameRegexp.MatchString(bvDisplayName) {
			return true
		}
	}
	return false
}

// check returns whether the the criteria applies to the build and if so, if it
// passes all the criteria. This returns true if the criteria does not apply.
func (c *lastRevisionCriteria) check(info lastRevisionBuildInfo) bool {
	if !c.shouldApply(info.buildVariant, info.buildVariantDisplayName) {
		// The criteria does not apply to this build variant, so it
		// automatically passes checks.
		return true
	}

	if info.successProportion() < c.minSuccessProportion {
		grip.Debug(message.Fields{
			"message":                "build does not meet minimum successful tasks proportion",
			"version_id":             info.versionID,
			"build_id":               info.buildID,
			"build_variant":          info.buildVariant,
			"min_success_proportion": c.minSuccessProportion,
			"success_proportion":     info.successProportion(),
		})
		return false
	}

	if info.finishedProportion() < c.minFinishedProportion {
		grip.Debug(message.Fields{
			"message":                 "build does not meet minimum finished tasks proportion",
			"version_id":              info.versionID,
			"build_id":                info.buildID,
			"build_variant":           info.buildVariant,
			"min_finished_proportion": c.minFinishedProportion,
			"finished_proportion":     info.finishedProportion(),
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
		if !isSuccessfulTask(tsk, c.knownIssuesAreSuccess) {
			grip.Debug(message.Fields{
				"message":                  "build has required task but it was not successful",
				"version_id":               info.versionID,
				"build_id":                 info.buildID,
				"build_variant":            info.buildVariant,
				"required_successful_task": taskName,
				"task_status":              utility.FromStringPtr(tsk.Status),
			})
			return false
		}
	}

	return true
}

// findLatestMatchingVersion iterates through the latest versions and finds the
// first one that matches the criteria. It returns nil version if no matching
// version is found.
func findLatestMatchingVersion(ctx context.Context, c client.Communicator, latestVersions []model.APIVersion, criteria []lastRevisionCriteria) (*model.APIVersion, error) {
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
func checkBuildsPassCriteria(ctx context.Context, c client.Communicator, builds []model.APIBuild, criteria []lastRevisionCriteria) (passesCriteria bool, err error) {
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
func checkBuildPassesCriteria(ctx context.Context, c client.Communicator, b model.APIBuild, criteria []lastRevisionCriteria) (passesCriteria bool, err error) {
	anyCriteriaApply := false
	for _, c := range criteria {
		if c.shouldApply(utility.FromStringPtr(b.BuildVariant), utility.FromStringPtr(b.DisplayName)) {
			anyCriteriaApply = true
			break
		}
	}
	if !anyCriteriaApply {
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

	for _, c := range criteria {
		buildInfo := newLastRevisionBuildInfo(b, tasks, c.knownIssuesAreSuccess)
		passesCriteria := c.check(buildInfo)
		if !passesCriteria {
			return false, nil
		}
	}
	return true, nil
}

// lastRevisionCriteriaGroup is a group of last revision criteria that can be
// saved and reused.
type lastRevisionCriteriaGroup struct {
	Name     string                         `json:"name" yaml:"name"`
	Criteria []reusableLastRevisionCriteria `json:"criteria" yaml:"criteria"`
}

func (cg *lastRevisionCriteriaGroup) toLastRevisionCriteria(projectID string, knownIssuesAreSuccess bool) ([]lastRevisionCriteria, error) {
	allCriteria := make([]lastRevisionCriteria, 0, len(cg.Criteria))
	for i, c := range cg.Criteria {
		criteria, err := newLastRevisionCriteria(projectID, c.BVRegexps, c.BVDisplayRegexps, c.MinSuccessProportion, c.MinFinishedProportion, c.SuccessfulTasks, knownIssuesAreSuccess)
		if err != nil {
			return nil, errors.Wrapf(err, "reusing last revision criteria from group '%s' at index %d", cg.Name, i)
		}
		allCriteria = append(allCriteria, *criteria)
	}
	return allCriteria, nil
}

// reusableLastRevisionCriteria defines the criteria for the last revision that
// can be saved and reused.
type reusableLastRevisionCriteria struct {
	BVRegexps             []string `json:"bv_regexps,omitempty" yaml:"bv_regexps,omitempty"`
	BVDisplayRegexps      []string `json:"bv_display_regexps,omitempty" yaml:"bv_display_regexps,omitempty"`
	MinSuccessProportion  float64  `json:"min_success_proportion,omitempty" yaml:"min_success_proportion,omitempty"`
	MinFinishedProportion float64  `json:"min_finished_proportion,omitempty" yaml:"min_finished_proportion,omitempty"`
	SuccessfulTasks       []string `json:"successful_tasks,omitempty" yaml:"successful_tasks,omitempty"`
}

func newReusableLastRevisionCriteria(c *lastRevisionCriteria) reusableLastRevisionCriteria {
	var bvRegexps []string
	for _, bvRegexp := range c.buildVariantRegexps {
		bvRegexps = append(bvRegexps, bvRegexp.String())
	}
	var bvDisplayRegexps []string
	for _, bvDisplayRegexp := range c.buildVariantDisplayNameRegexps {
		bvDisplayRegexps = append(bvDisplayRegexps, bvDisplayRegexp.String())
	}
	return reusableLastRevisionCriteria{
		BVRegexps:             bvRegexps,
		BVDisplayRegexps:      bvDisplayRegexps,
		MinSuccessProportion:  c.minSuccessProportion,
		MinFinishedProportion: c.minFinishedProportion,
		SuccessfulTasks:       c.successfulTasks,
	}
}

func saveLastRevisionCriteria(conf *ClientSettings, name string, criteria *lastRevisionCriteria) (*lastRevisionCriteriaGroup, error) {
	criteriaToSave := newReusableLastRevisionCriteria(criteria)
	var criteriaGroup *lastRevisionCriteriaGroup
	var criteriaGroupIdx int
	for i, cg := range conf.LastRevisionCriteriaGroups {
		if cg.Name == name {
			criteriaGroup = &cg
			criteriaGroupIdx = i
			break
		}
	}
	if criteriaGroup == nil {
		criteriaGroup = &lastRevisionCriteriaGroup{
			Name:     name,
			Criteria: []reusableLastRevisionCriteria{criteriaToSave},
		}
		conf.LastRevisionCriteriaGroups = append(conf.LastRevisionCriteriaGroups, *criteriaGroup)
	} else {
		// If criteria already exists in this group for the same build variant
		// name/display name regexps, overwrite it with the new criteria.
		// Otherwise, append the new criteria to the group.
		isNewCriteriaForGroup := true
		for i, c := range criteriaGroup.Criteria {
			if stringSetEquals(c.BVRegexps, criteriaToSave.BVRegexps) && stringSetEquals(c.BVDisplayRegexps, criteriaToSave.BVDisplayRegexps) {
				criteriaGroup.Criteria[i] = criteriaToSave
				isNewCriteriaForGroup = false
			}
		}
		if isNewCriteriaForGroup {
			criteriaGroup.Criteria = append(criteriaGroup.Criteria, criteriaToSave)
		}
		conf.LastRevisionCriteriaGroups[criteriaGroupIdx] = *criteriaGroup
	}

	if err := conf.Write(""); err != nil {
		return nil, errors.Wrap(err, "writing last revision criteria to configuration file")
	}

	return criteriaGroup, nil
}

func getLastRevisionCriteria(conf *ClientSettings, name, project string, knownIssuesAreSuccess bool) ([]lastRevisionCriteria, error) {
	var criteriaGroup *lastRevisionCriteriaGroup
	for _, cg := range conf.LastRevisionCriteriaGroups {
		if cg.Name == name {
			criteriaGroup = &cg
			break
		}
	}
	if criteriaGroup == nil {
		return nil, errors.Errorf("no last revision criteria group found with name '%s'", name)
	}

	return criteriaGroup.toLastRevisionCriteria(project, knownIssuesAreSuccess)
}

// stringSetEquals checks if two slices of strings have the same sets of
// strings, ignoring the order of elements and duplicates.
func stringSetEquals(a, b []string) bool {
	uniqueToA, uniqueToB := utility.StringSliceSymmetricDifference(a, b)
	return len(uniqueToA) == 0 && len(uniqueToB) == 0
}

func getModulesForVersion(ctx context.Context, c client.Communicator, versionID string) ([]model.APIManifestModule, error) {
	mfst, err := c.GetManifestForVersion(ctx, versionID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting manifest for version '%s'", versionID)
	}
	if mfst == nil {
		// Manifests are only available for versions using modules, so it's
		// valid to not get a manifest.
		return nil, nil
	}
	return mfst.Modules, nil
}
