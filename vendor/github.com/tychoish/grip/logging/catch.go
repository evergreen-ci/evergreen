package logging

// Catch Logging
//
// Logging helpers for catching and logging error messages. Helpers exist
// for the following levels, with helpers defined both globally for the
// global logger and for Journaler logging objects.
import (
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

func (g *Grip) CatchLog(l level.Priority, err error) {
	g.Send(message.NewErrorMessage(l, err))
}

func (g *Grip) CatchDefault(err error) {
	g.Send(message.NewErrorMessage(g.DefaultLevel(), err))
}

func (g *Grip) CatchEmergency(err error) {
	g.Send(message.NewErrorMessage(level.Emergency, err))
}
func (g *Grip) CatchEmergencyPanic(err error) {
	g.sendPanic(message.NewErrorMessage(level.Emergency, err))
}
func (g *Grip) CatchEmergencyFatal(err error) {
	g.sendFatal(message.NewErrorMessage(level.Emergency, err))
}

func (g *Grip) CatchAlert(err error) {
	g.Send(message.NewErrorMessage(level.Alert, err))
}

func (g *Grip) CatchCritical(err error) {
	g.Send(message.NewErrorMessage(level.Critical, err))
}

func (g *Grip) CatchError(err error) {
	g.Send(message.NewErrorMessage(level.Error, err))
}

func (g *Grip) CatchWarning(err error) {
	g.Send(message.NewErrorMessage(level.Warning, err))
}

func (g *Grip) CatchNotice(err error) {
	g.Send(message.NewErrorMessage(level.Notice, err))
}

func (g *Grip) CatchInfo(err error) {
	g.Send(message.NewErrorMessage(level.Info, err))
}

func (g *Grip) CatchDebug(err error) {
	g.Send(message.NewErrorMessage(level.Debug, err))
}
