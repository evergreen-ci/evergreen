package units

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
)

const patchIntentJobName = "patch-intent-processor"

func init() {
	registry.AddJobType(patchIntentJobName,
		func() amboy.Job { return makePatchIntentProcessor() })
}

type patchIntentProcessor struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

	Intent  patch.Intent  `bson:"intent" json:"intent"`
	PatchID bson.ObjectId `bson:"patch_id" json:"patch_id" yaml:"patch_id"`
	user    *user.DBUser
}

// NewPatchIntentProcessor creates an amboy job to create a patch from the
// given patch intent with the given object ID for the patch
func NewPatchIntentProcessor(patchID bson.ObjectId, intent patch.Intent) amboy.Job {
	j := makePatchIntentProcessor()
	j.SetID(fmt.Sprintf("%s-%s-%s", patchIntentJobName, intent.GetType(), intent.ID()))
	j.Intent = intent
	j.PatchID = patchID
	return j
}

func makePatchIntentProcessor() *patchIntentProcessor {
	return &patchIntentProcessor{
		env: evergreen.GetEnvironment(),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    patchIntentJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func (j *patchIntentProcessor) Run() {
	defer j.MarkComplete()
	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		j.AddError(err)
	}
	if j.Intent == nil {
		j.AddError(errors.New("nil intent"))
	}
	if j.HasErrors() {
		return
	}

	patchDoc := j.Intent.NewPatch()

	if err := j.finishPatch(patchDoc, githubOauthToken); err != nil {
		j.AddError(err)
		return
	}

	if j.Intent.GetType() == patch.GithubIntentType {
		update := NewGithubStatusUpdateJobForPatchWithVersion(patchDoc.Version)
		err = j.env.LocalQueue().Put(update)
		j.AddError(err)
		grip.ErrorWhen(err != nil, message.Fields{
			"message":            "Failed to queue status update",
			"errors":             err.Error(),
			"job":                j.ID(),
			"patch_id":           j.PatchID,
			"update_id":          update.ID(),
			"update_for_version": patchDoc.Version,
			"intent_type":        j.Intent.GetType(),
			"intent_id":          j.Intent.ID(),
		})
	}
}

func (j *patchIntentProcessor) finishPatch(patchDoc *patch.Patch, githubOauthToken string) error {
	c := grip.NewCatcher()

	switch j.Intent.GetType() {
	case patch.CliIntentType:
		c.Add(j.buildCliPatchDoc(patchDoc, githubOauthToken))

	case patch.GithubIntentType:
		c.Add(j.buildGithubPatchDoc(patchDoc, githubOauthToken))

	default:
		return errors.Errorf("Intent type '%s' is unknown", j.Intent.GetType())
	}
	if len := len(patchDoc.Patches); len != 1 {
		c.Add(errors.Errorf("patch document should have 1 patch, found %d", len))
	}
	if c.HasErrors() {
		grip.Error(message.Fields{
			"message":     "Failed to build patch document",
			"errors":      c.Resolve().Error(),
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"intent_type": j.Intent.GetType(),
			"intent_id":   j.Intent.ID(),
		})
		return c.Resolve()
	}
	var err error
	if j.user == nil {
		j.user, err = user.FindOne(user.ById(patchDoc.Author))
		if err != nil {
			return err
		}
		if j.user == nil {
			return errors.New("Can't find patch author")
		}
	}

	// Get and validate patched config and add it to the patch document
	project, err := validator.GetPatchedProject(patchDoc, githubOauthToken)
	if err != nil {
		return errors.Wrap(err, "invalid patched config")
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

	project.BuildProjectTVPairs(patchDoc, j.Intent.GetAlias())

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

	if j.Intent.ShouldFinalizePatch() {
		if _, err := model.FinalizePatch(patchDoc, j.Intent.RequesterIdentity(), githubOauthToken); err != nil {
			grip.Error(message.Fields{
				"message":     "Failed to finalize patch document",
				"errors":      c.Resolve().Error(),
				"job":         j.ID(),
				"patch_id":    j.PatchID,
				"intent_type": j.Intent.GetType(),
				"intent_id":   j.Intent.ID(),
			})
			return err
		}

	}

	return nil
}

func fetchDiffByURL(URL string) (string, error) {
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	resp, err := client.Get(URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Expected 200 OK, got %s", http.StatusText(resp.StatusCode))
	}
	if resp.ContentLength > patch.SizeLimit || resp.ContentLength == 0 {
		return "", errors.Errorf("Patch contents must be at least 1 byte and no greater than %d bytes; was %d bytes",
			patch.SizeLimit, resp.ContentLength)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (j *patchIntentProcessor) buildCliPatchDoc(patchDoc *patch.Patch, githubOauthToken string) error {
	defer j.Intent.SetProcessed()
	projectRef, err := model.FindOneProjectRef(patchDoc.Project)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref '%s'", patchDoc.Project)
	}
	if projectRef == nil {
		return errors.Errorf("Could not find project ref '%s'", patchDoc.Project)
	}

	gitCommit, err := thirdparty.GetCommitEvent(githubOauthToken, projectRef.Owner,
		projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "could not find base revision '%s' for project '%s'",
			patchDoc.Githash, projectRef.Identifier)
	}
	if gitCommit == nil {
		return errors.Errorf("commit hash %s doesn't seem to exist", patchDoc.Githash)
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

func (j *patchIntentProcessor) buildGithubPatchDoc(patchDoc *patch.Patch, githubOauthToken string) (err error) {
	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "github pr testing is disabled, error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.GithubPRTestingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     patchIntentJobName,
			"message": "github pr testing is disabled, not processing pull request",

			"intent_type": j.Intent.GetType(),
			"intent_id":   j.Intent.ID(),
		})
		return errors.New("github pr testing is disabled, not processing pull request")
	}
	defer j.Intent.SetProcessed()

	mustBeMemberOfOrg := j.env.Settings().GithubPRCreatorOrg
	if mustBeMemberOfOrg == "" {
		return errors.New("Github PR testing not configured correctly; requires a Github org to authenticate against")
	}

	projectRef, err := model.FindOneProjectRefByRepo(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo)
	if err != nil {
		return errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo)
	}
	if projectRef == nil {
		return errors.Errorf("Could not find project ref for repo '%s/%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo)
	}

	projectVars, err := model.FindOneProjectVars(projectRef.Identifier)
	if err != nil {
		return errors.Wrapf(err, "Could not find project vars for project '%s'", projectRef.Identifier)
	}
	if projectVars == nil {
		return errors.Errorf("Could not find project vars for project '%s'", projectRef.Identifier)
	}

	isMember, err := authAndFetchPRMergeBase(context.TODO(), patchDoc, mustBeMemberOfOrg,
		patchDoc.GithubPatchData.Author, githubOauthToken)
	if err != nil {
		grip.Alert(message.Fields{
			"message":   "github API failure",
			"source":    "patch intents",
			"job":       j.ID(),
			"patch_id":  j.PatchID,
			"error":     err.Error(),
			"base_repo": fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo": fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number": patchDoc.GithubPatchData.PRNumber,

			"intent_type": j.Intent.GetType(),
			"intent_id":   j.Intent.ID(),
		})
		return err
	}
	if !isMember {
		return errors.Errorf("user is not a member of %s", mustBeMemberOfOrg)
	}

	patchContent, err := fetchDiffByURL(patchDoc.GithubPatchData.DiffURL)
	if err != nil {
		return err
	}

	summaries, err := thirdparty.GetPatchSummaries(patchContent)
	if err != nil {
		return err
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

	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "failed to write patch file to db")
	}

	j.user, err = user.FindOne(user.ById(evergreen.GithubPatchUser))
	if err != nil {
		return err
	}
	if j.user == nil {
		j.user = &user.DBUser{
			Id:       evergreen.GithubPatchUser,
			DispName: "Github Pull Requests",
			APIKey:   util.RandomString(),
		}
		err = j.user.Insert()
	}

	return errors.Wrap(err, "failed to create github pull request user")
}

