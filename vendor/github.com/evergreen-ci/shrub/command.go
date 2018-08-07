package shrub

import (
	"time"
)

type Command interface {
	Resolve() *CommandDefinition
	Validate() error
}

type CommandDefinition struct {
	FunctionName  string                 `json:"func,omitempty"`
	ExecutionType string                 `json:"type,omitempty"`
	DisplayName   string                 `json:"display_name,omitempty"`
	CommandName   string                 `json:"command,omitempty"`
	RunVariants   []string               `json:"variants,omitempty"`
	TimeoutSecs   int                    `json:"timeout_secs,omitempty"`
	Params        map[string]interface{} `json:"params,omitempty"`
	Vars          map[string]string      `json:"vars,omitempty"`
}

func (c *CommandDefinition) Validate() error                      { return nil }
func (c *CommandDefinition) Resolve() *CommandDefinition          { return c }
func (c *CommandDefinition) Function(n string) *CommandDefinition { c.FunctionName = n; return c }
func (c *CommandDefinition) Type(n string) *CommandDefinition     { c.ExecutionType = n; return c }
func (c *CommandDefinition) Name(n string) *CommandDefinition     { c.DisplayName = n; return c }
func (c *CommandDefinition) Command(n string) *CommandDefinition  { c.CommandName = n; return c }
func (c *CommandDefinition) Timeout(s time.Duration) *CommandDefinition {
	c.TimeoutSecs = int(s.Seconds())
	return c
}
func (c *CommandDefinition) Variants(vs ...string) *CommandDefinition {
	c.RunVariants = append(c.RunVariants, vs...)
	return c
}
func (c *CommandDefinition) ResetVars() *CommandDefinition                      { c.Vars = nil; return c }
func (c *CommandDefinition) ResetParams() *CommandDefinition                    { c.Params = nil; return c }
func (c *CommandDefinition) ReplaceVars(v map[string]string) *CommandDefinition { c.Vars = v; return c }
func (c *CommandDefinition) ReplaceParams(v map[string]interface{}) *CommandDefinition {
	c.Params = v
	return c
}

func (c *CommandDefinition) Param(k string, v interface{}) *CommandDefinition {
	if c.Params == nil {
		c.Params = make(map[string]interface{})
	}

	c.Params[k] = v

	return c
}

func (c *CommandDefinition) ExtendParams(p map[string]interface{}) *CommandDefinition {
	if c.Params == nil {
		c.Params = p
		return c
	}

	for k, v := range p {
		c.Params[k] = v
	}

	return c
}

func (c *CommandDefinition) Var(k, v string) *CommandDefinition {
	if c.Vars == nil {
		c.Vars = make(map[string]string)
	}

	c.Vars[k] = v
	return c
}

func (c *CommandDefinition) ExtendVars(vars map[string]string) *CommandDefinition {
	if c.Vars == nil {
		c.Vars = vars
		return c
	}

	for k, v := range vars {
		c.Vars[k] = v

	}

	return c
}

type CommandSequence []*CommandDefinition

func (s *CommandSequence) Len() int { return len(*s) }

func (s *CommandSequence) Command() *CommandDefinition {
	c := &CommandDefinition{}
	*s = append(*s, c)
	return c
}

func (s *CommandSequence) Append(c ...*CommandDefinition) *CommandSequence {
	*s = append(*s, c...)
	return s
}

func (s *CommandSequence) Add(cmd Command) *CommandSequence { *s = append(*s, cmd.Resolve()); return s }

func (s *CommandSequence) Extend(cmds ...Command) *CommandSequence {
	for _, cmd := range cmds {
		*s = append(*s, cmd.Resolve())
	}
	return s
}
