package remote

import (
	"context"
	"io"
	"syscall"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// Constants representing process commands.
const (
	ProcessIDCommand               = "process_id"
	InfoCommand                    = "info"
	RunningCommand                 = "running"
	CompleteCommand                = "complete"
	WaitCommand                    = "wait"
	RespawnCommand                 = "respawn"
	SignalCommand                  = "signal"
	RegisterSignalTriggerIDCommand = "register_signal_trigger_id"
	GetTagsCommand                 = "get_tags"
	TagCommand                     = "add_tag"
	ResetTagsCommand               = "reset_tags"
)

func (s *service) processInfo(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := infoRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), InfoCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), InfoCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(proc.Info(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), InfoCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, InfoCommand)
}

func (s *service) processRunning(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := runningRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), RunningCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), RunningCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeRunningResponse(proc.Running(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), RunningCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, RunningCommand)
}

func (s *service) processComplete(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &completeRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), CompleteCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), CompleteCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeCompleteResponse(proc.Complete(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), CompleteCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, CompleteCommand)
}

func (s *service) processWait(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := waitRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), WaitCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), WaitCommand)
		return
	}

	exitCode, err := proc.Wait(ctx)
	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeWaitResponse(exitCode, err))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), WaitCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, WaitCommand)
}

func (s *service) processRespawn(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := respawnRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), RespawnCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), RespawnCommand)
		return
	}

	pctx, cancel := context.WithCancel(context.Background())
	newProc, err := proc.Respawn(pctx)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "failed to respawned process"), RespawnCommand)
		cancel()
		return
	}
	if err = s.manager.Register(ctx, newProc); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "failed to register respawned process"), RespawnCommand)
		cancel()
		return
	}

	if err = newProc.RegisterTrigger(ctx, func(jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		newProcInfo := getProcInfoNoHang(ctx, newProc)
		cancel()
		if !newProcInfo.Complete {
			shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "failed to register trigger on respawned process"), RespawnCommand)
			return
		}
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(newProc.Info(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), RespawnCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, RespawnCommand)
}

func (s *service) processSignal(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := signalRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), SignalCommand)
		return
	}
	id := req.Params.ID
	sig := req.Params.Signal

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), SignalCommand)
		return
	}

	if err := proc.Signal(ctx, syscall.Signal(sig)); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not signal process"), SignalCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, SignalCommand)
}

func (s *service) processRegisterSignalTriggerID(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := registerSignalTriggerIDRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), RegisterSignalTriggerIDCommand)
		return
	}
	procID := req.Params.ID
	sigID := req.Params.SignalTriggerID

	makeTrigger, ok := jasper.GetSignalTriggerFactory(sigID)
	if !ok {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Errorf("could not get signal trigger ID %s", sigID), RegisterSignalTriggerIDCommand)
		return
	}

	proc, err := s.manager.Get(ctx, procID)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), RegisterSignalTriggerIDCommand)
		return
	}

	if err := proc.RegisterSignalTrigger(ctx, makeTrigger()); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not register signal trigger"), RegisterSignalTriggerIDCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, RegisterSignalTriggerIDCommand)
}

func (s *service) processTag(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := tagRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), TagCommand)
		return
	}
	id := req.Params.ID
	tag := req.Params.Tag

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), TagCommand)
		return
	}

	proc.Tag(tag)

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, TagCommand)
}

func (s *service) processGetTags(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &getTagsRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GetTagsCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), GetTagsCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeGetTagsResponse(proc.GetTags()))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GetTagsCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GetTagsCommand)
}

func (s *service) processResetTags(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := resetTagsRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), ResetTagsCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), ResetTagsCommand)
		return
	}

	proc.ResetTags()

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, ResetTagsCommand)
}
