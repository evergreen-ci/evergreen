package taskexec

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
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
	LastError        error
	ExecutionHistory []executionRecord
	ConfigPath       string
}

// executionRecord tracks the execution of a single command
type executionRecord struct {
	stepIndex  int
	success    bool
	durationMs int64
	errMsg     string
}

// GetStepExecution returns whether a step has been executed and if it succeeded.
func (ds *DebugState) GetStepExecution(index int) (executed, success bool) {
	for _, record := range ds.ExecutionHistory {
		if record.stepIndex == index {
			return true, record.success
		}
	}
	return false, false
}

// CommandInfo represents a single command in the linear execution order.
// Commands are flattened from their hierarchical structure (i.e. inside functions)
// into a sequential list for step-by-step execution.
type CommandInfo struct {
	Index            int
	Command          model.PluginCommandConf
	IsFunction       bool
	FunctionName     string
	DisplayName      string
	BlockType        command.BlockType
	BlockIndex       int
	BlockCmdNum      int
	BlockTotalCmds   int
	FuncSubCmdNum    int
	FuncTotalSubCmds int
}

func (ci CommandInfo) stepNumber() string {
	if ci.FuncSubCmdNum > 0 && ci.FuncTotalSubCmds > 1 {
		return fmt.Sprintf("%d.%d", ci.BlockCmdNum, ci.FuncSubCmdNum)
	}
	return fmt.Sprintf("%d", ci.BlockCmdNum)
}

// FullStepNumber returns the block-qualified step number string.
// For main block commands it returns just the step number (e.g. "5" or "5.3").
// For pre/post blocks it returns a qualified form (e.g. "pre:1.2").
func (ci CommandInfo) FullStepNumber() string {
	step := ci.stepNumber()
	if ci.BlockType != command.MainTaskBlock {
		return fmt.Sprintf("%s:%s", ci.BlockType, step)
	}
	return step
}

// ResolveStepNumber parses a step number string (e.g. "5", "5.3", "pre:1.2")
// and returns the flat index of the matching command in the CommandList.
func (ds *DebugState) ResolveStepNumber(stepNum string) (int, error) {
	for i, cmd := range ds.CommandList {
		if cmd.FullStepNumber() == stepNum {
			return i, nil
		}
	}
	return -1, errors.Errorf("step number '%s' not found", stepNum)
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
