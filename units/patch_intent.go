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
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
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
	logger   grip.Journaler
	env      evergreen.Environment

	Intent     patch.Intent  `bson:"intent" json:"intent"`
	PatchID    bson.ObjectId `bson:"patch_id" json:"patch_id" yaml:"patch_id"`
	user       *user.DBUser
	projectRef *model.ProjectRef
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
		env:    evergreen.GetEnvironment(),
		logger: logging.MakeGrip(grip.GetSender()),
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
	defer j.Intent.SetProcessed()

	patchDoc := j.Intent.NewPatch()
	switch j.Intent.GetType() {
	case patch.CliIntentType:
		j.AddError(j.buildCliPatchDoc(patchDoc, githubOauthToken))

	case patch.GithubIntentType:
		j.AddError(j.buildGithubPatchDoc(patchDoc, githubOauthToken))

	default:
		j.AddError(errors.Errorf("Intent type '%s' is unknown", j.Intent.GetType()))
	}
	if len := len(patchDoc.Patches); len != 1 {
		j.AddError(errors.Errorf("patch document should have 1 patch, found %d", len))
	}
	if j.HasErrors() {
		j.logger.Error(message.Fields{
			"message": "Failed to build patch document",
			"errors":  j.Error().Error(),
		})
		return
	}

	j.user, err = user.FindOne(user.ById(patchDoc.Author))
	if err != nil {
		j.AddError(err)
		return
	}
	if j.user == nil {
		j.AddError(errors.New("Can't find patch author"))
		return
	}

	// Get and validate patched config and add it to the patch document
	project, err := validator.GetPatchedProject(patchDoc, githubOauthToken)
	if err != nil {
		j.AddError(errors.Wrap(err, "invalid patched config"))
		return
	}

	if patchDoc.Patches[0].ModuleName != "" {
		// is there a module? validate it.
		var module *model.Module
		module, err = project.GetModuleByName(patchDoc.Patches[0].ModuleName)
		if err != nil {
			j.AddError(errors.Wrapf(err, "could not find module '%s'", patchDoc.Patches[0].ModuleName))
		}
		if module == nil {
			j.AddError(errors.Errorf("no module named '%s'", patchDoc.Patches[0].ModuleName))
		}
	}

	// verify that all variants exists
	for _, buildVariant := range patchDoc.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			j.AddError(errors.Errorf("No such buildvariant: '%s'", buildVariant))
		}
	}

	if j.HasErrors() {
		return
	}

	// add the project config
	projectYamlBytes, err := yaml.Marshal(project)
	if err != nil {
		j.AddError(errors.Wrap(err, "error marshaling patched config"))
		return
	}
	patchDoc.PatchedConfig = string(projectYamlBytes)

	project.BuildProjectTVPairs(patchDoc, j.Intent.GetAlias())

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = j.user.IncPatchNumber()
	if err != nil {
		j.AddError(errors.Wrap(err, "error computing patch num"))
		return
	}

	patchDoc.CreateTime = time.Now()
	patchDoc.Id = j.PatchID

	if err := patchDoc.Insert(); err != nil {
		j.AddError(err)
		return
	}

	if j.Intent.ShouldFinalizePatch() {
		if _, err := model.FinalizePatch(patchDoc, j.Intent.RequesterIdentity(), githubOauthToken); err != nil {
			j.AddError(err)
		}

		// TODO ugly
		if j.Intent.GetType() == patch.GithubIntentType {
			job := NewGithubStatusUpdateJobForPatchWithVersion(patchDoc.Version)
			job.Run()
		}
	}
}

func fetchPatchByURL(URL string) (string, error) {
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

func (j *patchIntentProcessor) buildCliPatchDoc(patchDoc *patch.Patch, githubOauthToken string) (err error) {
	j.projectRef, err = model.FindOneProjectRef(patchDoc.Project)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref '%s'", patchDoc.Project)
	}
	if j.projectRef == nil {
		return errors.Errorf("Could not find project ref '%s'", patchDoc.Project)
	}

	gitCommit, err := thirdparty.GetCommitEvent(githubOauthToken, j.projectRef.Owner,
		j.projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "could not find base revision '%s' for project '%s'",
			patchDoc.Githash, j.projectRef.Identifier)
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
	mustBeMemberOfOrg := j.env.Settings().GithubPRTesting.MemberOf
	if mustBeMemberOfOrg == "" {
		err = errors.New("Github PR testing not configured correctly; requires a Github org to authenticate against")
		return
	}

	j.projectRef, err = model.FindOneProjectRefByRepo(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo)
	if err != nil {
		return errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo)
	}
	if j.projectRef == nil {
		return errors.Errorf("Could not find project ref for repo '%s/%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo)
	}

	projectVars, err := model.FindOneProjectVars(j.projectRef.Identifier)
	if err != nil {
		return errors.Wrapf(err, "Could not find project vars for project '%s'", j.projectRef.Identifier)
	}
	if projectVars == nil {
		return errors.Errorf("Could not find project vars for project '%s'", j.projectRef.Identifier)
	}

	isMember, err := authAndFetchPRMergeBase(patchDoc, mustBeMemberOfOrg,
		patchDoc.GithubPatchData.Author, githubOauthToken)
	if err != nil {
		return err
	}
	if !isMember {
		return errors.Errorf("user is not member of %s", patchDoc.GithubPatchData.BaseOwner)
	}

	patchContent, err := fetchPatchByURL(patchDoc.GithubPatchData.DiffURL)
	if err != nil {
		return err
	}

	summaries, err := thirdparty.GetPatchSummaries(patchContent)
	if err != nil {
		return err
	}

	patchFileId := bson.NewObjectId().Hex()
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		ModuleName: "",
		Githash:    patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileId,
			Summary:     summaries,
		},
	})
	patchDoc.Project = j.projectRef.Identifier

	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "failed to write patch file to db")
	}

	return nil
}

func authAndFetchPRMergeBase(patchDoc *patch.Patch, requiredOrganization, githubUser, githubOauthToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token := strings.Split(githubOauthToken, " ")
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token[1]},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

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
