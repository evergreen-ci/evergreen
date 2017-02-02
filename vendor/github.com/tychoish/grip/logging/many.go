package logging

import (
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

///////////////////////////////////////////////////////////////////////////
//
// Multi Send

func (g *Grip) multiSend(l level.Priority, msgs []message.Composer) {
	for _, m := range msgs {
		_ = m.SetPriority(l)
		g.Send(m)
	}
}

func (g *Grip) LogMany(l level.Priority, msgs ...message.Composer) {
	g.multiSend(l, msgs)
}

func (g *Grip) DefaultMany(msgs ...message.Composer) {
	g.multiSend(g.Level().Default, msgs)
}

func (g *Grip) EmergencyMany(msgs ...message.Composer) {
	g.multiSend(level.Emergency, msgs)
}

func (g *Grip) AlertMany(msgs ...message.Composer) {
	g.multiSend(level.Alert, msgs)
}

func (g *Grip) CriticalMany(msgs ...message.Composer) {
	g.multiSend(level.Critical, msgs)
}

func (g *Grip) ErrorMany(msgs ...message.Composer) {
	g.multiSend(level.Critical, msgs)
}

func (g *Grip) WarningMany(msgs ...message.Composer) {
	g.multiSend(level.Warning, msgs)
}

func (g *Grip) NoticeMany(msgs ...message.Composer) {
	g.multiSend(level.Notice, msgs)
}

func (g *Grip) InfoMany(msgs ...message.Composer) {
	g.multiSend(level.Info, msgs)
}

func (g *Grip) DebugMany(msgs ...message.Composer) {
	g.multiSend(level.Debug, msgs)
}

///////////////////////////////////////////////////////////////////////////
//
// Conditional Multi Send

func (g *Grip) conditionalMultiSend(conditional bool, l level.Priority, msgs []message.Composer) {
	if !conditional {
		return
	}

	g.multiSend(l, msgs)
}

func (g *Grip) LogManyWhen(conditional bool, l level.Priority, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, l, msgs)
}

func (g *Grip) DefaultManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, g.Level().Default, msgs)
}

func (g *Grip) EmergencyManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Emergency, msgs)
}

func (g *Grip) AlertManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Alert, msgs)
}

func (g *Grip) CriticalManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Critical, msgs)
}

func (g *Grip) ErrorManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Critical, msgs)
}

func (g *Grip) WarningManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Warning, msgs)
}

func (g *Grip) NoticeManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Notice, msgs)
}

func (g *Grip) InfoManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Info, msgs)
}

func (g *Grip) DebugManyWhen(conditional bool, msgs ...message.Composer) {
	g.conditionalMultiSend(conditional, level.Debug, msgs)
}
