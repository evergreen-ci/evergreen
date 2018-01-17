package logging

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

/////////////

func (g *Grip) LogWhen(conditional bool, l level.Priority, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(l, m)))
}
func (g *Grip) LogWhenln(conditional bool, l level.Priority, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(l, msg...)))
}
func (g *Grip) LogWhenf(conditional bool, l level.Priority, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(l, msg, args...)))
}

/////////////

func (g *Grip) EmergencyWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Emergency, m)))
}
func (g *Grip) EmergencyWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Emergency, msg...)))
}
func (g *Grip) EmergencyWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Emergency, msg, args...)))
}

/////////////

func (g *Grip) AlertWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Alert, m)))
}
func (g *Grip) AlertWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Alert, msg...)))
}
func (g *Grip) AlertWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Alert, msg, args...)))
}

/////////////

func (g *Grip) CriticalWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Critical, m)))
}
func (g *Grip) CriticalWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Critical, msg...)))
}
func (g *Grip) CriticalWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Critical, msg, args...)))
}

/////////////

func (g *Grip) ErrorWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Error, m)))
}
func (g *Grip) ErrorWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Error, msg...)))
}
func (g *Grip) ErrorWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Error, msg, args...)))
}

/////////////

func (g *Grip) WarningWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Warning, m)))
}
func (g *Grip) WarningWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Warning, msg...)))
}
func (g *Grip) WarningWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Warning, msg, args...)))
}

/////////////

func (g *Grip) NoticeWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Notice, m)))
}
func (g *Grip) NoticeWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Notice, msg...)))
}
func (g *Grip) NoticeWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Notice, msg, args...)))
}

/////////////

func (g *Grip) InfoWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Info, m)))
}
func (g *Grip) InfoWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Info, msg...)))
}
func (g *Grip) InfoWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Info, msg, args...)))
}

/////////////

func (g *Grip) DebugWhen(conditional bool, m interface{}) {
	g.send(message.When(conditional, message.ConvertToComposer(level.Debug, m)))
}
func (g *Grip) DebugWhenln(conditional bool, msg ...interface{}) {
	g.send(message.When(conditional, message.NewLineMessage(level.Debug, msg...)))
}
func (g *Grip) DebugWhenf(conditional bool, msg string, args ...interface{}) {
	g.send(message.When(conditional, message.NewFormattedMessage(level.Debug, msg, args...)))
}
