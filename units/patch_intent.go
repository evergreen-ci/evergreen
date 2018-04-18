package units

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
)

const (
	githubAcceptDiff        = "application/vnd.github.v3.diff"
	patchIntentJobName      = "patch-intent-processor"
	errInvalidPatchedConfig = "invalid patched config"
)

func init() {
	registry.AddJobType(patchIntentJobName,
		func() amboy.Job { return makePatchIntentProcessor() })
}

type patchIntentProcessor struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

	IntentID   string        `bson:"intent_id" json:"intent_id" yaml:"intent_id"`
	IntentType string        `bson:"intent_type" json:"intent_type" yaml:"intent_type"`
	PatchID    bson.ObjectId `bson:"patch_id,omitempty" json:"patch_id" yaml:"patch_id"`

	user   *user.DBUser
	intent patch.Intent
}

// NewPatchIntentProcessor creates an amboy job to create a patch from the
// given patch intent with the given object ID for the patch
func NewPatchIntentProcessor(patchID bson.ObjectId, intent patch.Intent) amboy.Job {
	j := makePatchIntentProcessor()
	j.IntentID = intent.ID()
	j.IntentType = intent.GetType()
	j.PatchID = patchID
	j.intent = intent

	j.SetID(fmt.Sprintf("%s-%s-%s", patchIntentJobName, j.IntentType, j.IntentID))
	return j
}

func makePatchIntentProcessor() *patchIntentProcessor {
	j := &patchIntentProcessor{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    patchIntentJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *patchIntentProcessor) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		j.AddError(err)
	}
	if j.intent == nil {
		j.intent, err = patch.FindIntent(j.IntentID, j.IntentType)
		j.AddError(err)
	}

	if j.HasErrors() {
		return
	}

	patchDoc := j.intent.NewPatch()

	if err = j.finishPatch(ctx, patchDoc, githubOauthToken); err != nil {
		j.AddError(err)
		if j.IntentType == patch.GithubIntentType && strings.HasPrefix(err.Error(), errInvalidPatchedConfig) {
			update := NewGithubStatusUpdateJobForBadConfig(j.intent.ID())
			update.Run(ctx)
			j.AddError(update.Error())
		}
		return
	}

	if j.IntentType == patch.GithubIntentType {
		var update amboy.Job
		if len(patchDoc.Version) == 0 {
			update = NewGithubStatusUpdateJobForExternalPatch(patchDoc.Id.Hex())

		} else {
			update = NewGithubStatusUpdateJobForPatchWithVersion(patchDoc.Version)
		}
		update.Run(ctx)
		j.AddError(update.Error())
		grip.ErrorWhen(err != nil, message.WrapError(err, message.Fields{
			"message":            "Failed to queue status update",
			"job":                j.ID(),
			"patch_id":           j.PatchID,
			"update_id":          update.ID(),
			"update_for_version": patchDoc.Version,
			"intent_type":        j.IntentType,
			"intent_id":          j.IntentID,
			"source":             "patch intents",
		}))

		j.AddError(model.AbortPatchesWithGithubPatchData(patchDoc.CreateTime,
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.PRNumber))
	}
}

