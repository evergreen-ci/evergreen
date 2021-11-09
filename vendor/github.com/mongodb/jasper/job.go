package jasper

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
)

// Job is an alias for an amboy.Job
type Job amboy.Job

type amboyJob struct {
	OverrideEnviron bool `bson:"override_env" json:"override_env" yaml:"override_env"`
	OutputOptions   struct {
		SuppressOutput    bool `bson:"suppress_output,omitempty" json:"suppress_output,omitempty" yaml:"suppress_output,omitempty"`
		SuppressError     bool `bson:"suppress_error,omitempty" json:"suppress_error,omitempty" yaml:"suppress_error,omitempty"`
		SendOutputToError bool `bson:"redirect_output_to_error,omitempty" json:"redirect_output_to_error,omitempty" yaml:"redirect_output_to_error,omitempty"`
		SendErrorToOutput bool `bson:"redirect_error_to_output,omitempty" json:"redirect_error_to_output,omitempty" yaml:"redirect_error_to_output,omitempty"`
	} `bson:"output_opts,omitempty" json:"output_opts,omitempty" yaml:"output_opts,omitempty"`
	ExitCode         int               `bson:"exit_code" json:"exit_code" yaml:"exit_code"`
	Environment      map[string]string `bson:"env" json:"env" yaml:"env"`
	CmdString        string            `bson:"cmd" json:"cmd" yaml:"cmd"`
	WorkingDirectory string            `bson:"working_dir" json:"working_dir" yaml:"working_dir"`
	Output           struct {
		Error  string `bson:"error" json:"error" yaml:"error"`
		Output string `bson:"output" json:"output" yaml:"output"`
	} `bson:"output" json:"output" yaml:"output"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	makep ProcessConstructor
}

const (
	amboyJobName                     = "jasper-shell-job"
	amboySimpleCapturedOutputJobName = "jasper-simple-shell-job"
	amboyForegroundOutputJobName     = "jasper-foreground-job"
)

// RegisterJobs adds factories for the job types provided by these
// packages to make it possible to dispatch these jobs to a
// remote/distributed queue.
func RegisterJobs(pc ProcessConstructor) {
	registry.AddJobType(amboyJobName, func() amboy.Job { return amboyJobFactory(pc) })
	registry.AddJobType(amboySimpleCapturedOutputJobName, func() amboy.Job { return amboySimpleCapturedOutputJobFactory(pc) })
	registry.AddJobType(amboyForegroundOutputJobName, func() amboy.Job { return amboyForegroundOutputJobFactory(pc) })
}

func amboyJobFactory(pc ProcessConstructor) *amboyJob {
	j := &amboyJob{
		makep:    pc,
		ExitCode: -1,
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    amboyJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j

}

// NewJob constructs an amboy job that wraps a provided
// ProcessConstructor. The identifier of the job includes a hash of
// the command, so running the same command repeatedly may result in
// job collisions.
//
// Pass the process constructor to allow the amboy jobs to manipulate
// processes in an existing Manager.
func NewJob(pc ProcessConstructor, cmd string) Job {
	j := amboyJobFactory(pc)
	j.CmdString = cmd
	j.SetID(fmt.Sprintf("%s.%x", amboyJobName, sha1.Sum([]byte(cmd))))
	return j
}

// NewJobBasic constructs an amboy job that uses jasper process
// management internally. The identifier of the job includes a hash of
// the command, so running the same command repeatedly may result in
// job collisions.
func NewJobBasic(cmd string) Job {
	j := amboyJobFactory(newBasicProcess)
	j.CmdString = cmd
	j.SetID(fmt.Sprintf("%s.basic.%x", amboyJobName, sha1.Sum([]byte(cmd))))
	return j
}

// NewJobExtended builds a job that creates a process with environment
// variables and a working directory defined. The identifier of the
// job includes a hash of the command, so running the same command
// repeatedly may result in job collisions.
//
// Pass the process constructor to allow the amboy jobs to manipulate
// processes in an existing Manager.
func NewJobExtended(pc ProcessConstructor, cmd string, env map[string]string, wd string) Job {
	j := amboyJobFactory(pc)
	j.CmdString = cmd
	j.Environment = env
	j.WorkingDirectory = wd
	j.SetID(fmt.Sprintf("%s.ext.%x", amboyJobName, sha1.Sum([]byte(cmd))))
	return j
}

// NewJobBasicExtended builds a job that creates a process with environment
// variables and a working directory defined. The identifier of the
// job includes a hash of the command, so running the same command
// repeatedly may result in job collisions.
func NewJobBasicExtended(cmd string, env map[string]string, wd string) Job {
	j := amboyJobFactory(newBasicProcess)
	j.CmdString = cmd
	j.Environment = env
	j.WorkingDirectory = wd
	j.SetID(fmt.Sprintf("%s.ext.%x", amboyJobName, sha1.Sum([]byte(cmd))))
	return j
}

func (j *amboyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.ExitCode >= 0 {
		j.AddError(errors.New("cannot run command more than once"))
		return
	}

	opts, err := options.MakeCreation(j.CmdString)
	if err != nil {
		j.AddError(err)
		return
	}

	opts.Environment = j.Environment
	opts.OverrideEnviron = j.OverrideEnviron
	opts.Output.SuppressError = j.OutputOptions.SuppressError
	opts.Output.SuppressOutput = j.OutputOptions.SuppressOutput
	opts.Output.SendOutputToError = j.OutputOptions.SendOutputToError
	opts.Output.SendErrorToOutput = j.OutputOptions.SendErrorToOutput

	output := &bytes.Buffer{}
	error := &bytes.Buffer{}
	opts.Output.Error = error
	opts.Output.Output = output

	p, err := j.makep(ctx, opts)
	if err != nil {
		j.AddError(err)
		return
	}
	exitCode, err := p.Wait(ctx)
	j.AddError(err)
	j.ExitCode = exitCode
	j.Output.Error = error.String()
	j.Output.Output = output.String()
}

type amboySimpleCapturedOutputJob struct {
	Options *options.Create `bson:"options" json:"options" yaml:"options"`
	Output  struct {
		Error  string `bson:"error," json:"error," yaml:"error,"`
		Output string `bson:"output" json:"output" yaml:"output"`
	} `bson:"output" json:"output" yaml:"output"`
	ExitCode int `bson:"exit_code" json:"exit_code" yaml:"exit_code"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	makep ProcessConstructor
}

