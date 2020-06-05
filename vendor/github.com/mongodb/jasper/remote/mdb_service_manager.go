package remote

import (
	"context"
	"io"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// Constants representing manager commands.
const (
	ManagerIDCommand     = "id"
	CreateProcessCommand = "create_process"
	GetProcessCommand    = "get_process"
	ListCommand          = "list"
	GroupCommand         = "group"
	ClearCommand         = "clear"
	CloseCommand         = "close"
	WriteFileCommand     = "write_file"
)

func (s *mdbService) managerID(ctx context.Context, w io.Writer, msg mongowire.Message) {
	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeIDResponse(s.manager.ID()))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.New("could not make response"), ManagerIDCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ManagerIDCommand)
}

func (s *mdbService) managerCreateProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := createProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), CreateProcessCommand)
		return
	}
	opts := req.Options

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())

	proc, err := s.manager.CreateProcess(pctx, &opts)
	if err != nil {
		cancel()
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not create process"), CreateProcessCommand)
		return
	}

	if err = proc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		info := getProcInfoNoHang(ctx, proc)
		cancel()
		// If we get an error registering a trigger, then we should make sure that
		// the reason for it isn't just because the process has exited already,
		// since that should not be considered an error.
		if !info.Complete {
			shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not register trigger"), CreateProcessCommand)
			return
		}
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(getProcInfoNoHang(ctx, proc)))
	if err != nil {
		cancel()
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), CreateProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, CreateProcessCommand)
}

func (s *mdbService) managerList(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := listRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), ListCommand)
		return
	}
	filter := req.Filter

	procs, err := s.manager.List(ctx, filter)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not list processes"), ListCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}
	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), ListCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ListCommand)
}

func (s *mdbService) managerGroup(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := groupRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GroupCommand)
		return
	}
	tag := req.Tag

	procs, err := s.manager.Group(ctx, tag)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process group"), GroupCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GroupCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GroupCommand)
}

func (s *mdbService) managerGetProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := getProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GetProcessCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), GetProcessCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(proc.Info(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GetProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GetProcessCommand)
}

func (s *mdbService) managerClear(ctx context.Context, w io.Writer, msg mongowire.Message) {
	s.manager.Clear(ctx)
	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, ClearCommand)
}

func (s *mdbService) managerClose(ctx context.Context, w io.Writer, msg mongowire.Message) {
	if err := s.manager.Close(ctx); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, CloseCommand)
		return
	}
	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, CloseCommand)
}

func (s *mdbService) managerWriteFile(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &writeFileRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), WriteFileCommand)
		return
	}
	opts := req.Options

	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid write file options"), WriteFileCommand)
		return
	}
	if err := opts.DoWrite(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "failed to write to file"), WriteFileCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, WriteFileCommand)
}