func (j *patchIntentProcessor) finishPatch(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) error {
	catcher := grip.NewBasicCatcher()

	var err error
	canFinalize := true
	switch j.IntentType {
	case patch.CliIntentType:
		catcher.Add(j.buildCliPatchDoc(ctx, patchDoc, githubOauthToken))

	case patch.GithubIntentType:
		canFinalize, err = j.buildGithubPatchDoc(ctx, patchDoc, githubOauthToken)
		catcher.Add(err)

	default:
		return errors.Errorf("Intent type '%s' is unknown", j.IntentType)
	}
	if len := len(patchDoc.Patches); len != 1 {
		catcher.Add(errors.Errorf("patch document should have 1 patch, found %d", len))
	}

	if err = catcher.Resolve(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "Failed to build patch document",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
			"source":      "patch intents",
		}))

		return err
	}

	if j.user == nil {
		j.user, err = user.FindOne(user.ById(patchDoc.Author))
		if err != nil {
			return err
		}
		if j.user == nil {
			return errors.New("Can't find patch author")
		}
	}

	pref, err := model.FindOneProjectRef(patchDoc.Project)
	if err != nil {
		return errors.Wrap(err, "can't find patch project")
	}

	if !pref.Enabled {
		return errors.New("project is disabled")
	}

	if pref.PatchingDisabled {
		return errors.New("patching is diabled for project")
	}

	// Get and validate patched config and add it to the patch document
	project, err := validator.GetPatchedProject(ctx, patchDoc, githubOauthToken)
	if err != nil {
		return errors.Wrap(err, errInvalidPatchedConfig)
	}

	if patchDoc.Patches[0].ModuleName != "" {
		// is there a module? validate it.
		var module *model.Module
		module, err = project.GetModuleByName(patchDoc.Patches[0].ModuleName)
		if err != nil {
			return errors.Wrapf(err, "could not find module '%s'", patchDoc.Patches[0].ModuleName)
		}
		if module == nil {
			return errors.Errorf("no module named '%s'", patchDoc.Patches[0].ModuleName)
		}
	}

	// verify that all variants exists
	for _, buildVariant := range patchDoc.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return errors.Errorf("No such buildvariant: '%s'", buildVariant)
		}
	}

	// add the project config
	projectYamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return errors.Wrap(err, "error marshaling patched config")
	}
	patchDoc.PatchedConfig = string(projectYamlBytes)

	project.BuildProjectTVPairs(patchDoc, j.intent.GetAlias())

	if j.intent.ShouldFinalizePatch() && len(patchDoc.Tasks) == 0 &&
		len(patchDoc.BuildVariants) == 0 {
		return errors.New("patch has no build variants or tasks")
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = j.user.IncPatchNumber()
	if err != nil {
		return errors.Wrap(err, "error computing patch num")
	}

	patchDoc.CreateTime = time.Now()
	patchDoc.Id = j.PatchID

	if err := patchDoc.Insert(); err != nil {
		return err
	}

	if canFinalize && j.intent.ShouldFinalizePatch() {
		if _, err := model.FinalizePatch(ctx, patchDoc, j.intent.RequesterIdentity(), githubOauthToken); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "Failed to finalize patch document",
				"job":         j.ID(),
				"patch_id":    j.PatchID,
				"intent_type": j.IntentType,
				"intent_id":   j.IntentID,
				"source":      "patch intents",
			}))
			return err
		}
	}

	return nil
}

// buildPatchURL creates a URL to enable downloading patch files through the
// Github API
func buildPatchURL(gp *patch.GithubPatch) string {
	url := &url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path: fmt.Sprintf("/repos/%s/%s/pulls/%d.diff", gp.BaseOwner,
			gp.BaseRepo, gp.PRNumber),
	}

	return url.String()
}

func fetchDiffFromGithub(gh *patch.GithubPatch, token string) (string, []patch.Summary, error) {
	client, err := util.GetOAuth2HTTPClient(token)
	if err != nil {
		return "", nil, errors.Wrap(err, "error getting http client")
	}
	defer util.PutHTTPClient(client)

	req, err := http.NewRequest("GET", buildPatchURL(gh), nil)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to create github request")
	}

	diff, err := doGithubRequest(client, req, githubAcceptDiff)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to fetch diff from github")
	}

	summaries, err := thirdparty.GetPatchSummaries(diff)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get patch summary")
	}

	return diff, summaries, nil
}

func doGithubRequest(client *http.Client, req *http.Request, accept string) (string, error) {
	req.Header.Del("Accept")
	req.Header.Add("Accept", accept)

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch data from github")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Expected 200 OK, got %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if resp.ContentLength > patch.SizeLimit || resp.ContentLength == 0 {
		return "", errors.Errorf("Patch contents must be at least 1 byte and no greater than %d bytes; was %d bytes",
			patch.SizeLimit, resp.ContentLength)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response body from github response")
	}

	return string(bytes), nil
}

