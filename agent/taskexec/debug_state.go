package taskexec

import (
	"github.com/evergreen-ci/evergreen/model"
)

// DebugState maintains the state of a debug session
type DebugState struct {
	CurrentStepIndex int
	LoadedProject    *model.Project
	SelectedTask     string
	SelectedVariant  string
	CommandList      []CommandInfo
	WorkingDir       string
}

// CommandInfo represents a single command in the flattened command list
type CommandInfo struct {
	Index        int
	Command      model.PluginCommandConf
	IsFunction   bool
	FunctionName string
	DisplayName  string
}

// NewDebugState creates a new debug state
func NewDebugState() *DebugState {
	return &DebugState{
		CurrentStepIndex: 0,
		CommandList:      []CommandInfo{},
	}
}
