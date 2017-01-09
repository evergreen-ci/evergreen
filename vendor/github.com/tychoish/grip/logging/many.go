package logging

import (
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

// Internal helpers to manage sending interaction

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