func (j *patchIntentProcessor) buildCliPatchDoc(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) error {
	defer j.intent.SetProcessed()
	projectRef, err := model.FindOneProjectRef(patchDoc.Project)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref '%s'", patchDoc.Project)
	}
	if projectRef == nil {
		return errors.Errorf("Could not find project ref '%s'", patchDoc.Project)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = thirdparty.GetCommitEvent(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "could not find base revision '%s' for project '%s'",
			patchDoc.Githash, projectRef.Identifier)
	}

	var reader io.ReadCloser
	reader, err = db.GetGridFile(patch.GridFSPrefix, patchDoc.Patches[0].PatchSet.PatchFileId)
	if err != nil {
		return err
	}
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}

	summaries, err := thirdparty.GetPatchSummaries(string(bytes))
	if err != nil {
		return err
	}
	patchDoc.Patches[0].ModuleName = ""
	patchDoc.Patches[0].PatchSet.Summary = summaries

	return nil
}

func (j *patchIntentProcessor) buildGithubPatchDoc(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) (bool, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return false, errors.Wrap(err, "github pr testing is disabled, error retrieving admin settings")
	}
	if flags.GithubPRTestingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     patchIntentJobName,
			"message": "github pr testing is disabled, not processing pull request",

			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		})
		return false, errors.New("github pr testing is disabled, not processing pull request")
	}
	defer j.intent.SetProcessed()

	mustBeMemberOfOrg := j.env.Settings().GithubPRCreatorOrg
	if mustBeMemberOfOrg == "" {
		return false, errors.New("Github PR testing not configured correctly; requires a Github org to authenticate against")
	}

	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch)
	if err != nil {
		return false, errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}
	if projectRef == nil {
		return false, errors.Errorf("Could not find project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}

	projectVars, err := model.FindOneProjectVars(projectRef.Identifier)
	if err != nil {
		return false, errors.Wrapf(err, "Could not find project vars for project '%s'", projectRef.Identifier)
	}
	if projectVars == nil {
		return false, errors.Errorf("Could not find project vars for project '%s'", projectRef.Identifier)
	}

	isMember, err := authAndFetchPRMergeBase(ctx, patchDoc, mustBeMemberOfOrg,
		patchDoc.GithubPatchData.Author, githubOauthToken)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "github API failure",
			"source":      "patch intents",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"base_repo":   fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":   fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":   patchDoc.GithubPatchData.PRNumber,
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		}))
		return false, err
	}

	patchContent, summaries, err := fetchDiffFromGithub(&patchDoc.GithubPatchData, githubOauthToken)
	if err != nil {
		return isMember, err
	}

	patchFileID := fmt.Sprintf("%s_%s", patchDoc.Id.Hex(), patchDoc.Githash)
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		ModuleName: "",
		Githash:    patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileID,
			Summary:     summaries,
		},
	})
	patchDoc.Project = projectRef.Identifier

	if err = db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return isMember, errors.Wrap(err, "failed to write patch file to db")
	}

	j.user, err = findEvergreenUserForPR(patchDoc.GithubPatchData.AuthorUID)
	if err != nil {
		return isMember, errors.Wrap(err, "failed to fetch user")
	}
	patchDoc.Author = j.user.Id

	return isMember, nil
}

func findEvergreenUserForPR(githubUID int) (*user.DBUser, error) {
	// try and find a user by github uid
	u, err := user.FindByGithubUID(githubUID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	// Otherwise, use the github patch user
	u, err = user.FindOne(user.ById(evergreen.GithubPatchUser))
	if err != nil {
		return u, err
	}
	// and if that user doesn't exist, make it
	if u == nil {
		u = &user.DBUser{
			Id:       evergreen.GithubPatchUser,
			DispName: "Github Pull Requests",
			APIKey:   util.RandomString(),
		}
		if err = u.Insert(); err != nil {
			return nil, errors.Wrap(err, "failed to create github pull request user")
		}
	}

	return u, err
}

func authAndFetchPRMergeBase(ctx context.Context, patchDoc *patch.Patch, requiredOrganization, githubUser, githubOauthToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	isMember, err := thirdparty.GithubUserInOrganization(ctx, githubOauthToken, requiredOrganization, githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "Failed to authenticate github PR",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
		return false, err

	}

	hash, err := thirdparty.GetPullRequestMergeBase(ctx, githubOauthToken, patchDoc.GithubPatchData)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "Failed to authenticate github PR",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
		return isMember, err
	}

	patchDoc.Githash = hash

	return isMember, nil
}
