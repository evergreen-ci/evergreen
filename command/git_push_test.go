package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/google/shlex"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
)

func TestExecuteGitPush(t *testing.T) {
	manager := &jasper.MockManager{
		Create: func(opts *jasper.CreateOptions) jasper.MockProcess {
			_, err := opts.Output.Output.Write([]byte("abcdef01345"))
			assert.NoError(t, err)
			proc := jasper.MockProcess{}
			proc.ProcInfo.Options = *opts
			return proc
		},
	}
	c := gitPush{
		base:           base{jasper: manager},
		Directory:      "src",
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
	}

	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Task: &task.Task{}, ProjectRef: &model.ProjectRef{Branch: "master"}}
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	assert.NoError(t, err)

	patch := &patch.Patch{
		Patches: []patch.ModulePatch{
			{
				ModuleName: "",
				PatchSet: patch.PatchSet{
					Summary: []patch.Summary{
						{
							Name: "hello.txt",
						},
					},
				},
			},
		},
		Githash:     "abcdef01345",
		Description: "testing 123",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "patch", patch)
	assert.NoError(t, c.Execute(ctx, comm, logger, conf))

	commands := []string{
		"git checkout master",
		"git rev-parse HEAD",
		`git add "hello.txt"`,
		`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit -m "testing 123" --author="evergreen <evergreen@mongodb.com>"`,
		"git push origin master",
	}

	assert.Len(t, manager.Procs, len(commands))
	for i, proc := range manager.Procs {
		args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
		splitCommand, err := shlex.Split(commands[i])
		assert.NoError(t, err)
		assert.Equal(t, splitCommand, args)
	}
}

func TestPushPatch(t *testing.T) {
	manager := &jasper.MockManager{}
	c := gitPush{
		base:           base{jasper: manager},
		Directory:      "src",
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
	}

	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	assert.NoError(t, err)

	params := pushParams{
		directory:   c.Directory,
		authorName:  "baxterthehacker",
		authorEmail: "baxter@thehacker.com",
		files:       []string{"hello.txt"},
		description: "testing 123",
		branch:      "master",
	}

	assert.NoError(t, c.pushPatch(context.Background(), logger, params))
	commands := []string{
		`git add "hello.txt"`,
		`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit -m "testing 123" --author="baxterthehacker <baxter@thehacker.com>"`,
		"git push origin master",
	}
	assert.Len(t, manager.Procs, len(commands))
	for i, proc := range manager.Procs {
		args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
		splitCommand, err := shlex.Split(commands[i])
		assert.NoError(t, err)
		assert.Equal(t, splitCommand, args)
	}
}

func TestRevParse(t *testing.T) {
	manager := &jasper.MockManager{}
	c := gitPush{
		base:           base{jasper: manager},
		Directory:      "src",
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
	}

	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Project: &model.Project{}}
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	assert.NoError(t, err)

	_, err = c.revParse(context.Background(), conf, logger, "HEAD")
	assert.NoError(t, err)
	commands := []string{"git rev-parse HEAD"}

	assert.Len(t, manager.Procs, len(commands))
	for i, proc := range manager.Procs {
		args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
		splitCommand, err := shlex.Split(commands[i])
		assert.NoError(t, err)
		assert.Equal(t, splitCommand, args)
	}
}
