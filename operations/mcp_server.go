package operations

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	restclient "github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	perrors "github.com/pkg/errors"
	"github.com/urfave/cli"
)

// CLI flags specific to MCP server
const (
	mcpStdioFlagName   = "stdio"
	mcpPortFlagName    = "port"
	mcpPatchFlagName   = "patch"
	mcpProjectFlagName = "project"
	mcpMaxPatchesFlag  = "max-patches"
)

// TaskSummary represents a light view of a task for MCP responses
type TaskSummary struct {
	ID        string `json:"id"`
	Display   string `json:"display_name"`
	Status    string `json:"status"`
	Variant   string `json:"variant"`
	Execution int    `json:"execution"`
}

// MCPServer returns the CLI command definition for the Model Context Protocol server
func MCPServer() cli.Command {
	flags := []cli.Flag{
		cli.BoolFlag{
			Name:  mcpStdioFlagName,
			Usage: "run MCP server over stdio (JSON-RPC 2.0, newline-delimited)",
		},
		cli.IntFlag{
			Name:  mcpPortFlagName,
			Usage: "run MCP server listening on TCP port (not yet implemented)",
			Value: 0,
		},
		cli.StringFlag{
			Name:  mcpPatchFlagName,
			Usage: "explicit patch ID to use in server session",
		},
		cli.StringFlag{
			Name:  mcpProjectFlagName,
			Usage: "project identifier to narrow patch detection",
		},
		cli.IntFlag{
			Name:  mcpMaxPatchesFlag,
			Usage: "max recent patches to consider in detection heuristic",
			Value: 10,
		},
	}

	return cli.Command{
		Name:  "mcp-server",
		Usage: "expose Evergreen patch/task helpers over the Model Context Protocol",
		Flags: flags,
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(c.GlobalString(confFlagName))
			if err != nil {
				return perrors.Wrap(err, "loading configuration")
			}

			rest, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return perrors.Wrap(err, "setting up REST communicator")
			}
			defer rest.Close()

			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return perrors.Wrap(err, "setting up legacy Evergreen client")
			}

			server := &mcpServer{
				conf:        conf,
				rest:        rest,
				ac:          ac,
				rc:          rc,
				patchHint:   c.String(mcpPatchFlagName),
				projectHint: c.String(mcpProjectFlagName),
				maxPatches:  c.Int(mcpMaxPatchesFlag),
			}

			if c.Bool(mcpStdioFlagName) || c.Int(mcpPortFlagName) == 0 {
				grip.Notice("Starting MCP server on stdio (JSON-RPC 2.0, newline-delimited)")
				return server.runStdio(ctx, os.Stdin, os.Stdout)
			}
			return errors.New("TCP transport not yet implemented; use --stdio")
		},
	}
}

type mcpServer struct {
	conf        *ClientSettings
	rest        restclient.Communicator
	ac          *legacyClient
	rc          *legacyClient
	patchHint   string
	projectHint string
	maxPatches  int
}

// JSON-RPC structures

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (s *mcpServer) runStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Increase the buffer to accommodate larger requests
	buf := make([]byte, 0, 1024*64)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		// ignore empty lines
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeRPC(out, rpcResponse{JSONRPC: "2.0", ID: nullID(), Error: &rpcError{Code: -32700, Message: "parse error", Data: err.Error()}})
			continue
		}
		resp := s.dispatch(ctx, &req)
		writeRPC(out, resp)
	}
	return scanner.Err()
}

func nullID() json.RawMessage { return json.RawMessage("null") }

func writeRPC(w io.Writer, resp rpcResponse) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(resp)
}

func (s *mcpServer) dispatch(ctx context.Context, req *rpcRequest) rpcResponse {
	if req.JSONRPC != "2.0" {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32600, Message: "invalid request: jsonrpc must be '2.0'"}}
	}
	switch req.Method {
	case "detectPatchId":
		var p struct {
			Project string `json:"project"`
			Branch  string `json:"branch"`
			PatchId string `json:"patchId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		patchID, reason, err := s.detectPatchID(ctx, detectOpts{PatchFlag: p.PatchId, Project: p.Project, MaxPatches: s.maxPatches})
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "detectPatchId error", Data: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"patchId": patchID, "reason": reason}}

	case "listPatchTasks":
		var p struct {
			PatchId    string `json:"patchId"`
			FailedOnly bool   `json:"failedOnly"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.PatchId == "" {
			var err error
			p.PatchId, _, err = s.detectPatchID(ctx, detectOpts{PatchFlag: s.patchHint, Project: s.projectHint, MaxPatches: s.maxPatches})
			if err != nil {
				return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "could not resolve patch id", Data: err.Error()}}
			}
		}
		tasks, err := s.listTasksForPatch(ctx, p.PatchId, p.FailedOnly)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "listPatchTasks error", Data: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: tasks}

	case "getTaskLogs":
		var p struct {
			TaskId    string `json:"taskId"`
			Execution *int   `json:"execution"`
			Type      string `json:"type"`
			Tail      int    `json:"tail"`
		}
		_ = json.Unmarshal(req.Params, &p)
		content, err := s.getTaskLogs(ctx, p.TaskId, p.Execution, p.Type, p.Tail)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "getTaskLogs error", Data: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"content": content}}

	case "fetchFailedLogsForPatch":
		var p struct {
			PatchId string `json:"patchId"`
			Tail    int    `json:"tail"`
		}
		_ = json.Unmarshal(req.Params, &p)
		patchID := p.PatchId
		if patchID == "" {
			var reason string
			var err error
			patchID, reason, err = s.detectPatchID(ctx, detectOpts{PatchFlag: s.patchHint, Project: s.projectHint, MaxPatches: s.maxPatches})
			if err != nil {
				return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "could not resolve patch id", Data: err.Error()}}
			}
			_ = reason // not currently returned here
		}
		failedTasks, err := s.listTasksForPatch(ctx, patchID, true)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "listing failed tasks", Data: err.Error()}}
		}
		// Fetch logs concurrently with a bound.
		logs := make(map[string]string)
		type item struct {
			id   string
			exec int
		}
		jobs := make(chan item)
		wg := sync.WaitGroup{}
		mu := sync.Mutex{}
		workers := 4
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for it := range jobs {
					content, err := s.getTaskLogs(ctx, it.id, utility.ToIntPtr(it.exec), "all_logs", p.Tail)
					mu.Lock()
					if err != nil {
						logs[it.id] = fmt.Sprintf("error: %v", err)
					} else {
						logs[it.id] = content
					}
					mu.Unlock()
				}
			}()
		}
		for _, t := range failedTasks {
			jobs <- item{id: t.ID, exec: t.Execution}
		}
		close(jobs)
		wg.Wait()
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"logs": logs}}
	default:
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

