package operations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	restclient "github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mongodb/grip"
	perrors "github.com/pkg/errors"
	"github.com/urfave/cli"
)

// MCPServerSDK returns the CLI command definition for the Model Context Protocol server using the official SDK
func MCPServerSDK() cli.Command {
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
		Name:  "mcp-server-sdk",
		Usage: "expose Evergreen patch/task helpers over the Model Context Protocol using official SDK",
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

			// Create the MCP server using the official SDK
			server := mcp.NewServer(&mcp.Implementation{
				Name:    "evergreen-mcp-server",
				Version: "1.0.0",
			}, nil)

			// Create our Evergreen service wrapper
			evgService := &evergreenService{
				conf:        conf,
				rest:        rest,
				ac:          ac,
				rc:          rc,
				patchHint:   c.String(mcpPatchFlagName),
				projectHint: c.String(mcpProjectFlagName),
				maxPatches:  c.Int(mcpMaxPatchesFlag),
			}

			// Register tools
			if err := registerEvergreenTools(server, evgService); err != nil {
				return perrors.Wrap(err, "registering MCP tools")
			}

			if c.Bool(mcpStdioFlagName) || c.Int(mcpPortFlagName) == 0 {
				grip.Notice("Starting MCP server using official SDK on stdio")
				transport := mcp.NewStdioTransport()
				return server.Run(ctx, transport)
			}
			return errors.New("TCP transport not yet implemented; use --stdio")
		},
	}
}

// evergreenService wraps the Evergreen clients and configuration
type evergreenService struct {
	conf        *ClientSettings
	rest        restclient.Communicator
	ac          *legacyClient
	rc          *legacyClient
	patchHint   string
	projectHint string
	maxPatches  int
}

// Tool argument types for type safety
type DetectPatchIdArgs struct {
	Project string `json:"project,omitempty"`
	Branch  string `json:"branch,omitempty"`
	PatchId string `json:"patchId,omitempty"`
}

type ListPatchTasksArgs struct {
	PatchId    string `json:"patchId,omitempty"`
	FailedOnly bool   `json:"failedOnly,omitempty"`
}

type GetTaskLogsArgs struct {
	TaskId    string `json:"taskId"`
	Execution *int   `json:"execution,omitempty"`
	Type      string `json:"type,omitempty"`
	Tail      int    `json:"tail,omitempty"`
}

type FetchFailedLogsForPatchArgs struct {
	PatchId string `json:"patchId,omitempty"`
	Tail    int    `json:"tail,omitempty"`
}

