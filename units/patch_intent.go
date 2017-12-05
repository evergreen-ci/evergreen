package units

import (
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
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
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
	Intent   patch.Intent `bson:"intent" json:"intent"`
	logger   grip.Journaler

	user       *user.DBUser
	projectRef *model.ProjectRef
}

// NewHostStatsCollector logs statistics about host utilization per
// distro to the default grip logger.
func NewPatchIntentProcessor(id string, intent patch.Intent) amboy.Job {
	j := makePatchIntentProcessor()
	j.SetID(id)
	j.Intent = intent
	return j
}

func makePatchIntentProcessor() *patchIntentProcessor {
	return &patchIntentProcessor{
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
	githubOauthToken := evergreen.GetEnvironment().Settings().Credentials["github"]

	defer j.MarkComplete()
	if j.Intent == nil {
		j.AddError(errors.New("nil intent"))
		return
	}
	defer j.Intent.SetProcessed()

	patchDoc := j.Intent.NewPatch()

	switch j.Intent.GetType() {
	case patch.GithubIntentType:
		j.AddError(j.buildGithubPatchDoc(patchDoc))

	case patch.CliIntentType:
		j.AddError(j.buildCliPatchDoc(patchDoc, githubOauthToken))

	default:
		j.AddError(errors.Errorf("Intent type '%s' is unknown", j.Intent.GetType()))
	}

	var err error
	j.user, err = user.FindOne(user.ById(patchDoc.Author))
	if err != nil {
		j.AddError(err)
	}
	if j.user == nil {
		j.AddError(errors.New("Can't find user"))
	}

	if j.HasErrors() {
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
			j.AddError(errors.Wrapf(err, "could not find module %v", patchDoc.Patches[0].ModuleName))
		}
		if module == nil {
			j.AddError(errors.Errorf("no module named %s", patchDoc.Patches[0].ModuleName))
		}
	}

	// verify that all variants exists
	for _, buildVariant := range patchDoc.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			j.AddError(errors.Errorf("No such buildvariant: %v", buildVariant))
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

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = j.user.IncPatchNumber()
	if err != nil {
		j.AddError(errors.Wrap(err, "error computing patch num"))
		return
	}
	patchDoc.PatchedConfig = string(projectYamlBytes)

	patchDoc.ClearPatchData()

	model.ExpandTasksAndVariants(patchDoc, project)

	var pairs []model.TVPair
	for _, v := range patchDoc.BuildVariants {
		for _, t := range patchDoc.Tasks {
			if project.FindTaskForVariant(t, v) != nil {
				pairs = append(pairs, model.TVPair{v, t})
			}
		}
	}

	// update variant and tasks to include dependencies
	pairs = model.IncludePatchDependencies(project, pairs)

	patchDoc.SyncVariantsTasks(model.TVPairsToVariantTasks(pairs))
	patchDoc.CreateTime = time.Now()

	if err := patchDoc.Insert(); err != nil {
		j.AddError(err)
		return
	}

	if j.Intent.ShouldFinalizePatch() {
		if _, err := model.FinalizePatch(patchDoc, githubOauthToken); err != nil {
			j.AddError(err)
			return
		}
	}
}

func fetchPatchByURL(URL string) (string, error) {
	resp, err := http.Get(URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Expected 200 OK, got %s", http.StatusText(resp.StatusCode))
	}
	if resp.ContentLength > patch.SizeLimit || resp.ContentLength == 0 {
		return "", errors.Errorf("Patch contents must be at least 1 byte and no greater than %d bytes; was %d bytes", patch.SizeLimit, resp.ContentLength)
	}

	bytes, err := ioutil.ReadAll(resp.Body)

	return string(bytes), nil
}

func (j *patchIntentProcessor) buildCliPatchDoc(patchDoc *patch.Patch, githubOauthToken string) error {
	var err error
	j.projectRef, err = model.FindOneProjectRef(patchDoc.Project)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref %v", patchDoc.Project)
	}
	if j.projectRef == nil {
		return errors.Errorf("Could not find project : %s", patchDoc.Project)
	}

	gitCommit, err := thirdparty.GetCommitEvent(githubOauthToken, j.projectRef.Owner, j.projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "could not find base revision %s for project %s",
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

func (j *patchIntentProcessor) buildGithubPatchDoc(patchDoc *patch.Patch) error {
	var err error
	j.projectRef, err = model.FindOneProjectRefByRepo(patchDoc.GithubPatchData.Owner, patchDoc.GithubPatchData.Repository)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref %v", patchDoc.Project)
	}
	if j.projectRef == nil {
		return errors.Errorf("Could not find project : %s", patchDoc.Project)
	}

	// TODO: dbuser
	// TODO: build variants and tasks
	patchContent, err := fetchPatchByURL(patchDoc.GithubPatchData.PatchURL)
	if err != nil {
		return err
	}

	summaries, err := thirdparty.GetPatchSummaries(patchContent)
	if err != nil {
		return err
	}

	patchFileId := bson.NewObjectId().String()
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		ModuleName: "",
		Githash:    patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileId,
			Summary:     summaries,
		},
	})

	if err := db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(patchContent)); err != nil {
		return errors.Wrap(err, "failed to write patch file to db")
	}

	return nil
}
