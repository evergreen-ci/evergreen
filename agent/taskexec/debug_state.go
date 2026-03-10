package taskexec

import (
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/model"
)

// SetupPhaseStatus tracks the outcome of the setup phase.
type SetupPhaseStatus struct {
	Completed     bool
	TotalSteps    int
	FailedAtStep  int
	FailureCount  int
	FailureReason string
}

// DebugState maintains the state of a debug session.
type DebugState struct {
	CurrentStepIndex int
	LoadedProject    *model.Project
	SelectedTask     string
	SelectedVariant  string
	CustomVars       map[string]string
	CommandList      []CommandInfo
	WorkingDir       string
	LastError        error
	ExecutionHistory []executionRecord
	ConfigPath       string
	SetupStatus      SetupPhaseStatus
}

// executionRecord tracks the execution of a single command
type executionRecord struct {
	StepIndex  int
	Success    bool
	DurationMs int64
	Error      string
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
		ExecutionHistory: []executionRecord{},
	}
}

func (ds *DebugState) addExecutionRecord(record executionRecord) {
	ds.ExecutionHistory = append(ds.ExecutionHistory, record)
}

// HasMoreSteps returns true if there are more steps to execute
func (ds *DebugState) HasMoreSteps() bool {
	return ds.CurrentStepIndex < len(ds.CommandList)
}
