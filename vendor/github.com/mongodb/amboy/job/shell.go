package job

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/grip"
)

// ShellJob is an amboy.Job implementation that runs shell commands in
// the context of an amboy.Job object.
type ShellJob struct {
	Command    string            `bson:"command" json:"command" yaml:"command"`
	Output     string            `bson:"output" json:"output" yaml:"output"`
	WorkingDir string            `bson:"working_dir" json:"working_dir" yaml:"working_dir"`
	Env        map[string]string `bson:"env" json:"env" yaml:"env"`

	Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

// NewShellJob takes the command, as a string along with the name of a
// file that the command would create, and returns a pointer to a
// ShellJob object. If the "creates" argument is an empty string then
// the command always runs, otherwise only if the file specified does
// not exist. You can change the dependency with the SetDependency
// argument.
func NewShellJob(cmd string, creates string) *ShellJob {
	j := NewShellJobInstance()
	j.Command = cmd

	if creates != "" {
		j.SetDependency(dependency.NewCreatesFile(creates))
	}

	j.SetID(fmt.Sprintf("shell-job-%d-%s", GetNumber(), strings.Split(cmd, " ")[0]))

	return j
}

// NewShellJobInstance returns a pointer to an initialized ShellJob
// instance, but does not set the command or the name. Use when the
// command is not known at creation time.
func NewShellJobInstance() *ShellJob {
	j := &ShellJob{
		Env: make(map[string]string),
		Base: Base{
			JobType: amboy.JobType{
				Name:    "shell",
				Version: 1,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// Run executes the shell commands. Add keys to the Env map to modify
// the environment, or change the value of the WorkingDir property to
// set the working directory for this command. Captures output into
// the Output attribute, and returns the error value of the command.
func (j *ShellJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	grip.Debugf("running %s", j.Command)

	args := strings.Split(j.Command, " ")

	j.mutex.RLock()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // nolint
	j.mutex.RUnlock()

	cmd.Dir = j.WorkingDir
	cmd.Env = j.getEnVars()

	output, err := cmd.CombinedOutput()
	j.AddError(err)

	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.Output = strings.TrimSpace(string(output))
}

func (j *ShellJob) getEnVars() []string {
	if len(j.Env) == 0 {
		return []string{}
	}

	output := []string{}
	for k, v := range j.Env {
		output = append(output, strings.Join([]string{k, v}, "="))
	}

	return output
}