// Helper: detectPatchID

type detectOpts struct {
	PatchFlag  string
	Project    string
	MaxPatches int
}

func (s *mcpServer) detectPatchID(ctx context.Context, opts detectOpts) (string, string, error) {
	if opts.PatchFlag != "" {
		return opts.PatchFlag, "provided by flag", nil
	}
	if env := strings.TrimSpace(os.Getenv("EVG_PATCH_ID")); env != "" {
		return env, "from EVG_PATCH_ID", nil
	}
	maxN := opts.MaxPatches
	if maxN <= 0 {
		maxN = 10
	}
	patches, err := s.ac.GetPatches(maxN)
	if err != nil {
		return "", "", err
	}
	if len(patches) == 0 {
		return "", "", errors.New("no recent patches found")
	}
	if opts.Project != "" {
		for _, p := range patches {
			if p.Project == opts.Project {
				return p.Id.Hex(), "recent patch for project", nil
			}
		}
	}
	// default to most recent
	return patches[0].Id.Hex(), "most recent patch", nil
}

// Helper: list tasks for a patch version, optionally filter by failed-only
func (s *mcpServer) listTasksForPatch(ctx context.Context, patchID string, failedOnly bool) ([]TaskSummary, error) {
	if patchID == "" {
		return nil, errors.New("patch id must be provided")
	}
	// Use legacy client to get patch with Version
	restPatch, err := s.ac.GetRestPatch(patchID)
	if err != nil {
		return nil, perrors.Wrap(err, "getting patch")
	}
	versionID := restPatch.Version
	if versionID == "" {
		return nil, errors.New("patch has no version id (not finalized?)")
	}
	builds, err := s.rest.GetBuildsForVersion(ctx, versionID)
	if err != nil {
		return nil, perrors.Wrap(err, "listing builds for version")
	}
	out := []TaskSummary{}
	for _, b := range builds {
		buildID := utility.FromStringPtr(b.Id)
		if buildID == "" {
			continue
		}
		tasks, err := s.rest.GetTasksForBuild(ctx, buildID)
		if err != nil {
			return nil, perrors.Wrapf(err, "listing tasks for build %s", buildID)
		}
		for _, t := range tasks {
			status := utility.FromStringPtr(t.Status)
			if failedOnly && status != evergreen.TaskFailed {
				continue
			}
			out = append(out, TaskSummary{
				ID:        utility.FromStringPtr(t.Id),
				Display:   utility.FromStringPtr(t.DisplayName),
				Status:    status,
				Variant:   utility.FromStringPtr(t.BuildVariant),
				Execution: t.Execution,
			})
		}
	}
	return out, nil
}

// Helper: get task logs content as a string
func (s *mcpServer) getTaskLogs(ctx context.Context, taskID string, execution *int, logType string, tail int) (string, error) {
	if taskID == "" {
		return "", errors.New("taskId is required")
	}
	if logType == "" {
		logType = "all_logs"
	}
	r, err := s.rest.GetTaskLogs(ctx, restclient.GetTaskLogsOptions{
		TaskID:        taskID,
		Execution:     execution,
		Type:          logType,
		TailLimit:     tail,
		PrintTime:     false,
		PrintPriority: false,
		Paginate:      false,
	})
	if err != nil {
		return "", err
	}
	defer r.Close()
	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Utility to convert possibly numeric JSON id to a string for logging (not used in payloads but handy for debug)
func idToString(id json.RawMessage) string {
	if len(id) == 0 || string(id) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(id, &s); err == nil {
		return s
	}
	var n int64
	if err := json.Unmarshal(id, &n); err == nil {
		return strconv.FormatInt(n, 10)
	}
	return string(id)
}
