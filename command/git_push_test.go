package command

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitPush(t *testing.T) {
	token := "0123456789"
	c := gitPush{
		Directory:      "src",
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
		Token:          token,
	}
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{
		Task:       &task.Task{},
		ProjectRef: &model.ProjectRef{Branch: "master"},
		Distro:     &distro.Distro{CloneMethod: distro.CloneMethodOAuth},
		Expansions: &util.Expansions{},
	}
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	require.NoError(t, err)

	var splitCommand []string
	for name, test := range map[string]func(*testing.T){
		"Execute": func(*testing.T) {
			manager := &mock.Manager{}
			c.base.jasper = manager
			manager.Create = func(opts *options.Create) mock.Process {
				_, err = opts.Output.Output.Write([]byte("abcdef01345"))
				assert.NoError(t, err)
				proc := mock.Process{}
				proc.ProcInfo.Options = *opts
				return proc
			}
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
				`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit --file - --author="evergreen <evergreen@mongodb.com>"`,
				"git push origin master",
			}

			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
		"PushPatchMock": func(*testing.T) {
			manager := &mock.Manager{}
			manager.Create = func(opts *options.Create) mock.Process {
				_, err = opts.Output.Error.Write([]byte(fmt.Sprintf("The key: %s", token)))
				assert.NoError(t, err)
				proc := mock.Process{}
				proc.ProcInfo.Options = *opts
				return proc
			}
			c.base.jasper = manager
			params := pushParams{
				directory:     c.Directory,
				authorName:    "baxterthehacker",
				authorEmail:   "baxter@thehacker.com",
				commitMessage: "testing 123",
				branch:        "master",
				token:         token,
			}

			assert.NoError(t, c.pushPatch(context.Background(), logger, params))
			commands := []string{
				`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit --file - --author="baxterthehacker <baxter@thehacker.com>"`,
				"git push origin master",
			}
			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}

			assert.NoError(t, logger.Close())
			msgs := comm.GetMockMessages()[""]
			assert.Equal(t, "The key: [redacted oauth token]", msgs[len(msgs)-1].Message)
		},
		"PushPatch": func(*testing.T) {
			ctx := context.Background()
			jpm, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			c.base.jasper = jpm
			c.DryRun = true

			repoDir, err := ioutil.TempDir("", "test_repo")
			require.NoError(t, err)
			require.NoError(t, ioutil.WriteFile(path.Join(repoDir, "test1.txt"), []byte("test1"), 0644))
			require.NoError(t, ioutil.WriteFile(path.Join(repoDir, "test2.txt"), []byte("test2"), 0644))

			// create repo
			createRepoCommands := []string{
				`git init`,
				`git add --all`,
				`git -c "user.name=baxterthehacker" -c "user.email=baxter@thehacker.com" commit -m "testing..."`,
			}
			cmd := jpm.CreateCommand(ctx).Directory(repoDir).Append(createRepoCommands...)
			require.NoError(t, cmd.Run(ctx))

			require.NoError(t, ioutil.WriteFile(path.Join(repoDir, "test3.txt"), []byte("test3"), 0644))
			addToIndexCommands := []string{
				`git rm test1.txt`,
				`git add test3.txt`,
			}
			cmd = jpm.CreateCommand(ctx).Directory(repoDir).Append(addToIndexCommands...)
			require.NoError(t, cmd.Run(ctx))

			params := pushParams{
				directory:     repoDir,
				authorName:    "baxterthehacker",
				authorEmail:   "baxter@thehacker.com",
				commitMessage: "testing 123",
				branch:        "master",
			}
			assert.NoError(t, c.pushPatch(ctx, logger, params))

			stdout := noopWriteCloser{&bytes.Buffer{}}
			cmd = jpm.CreateCommand(ctx).Directory(repoDir).Append(`git diff-tree --no-commit-id --name-only -r HEAD`).SetOutputWriter(stdout)
			assert.NoError(t, cmd.Run(ctx))

			filesChanged := strings.TrimSpace(stdout.String())
			filesChangedSlice := strings.Split(strings.Replace(filesChanged, "\r\n", "\n", -1), "\n")
			assert.Len(t, filesChangedSlice, 2)
			assert.Equal(t, "test1.txt", filesChangedSlice[0])
			assert.Equal(t, "test3.txt", filesChangedSlice[1])

			assert.NoError(t, os.RemoveAll(repoDir))
		},
		"RevParse": func(*testing.T) {
			manager := &mock.Manager{}
			c.base.jasper = manager
			_, err = c.revParse(context.Background(), conf, logger, "HEAD")
			assert.NoError(t, err)
			commands := []string{"git rev-parse HEAD"}

			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
	} {
		c.DryRun = false
		t.Run(name, test)
	}
}
