/*
Package logging provides the primary implementation of the Journaler
interface (which is cloned in public functions in the grip interface
itself.)

Basic Logging

Loging helpers exist for the following levels:

   Emergency + (fatal/panic)
   Alert + (fatal/panic)
   Critical + (fatal/panic)
   Error + (fatal/panic)
   Warning
   Notice
   Info
   Debug
*/
package logging

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

func (g *Grip) Log(l level.Priority, msg interface{}) {
	g.send(message.ConvertToComposer(l, msg))
}
func (g *Grip) Logf(l level.Priority, msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(l, msg, a...))
}
func (g *Grip) Logln(l level.Priority, a ...interface{}) {
	g.send(message.NewLineMessage(l, a...))
}

func (g *Grip) Emergency(msg interface{}) {
	g.send(message.ConvertToComposer(level.Emergency, msg))
}
func (g *Grip) Emergencyf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Emergency, msg, a...))
}
func (g *Grip) Emergencyln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Emergency, a...))
}
func (g *Grip) EmergencyPanic(msg interface{}) {
	g.sendPanic(message.ConvertToComposer(level.Emergency, msg))
}
func (g *Grip) EmergencyPanicf(msg string, a ...interface{}) {
	g.sendPanic(message.NewFormattedMessage(level.Emergency, msg, a...))
}
func (g *Grip) EmergencyPanicln(a ...interface{}) {
	g.sendPanic(message.NewLineMessage(level.Emergency, a...))
}
func (g *Grip) EmergencyFatal(msg interface{}) {
	g.sendFatal(message.ConvertToComposer(level.Emergency, msg))
}
func (g *Grip) EmergencyFatalf(msg string, a ...interface{}) {
	g.sendFatal(message.NewFormattedMessage(level.Emergency, msg, a...))
}
func (g *Grip) EmergencyFatalln(a ...interface{}) {
	g.sendFatal(message.NewLineMessage(level.Emergency, a...))
}

func (g *Grip) Alert(msg interface{}) {
	g.send(message.ConvertToComposer(level.Alert, msg))
}
func (g *Grip) Alertf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Alert, msg, a...))
}
func (g *Grip) Alertln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Alert, a...))
}

func (g *Grip) Critical(msg interface{}) {
	g.send(message.ConvertToComposer(level.Critical, msg))
}
func (g *Grip) Criticalf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Critical, msg, a...))
}
func (g *Grip) Criticalln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Critical, a...))
}

func (g *Grip) Error(msg interface{}) {
	g.send(message.ConvertToComposer(level.Error, msg))
}
func (g *Grip) Errorf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Error, msg, a...))
}
func (g *Grip) Errorln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Error, a...))
}

func (g *Grip) Warning(msg interface{}) {
	g.send(message.ConvertToComposer(level.Warning, msg))
}
func (g *Grip) Warningf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Warning, msg, a...))
}
func (g *Grip) Warningln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Warning, a...))
}

func (g *Grip) Notice(msg interface{}) {
	g.send(message.ConvertToComposer(level.Notice, msg))
}
func (g *Grip) Noticef(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Notice, msg, a...))
}
func (g *Grip) Noticeln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Notice, a...))
}

func (g *Grip) Info(msg interface{}) {
	g.send(message.ConvertToComposer(level.Info, msg))
}
func (g *Grip) Infof(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Info, msg, a...))
}
func (g *Grip) Infoln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Info, a...))
}
func (g *Grip) Debug(msg interface{}) {
	g.send(message.ConvertToComposer(level.Debug, msg))
}
func (g *Grip) Debugf(msg string, a ...interface{}) {
	g.send(message.NewFormattedMessage(level.Debug, msg, a...))
}
func (g *Grip) Debugln(a ...interface{}) {
	g.send(message.NewLineMessage(level.Debug, a...))
}
