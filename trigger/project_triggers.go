package trigger

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version
func TriggerDownstreamVersion(args ProcessorArgs) (*model.Version, error) {
	if args.ConfigFile != "" && args.Command != "" {
		return nil, errors.New("cannot specify both a file and command")
	}
	if args.SourceVersion == nil {
		return nil, errors.Errorf("unable to find source version in project %s", args.DownstreamProject.Identifier)
	}

	// propagate version metadata to the downstream version
	metadata, err := metadataFromVersion(*args.SourceVersion, args.DownstreamProject)
	if err != nil {
		return nil, err
	}
	metadata.TriggerID = args.TriggerID
	metadata.TriggerType = args.TriggerType
	metadata.EventID = args.EventID
	metadata.TriggerDefinitionID = args.DefinitionID
	metadata.Alias = args.Alias

	// get the downstream config
	var config *model.Project
	if args.ConfigFile != "" {
		config, err = makeDownstreamConfigFromFile(args.DownstreamProject, args.ConfigFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else if args.Command != "" {
		config, err = makeDownstreamConfigFromCommand(args.DownstreamProject, args.Command, args.GenerateFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		return nil, errors.New("must specify a file or command to define downstream project config")
	}

	// create version
	v, err := repotracker.CreateVersionFromConfig(context.Background(), &args.DownstreamProject, config, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating version")
	}
	err = args.SourceVersion.AddSatisfiedTrigger(args.DefinitionID)
	if err != nil {
		return nil, err
	}
	err = model.UpdateLastRevision(v.Identifier, v.Revision)
	if err != nil {
		return nil, errors.Wrap(err, "error updating last revision")
	}
	err = repotracker.AddBuildBreakSubscriptions(v, &args.DownstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "error adding build break subscriptions")
	}
	err = model.DoProjectActivation(args.DownstreamProject.Identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "error activating project %s", args.DownstreamProject.Identifier)
	}

	return v, nil
}

func metadataFromVersion(source model.Version, ref model.ProjectRef) (repotracker.VersionMetadata, error) {
	metadata := repotracker.VersionMetadata{
		SourceVersion: &source,
	}
	metadata.Revision = model.Revision{
		Author:          source.Author,
		AuthorEmail:     source.AuthorEmail,
		CreateTime:      source.CreateTime,
		RevisionMessage: source.Message,
	}
	repo, err := model.FindRepository(ref.Identifier)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding most recent revision")
	}
	metadata.Revision.Revision = repo.LastRevision
	author, err := user.FindOneById(source.AuthorID)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding version author")
	}
	if author != nil {
		metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
	}

	return metadata, nil
}

func makeDownstreamConfigFromFile(ref model.ProjectRef, file string) (*model.Project, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error getting evergreen settings")
	}
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	configFile, err := thirdparty.GetGithubFile(ctx, token, ref.Owner, ref.Repo, file, "")
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching project file for '%s'", ref.Identifier)
	}
	fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode config file for '%s'", ref.Identifier)
	}

	config := model.Project{}
	err = model.LoadProjectInto(fileContents, ref.Identifier, &config)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing config file for '%s'", ref.Identifier)
	}
	return &config, nil
}

func makeDownstreamConfigFromCommand(ref model.ProjectRef, command, generateFile string) (*model.Project, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving config")
	}
	baseConfig := model.Project{
		Identifier: ref.Identifier,
		Tasks: []model.ProjectTask{
			{
				Name: "generate-config",
				Commands: []model.PluginCommandConf{
					{
						Command: "git.get_project",
						Type:    evergreen.CommandTypeSetup,
						Params: map[string]interface{}{
							"directory": "${workdir}/src",
						},
					},
					{
						Command: "subprocess.exec",
						Params: map[string]interface{}{
							"working_dir": "src",
							"command":     command,
						},
					},
					{
						Command: "generate.tasks",
						Params: map[string]interface{}{
							"files": []string{generateFile},
						},
					},
				},
			},
		},
		BuildVariants: model.BuildVariants{
			{
				Name:        "generate",
				DisplayName: "generate",
				RunOn:       []string{settings.Triggers.GenerateTaskDistro},
				Tasks: []model.BuildVariantTaskUnit{
					{Name: "generate-config"},
				},
			},
		},
	}
	return &baseConfig, nil
}
