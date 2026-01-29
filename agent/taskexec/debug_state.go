package taskexec

import (
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/model"
)

// DebugState maintains the state of a debug session.
type DebugState struct {
	CurrentStepIndex int
	LoadedProject    *model.Project
	SelectedTask     string
	SelectedVariant  string
	CustomVars       map[string]string
	CommandList      []CommandInfo
	WorkingDir       string
}

// CommandInfo represents a single command in the linear execution order.
// Commands are flattened from their hierarchical structure (i.e. inside functions)
// into a sequential list for step-by-step execution.
type CommandInfo struct {
	Index        int
	Command      model.PluginCommandConf
	IsFunction   bool
	FunctionName string
	DisplayName  string
	BlockType    command.BlockType
	BlockIndex   int
	BlockCmdNum  int
}

// NewDebugState creates a new debug state.
func NewDebugState() *DebugState {
	return &DebugState{
		CurrentStepIndex: 0,
		CustomVars:       make(map[string]string),
		CommandList:      []CommandInfo{},
	}
}

// HasMoreSteps returns true if there are more steps to execute
func (ds *DebugState) HasMoreSteps() bool {
	return ds.CurrentStepIndex < len(ds.CommandList)
}