func amboySimpleCapturedOutputJobFactory(pc ProcessConstructor) *amboySimpleCapturedOutputJob {
	j := &amboySimpleCapturedOutputJob{
		makep:    pc,
		ExitCode: -1,
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    amboySimpleCapturedOutputJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJobOptions creates a new job using the options to define the
// parameters of the job. The identifier of the job is the hash of the
// options structure.
//
// Pass the process constructor to allow the amboy jobs to manipulate
// processes in an existing Manager.
func NewJobOptions(pc ProcessConstructor, opts *options.Create) Job {
	j := amboySimpleCapturedOutputJobFactory(pc)
	j.Options = opts
	j.SetID(fmt.Sprintf("%s.%x", j.Type().Name, opts.Hash()))
	return j
}

func (j *amboySimpleCapturedOutputJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.ExitCode >= 0 {
		j.AddError(errors.New("cannot run command more than once"))
		return
	}

	output := &bytes.Buffer{}
	error := &bytes.Buffer{}
	j.Options.Output.Error = error
	j.Options.Output.Output = output

	p, err := j.makep(ctx, j.Options)
	if err != nil {
		j.AddError(err)
		return
	}
	exitCode, err := p.Wait(ctx)
	j.AddError(err)
	j.ExitCode = exitCode
	j.Output.Error = error.String()
	j.Output.Output = output.String()
}

type amboyForegroundOutputJob struct {
	Options  *options.Create `bson:"options" json:"options" yaml:"options"`
	ExitCode int             `bson:"exit_code" json:"exit_code" yaml:"exit_code"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	makep ProcessConstructor
}

func amboyForegroundOutputJobFactory(pc ProcessConstructor) *amboyForegroundOutputJob {
	j := &amboyForegroundOutputJob{
		ExitCode: -1,
		makep:    pc,
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    amboyForegroundOutputJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJobForeground creates an amboy job that writes all output
// linewise to the current processes global grip logging instance with
// error and output separated by level.
//
// Pass the process constructor to allow the amboy jobs to manipulate
// processes in an existing Manager.
func NewJobForeground(pc ProcessConstructor, opts *options.Create) Job {
	j := amboyForegroundOutputJobFactory(pc)
	j.SetID(fmt.Sprintf("%s.%x", j.Type().Name, opts.Hash()))
	j.Options = opts
	return j
}

// NewJobBasicForeground creates an amboy job that writes all output
// linewise to the current processes global grip logging instance with
// error and output separated by level.
func NewJobBasicForeground(opts *options.Create) Job {
	j := amboyForegroundOutputJobFactory(newBasicProcess)
	j.SetID(fmt.Sprintf("%s.basic.%x", j.Type().Name, opts.Hash()))
	j.Options = opts
	return j
}

func (j *amboyForegroundOutputJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.ExitCode >= 0 {
		j.AddError(errors.New("cannot run command more than once"))
		return
	}

	j.Options.Output.Error = send.MakeWriterSender(grip.GetSender(), level.Error)
	j.Options.Output.Output = send.MakeWriterSender(grip.GetSender(), level.Info)

	p, err := j.makep(ctx, j.Options)
	if err != nil {
		j.AddError(err)
		return
	}
	exitCode, err := p.Wait(ctx)
	j.AddError(err)
	j.ExitCode = exitCode
}
