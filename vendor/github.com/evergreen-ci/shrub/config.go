// Package shrub provides a simple, low-overhead interface for
// generating Evergreen project configurations.
//
// // In general, you can either implement  your config using either
// direct declarative constructors for all objects or the mentions
// which provide a more user-friendly interface. When you have a
// complete configuration, simply use json.Marshal() to serialize the
// config and write the contents to a file or a web request.
//
// Be aware that some command methods will panic if you attempt to
// construct an invalid command. This allows nearly all methods in
// this interface to be chain-able (e.g. fluent) without
// requiring excessive error handling. You can use the SafeBuilder
// function which will convert a panic into a an error.
package shrub

import (
	"fmt"
	"time"
)

// Configuration is the top-level representation of the components of
// an evergreen project configuration.
type Configuration struct {
	Functions map[string]*CommandSequence `json:"functions,omitempty" yaml:"functions,omitempty"`
	Tasks     []*Task                     `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Groups    []*TaskGroup                `json:"task_groups,omitempty" yaml:"task_groups,omitempty"`
	Variants  []*Variant                  `json:"buildvariants,omitempty" yaml:"buildvariants,omitempty"`
	Pre       *CommandSequence            `json:"pre,omitempty" yaml:"pre,omitempty"`
	Post      *CommandSequence            `json:"post,omitempty" yaml:"post,omitempty"`
	Timeout   *CommandSequence            `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Top Level Options
	ExecTimeoutSecs int      `json:"exec_timeout_secs,omitempty" yaml:"exec_timeout_secs,omitempty"`
	BatchTimeSecs   int      `json:"batchtime,omitempty" yaml:"batchtime,omitempty"`
	Stepback        bool     `json:"stepback,omitempty" yaml:"stepback,omitempty"`
	CommandType     string   `json:"command_type,omitempty" yaml:"command_type,omitempty"`
	IgnoreFiles     []string `json:"ignore,omitempty" yaml:"ignore,omitempty"`
}

// Task returns a task of the specified name. If the task already
// exists, then it returns the existing task of that name, and
// otherwise returns a new task of the specified name.
func (c *Configuration) Task(name string) *Task {
	for _, t := range c.Tasks {
		if t.Name == name {
			return t
		}
	}

	t := new(Task)
	t.Name = name
	c.Tasks = append(c.Tasks, t)
	return t
}

// TaskGroup returns a task group configuration of the specified
// name. If the taskgroup already exists, then it returns the existing
// task group of that name, and otherwise returns a new task group of
// the specified name.
func (c *Configuration) TaskGroup(name string) *TaskGroup {
	for _, g := range c.Groups {
		if g.GroupName == name {
			return g
		}
	}

	g := new(TaskGroup)
	c.Groups = append(c.Groups, g)
	return g.Name(name)
}

// Function creates a new function of the specific name and returns a
// CommandSequence builder for use in adding commands to the function.
func (c *Configuration) Function(name string) *CommandSequence {
	if c.Functions == nil {
		c.Functions = make(map[string]*CommandSequence)
	}

	seq, ok := c.Functions[name]
	if ok {
		return seq
	}

	seq = new(CommandSequence)
	c.Functions[name] = seq
	return seq
}

// Variant returns a build variant of the specified name. If the
// variant already exists, then it returns the existing variant of
// that name, and otherwise returns a new variant of the specified
// name.
func (c *Configuration) Variant(id string) *Variant {
	for _, v := range c.Variants {
		if v.BuildName == id {
			return v
		}
	}

	v := new(Variant)
	c.Variants = append(c.Variants, v)
	return v.Name(id)
}

////////////////////////////////////////////////////////////////////////
//
// Highlevel project-wide configuration settings.

// ExectTimeout allows you to set the exec timeout for all commands to
// a specified value. This value has second-level granularity.
func (c *Configuration) ExecTimeout(dur time.Duration) *Configuration {
	c.ExecTimeoutSecs = int(dur.Seconds())
	return c
}

func (c *Configuration) BatchTime(dur time.Duration) *Configuration {
	c.BatchTimeSecs = int(dur.Seconds())
	return c
}

func (c *Configuration) SetCommandType(t string) *Configuration {
	switch t {
	case "system", "setup", "task":
		c.CommandType = t
	default:
		panic(fmt.Sprintf("%s, is not a valid command type", t))
	}

	return c
}
