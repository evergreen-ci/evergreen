package message

import "github.com/mongodb/grip/level"

type condComposer struct {
	cond bool
	msg  Composer
}

func When(cond bool, m interface{}) Composer {
	return &condComposer{cond: cond, msg: ConvertToComposer(level.Priority(0), m)}
}
func Whenf(cond bool, m string, args ...interface{}) Composer {
	return &condComposer{cond: cond, msg: NewFormatted(m, args...)}
}
func Whenln(cond bool, args ...interface{}) Composer {
	return &condComposer{cond: cond, msg: NewLine(args...)}
}
func WhenMsg(cond bool, m string) Composer {
	return &condComposer{cond: cond, msg: NewString(m)}
}
func (c *condComposer) String() string                     { return c.msg.String() }
func (c *condComposer) Raw() interface{}                   { return c.msg.Raw() }
func (c *condComposer) Priority() level.Priority           { return c.msg.Priority() }
func (c *condComposer) SetPriority(p level.Priority) error { return c.msg.SetPriority(p) }
func (c *condComposer) Loggable() bool {
	if c.cond {
		return c.msg.Loggable()
	}

	return false
}
