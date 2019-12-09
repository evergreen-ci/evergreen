package wire

import (
	"context"
	"io"
	"time"

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

func (s *service) managerID(ctx context.Context, w io.Writer, msg mongowire.Message) {
	resp, err := shell.ResponseToMessage(makeIDResponse(s.manager.ID()))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.New("could not make response"), ManagerIDCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ManagerIDCommand)
}

func (s *service) managerCreateProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := createProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not read request"), CreateProcessCommand)
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
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not create process"), CreateProcessCommand)
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
			shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not register trigger"), CreateProcessCommand)
			return
		}
	}

	resp, err := shell.ResponseToMessage(makeInfoResponse(getProcInfoNoHang(ctx, proc)))
	if err != nil {
		cancel()
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not make response"), CreateProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, CreateProcessCommand)
}

func (s *service) managerList(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := listRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not read request"), ListCommand)
		return
	}
	filter := req.Filter

	procs, err := s.manager.List(ctx, filter)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not list processes"), ListCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}
	resp, err := shell.ResponseToMessage(makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not make response"), ListCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ListCommand)
}

func (s *service) managerGroup(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := groupRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not read request"), GroupCommand)
		return
	}
	tag := req.Tag

	procs, err := s.manager.Group(ctx, tag)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not get process group"), GroupCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}

	resp, err := shell.ResponseToMessage(makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not make response"), GroupCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GroupCommand)
}

func (s *service) managerGetProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := getProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not read request"), GetProcessCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not get process"), GetProcessCommand)
		return
	}

	resp, err := shell.ResponseToMessage(makeInfoResponse(proc.Info(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not make response"), GetProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GetProcessCommand)
}

func (s *service) managerClear(ctx context.Context, w io.Writer, msg mongowire.Message) {
	s.manager.Clear(ctx)
	shell.WriteOKResponse(ctx, w, ClearCommand)
}

func (s *service) managerClose(ctx context.Context, w io.Writer, msg mongowire.Message) {
	if err := s.manager.Close(ctx); err != nil {
		shell.WriteErrorResponse(ctx, w, err, CloseCommand)
		return
	}
	shell.WriteOKResponse(ctx, w, CloseCommand)
}

func (s *service) managerWriteFile(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &writeFileRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "could not read request"), WriteFileCommand)
		return
	}
	opts := req.Options

	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "invalid write file options"), WriteFileCommand)
		return
	}
	if err := opts.DoWrite(); err != nil {
		shell.WriteErrorResponse(ctx, w, errors.Wrap(err, "failed to write to file"), WriteFileCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, WriteFileCommand)
}

func getProcInfoNoHang(ctx context.Context, p jasper.Process) jasper.ProcessInfo {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return p.Info(ctx)
}