func authAndFetchPRMergeBase(ctx context.Context, patchDoc *patch.Patch, requiredOrganization, githubUser, githubOauthToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpClient, err := util.GetHttpClientForOauth2(githubOauthToken)
	if err != nil {
		return false, err
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	// doesn't count against API limits
	limits, _, err := client.RateLimits(ctx)
	if err != nil {
		return false, err
	}
	if limits == nil || limits.Core == nil {
		return false, errors.New("rate limits response was empty")
	}
	if limits.Core.Remaining < 3 {
		return false, errors.New("github rate limit would be exceeded")
	}

	isMember, _, err := client.Organizations.IsMember(context.Background(), requiredOrganization, githubUser)
	if !isMember || err != nil {
		grip.Info(message.Fields{
			"message":      "Failed to authenticate github PR",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		})
		return false, err
	}

	commits, _, err := client.PullRequests.ListCommits(ctx, patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.PRNumber, nil)
	if err != nil {
		return isMember, err
	}
	if len(commits) == 0 {
		return isMember, errors.New("No commits received from github")
	}
	if commits[0].SHA == nil {
		return isMember, errors.New("hash is missing from pull request commit list")
	}

	commit, _, err := client.Repositories.GetCommit(ctx,
		patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo, *commits[0].SHA)
	if err != nil {
		return isMember, err
	}
	if commit == nil {
		return isMember, errors.New("couldn't find commit")
	}
	if len(commit.Parents) == 0 {
		return isMember, errors.New("can't find pull request branch point")
	}
	if commit.Parents[0].SHA == nil {
		return isMember, errors.New("parent hash is missing")
	}

	patchDoc.Githash = *commit.Parents[0].SHA

	return true, nil
}