// registerEvergreenTools registers all the Evergreen MCP tools
func registerEvergreenTools(server *mcp.Server, evg *evergreenService) error {
	// Register detectPatchId tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "detectPatchId",
		Description: "Automatically detect the current patch ID using precedence: explicit patch parameter, EVG_PATCH_ID environment variable, or most recent patch",
	}, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[DetectPatchIdArgs]) (*mcp.CallToolResultFor[any], error) {
		patchID, reason, err := evg.detectPatchID(ctx, detectOpts{
			PatchFlag:  params.Arguments.PatchId,
			Project:    params.Arguments.Project,
			MaxPatches: evg.maxPatches,
		})
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error detecting patch ID: %v", err)}},
			}, nil
		}

		result := map[string]any{
			"patchId": patchID,
			"reason":  reason,
		}
		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Detected patch ID: %s (reason: %s)", patchID, reason)}},
			Meta:    result,
		}, nil
	})

	// Register listPatchTasks tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "listPatchTasks",
		Description: "List tasks for a patch, with optional filtering for failed tasks only",
	}, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[ListPatchTasksArgs]) (*mcp.CallToolResultFor[any], error) {
		patchID := params.Arguments.PatchId
		if patchID == "" {
			var err error
			patchID, _, err = evg.detectPatchID(ctx, detectOpts{
				PatchFlag:  evg.patchHint,
				Project:    evg.projectHint,
				MaxPatches: evg.maxPatches,
			})
			if err != nil {
				return &mcp.CallToolResultFor[any]{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Could not resolve patch ID: %v", err)}},
				}, nil
			}
		}

		tasks, err := evg.listTasksForPatch(ctx, patchID, params.Arguments.FailedOnly)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error listing tasks: %v", err)}},
			}, nil
		}

		var content strings.Builder
		content.WriteString(fmt.Sprintf("Found %d tasks for patch %s:\n", len(tasks), patchID))
		for _, task := range tasks {
			content.WriteString(fmt.Sprintf("- %s (%s) - %s on %s\n", task.Display, task.ID, task.Status, task.Variant))
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: content.String()}},
			Meta:    map[string]any{"tasks": tasks, "patchId": patchID},
		}, nil
	})

	// Register getTaskLogs tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "getTaskLogs",
		Description: "Get logs for a specific task",
	}, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GetTaskLogsArgs]) (*mcp.CallToolResultFor[any], error) {
		content, err := evg.getTaskLogs(ctx, params.Arguments.TaskId, params.Arguments.Execution, params.Arguments.Type, params.Arguments.Tail)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error getting task logs: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: content}},
			Meta:    map[string]any{"taskId": params.Arguments.TaskId, "logLength": len(content)},
		}, nil
	})

	// Register fetchFailedLogsForPatch tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetchFailedLogsForPatch",
		Description: "Fetch logs for all failed tasks in a patch",
	}, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[FetchFailedLogsForPatchArgs]) (*mcp.CallToolResultFor[any], error) {
		patchID := params.Arguments.PatchId
		if patchID == "" {
			var err error
			patchID, _, err = evg.detectPatchID(ctx, detectOpts{
				PatchFlag:  evg.patchHint,
				Project:    evg.projectHint,
				MaxPatches: evg.maxPatches,
			})
			if err != nil {
				return &mcp.CallToolResultFor[any]{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Could not resolve patch ID: %v", err)}},
				}, nil
			}
		}

		failedTasks, err := evg.listTasksForPatch(ctx, patchID, true)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error listing failed tasks: %v", err)}},
			}, nil
		}

		// Fetch logs concurrently with a bound
		logs := make(map[string]string)
		type item struct {
			id   string
			exec int
		}
		jobs := make(chan item, len(failedTasks))
		wg := sync.WaitGroup{}
		mu := sync.Mutex{}
		workers := 4

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for it := range jobs {
					content, err := evg.getTaskLogs(ctx, it.id, utility.ToIntPtr(it.exec), "all_logs", params.Arguments.Tail)
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

		var content strings.Builder
		content.WriteString(fmt.Sprintf("Fetched logs for %d failed tasks in patch %s:\n\n", len(logs), patchID))
		for taskID, logContent := range logs {
			content.WriteString(fmt.Sprintf("=== Task %s ===\n%s\n\n", taskID, logContent))
		}

		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: content.String()}},
			Meta:    map[string]any{"logs": logs, "patchId": patchID, "taskCount": len(logs)},
		}, nil
	})

	return nil
}

// Helper methods - reusing the same logic from the original implementation

// detectPatchID detects the patch ID using the same logic as the original implementation
func (evg *evergreenService) detectPatchID(ctx context.Context, opts detectOpts) (string, string, error) {
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
	patches, err := evg.ac.GetPatches(maxN)
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

// listTasksForPatch lists tasks for a patch version, optionally filtering by failed-only
func (evg *evergreenService) listTasksForPatch(ctx context.Context, patchID string, failedOnly bool) ([]TaskSummary, error) {
	if patchID == "" {
		return nil, errors.New("patch id must be provided")
	}
	// Use legacy client to get patch with Version
	restPatch, err := evg.ac.GetRestPatch(patchID)
	if err != nil {
		return nil, perrors.Wrap(err, "getting patch")
	}
	versionID := restPatch.Version
	if versionID == "" {
		return nil, errors.New("patch has no version id (not finalized?)")
	}
	builds, err := evg.rest.GetBuildsForVersion(ctx, versionID)
	if err != nil {
		return nil, perrors.Wrap(err, "listing builds for version")
	}
	out := []TaskSummary{}
	for _, b := range builds {
		buildID := utility.FromStringPtr(b.Id)
		if buildID == "" {
			continue
		}
		tasks, err := evg.rest.GetTasksForBuild(ctx, buildID)
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

// getTaskLogs gets task logs content as a string
func (evg *evergreenService) getTaskLogs(ctx context.Context, taskID string, execution *int, logType string, tail int) (string, error) {
	if taskID == "" {
		return "", errors.New("taskId is required")
	}
	if logType == "" {
		logType = "all_logs"
	}
	r, err := evg.rest.GetTaskLogs(ctx, restclient.GetTaskLogsOptions{
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
