package model

import (
	"fmt"

	"github.com/pkg/errors"
)

// TaskExecutionStep represents a single step in a task's execution plan.
// This replicates the step-building logic from agent/taskexec without importing agent packages.
type TaskExecutionStep struct {
	StepNumber   string `json:"step_number"`
	DisplayName  string `json:"display_name"`
	CommandName  string `json:"command_name"`
	IsFunction   bool   `json:"is_function"`
	FunctionName string `json:"function_name"`
	BlockType    string `json:"block_type"`
}

// GetTaskExecutionSteps builds a flat list of execution steps for a task,
// mirroring the logic in agent/taskexec/local_executor.go:rebuildCommandList().
func GetTaskExecutionSteps(project *Project, taskName string) ([]TaskExecutionStep, error) {
	task := project.FindProjectTask(taskName)
	if task == nil {
		return nil, errors.Errorf("task '%s' not found in project", taskName)
	}

	type commandBlock struct {
		blockType string
		commands  []PluginCommandConf
	}

	var blocks []commandBlock

	if project.Pre != nil {
		if cmds := project.Pre.List(); len(cmds) > 0 {
			blocks = append(blocks, commandBlock{blockType: "pre", commands: cmds})
		}
	}

	blocks = append(blocks, commandBlock{blockType: "", commands: task.Commands})

	if project.Post != nil {
		if cmds := project.Post.List(); len(cmds) > 0 {
			blocks = append(blocks, commandBlock{blockType: "post", commands: cmds})
		}
	}

	var steps []TaskExecutionStep

	for _, block := range blocks {
		totalCmds := len(block.commands)

		for cmdIdx, cmd := range block.commands {
			cmdNum := cmdIdx + 1

			if cmd.Function != "" {
				funcCmds, ok := project.Functions[cmd.Function]
				if !ok || funcCmds == nil {
					// Function not found; add a single entry for the unresolved function.
					steps = append(steps, TaskExecutionStep{
						StepNumber:   formatStepNumber(block.blockType, cmdNum, 0, 0),
						DisplayName:  formatDisplayName(cmd.Function, cmd.DisplayName, block.blockType, cmdNum, totalCmds, cmd.Function, 0, 0),
						CommandName:  cmd.Function,
						IsFunction:   true,
						FunctionName: cmd.Function,
						BlockType:    block.blockType,
					})
					continue
				}

				subCmds := funcCmds.List()
				for subIdx, subCmd := range subCmds {
					if subCmd.Function != "" {
						// Nested functions are not supported; skip.
						continue
					}
					subCmdNum := subIdx + 1
					totalSubCmds := len(subCmds)
					steps = append(steps, TaskExecutionStep{
						StepNumber:   formatStepNumber(block.blockType, cmdNum, subCmdNum, totalSubCmds),
						DisplayName:  formatDisplayName(subCmd.Command, subCmd.DisplayName, block.blockType, cmdNum, totalCmds, cmd.Function, subCmdNum, totalSubCmds),
						CommandName:  subCmd.Command,
						IsFunction:   true,
						FunctionName: cmd.Function,
						BlockType:    block.blockType,
					})
				}
			} else {
				steps = append(steps, TaskExecutionStep{
					StepNumber:  formatStepNumber(block.blockType, cmdNum, 0, 0),
					DisplayName: formatDisplayName(cmd.Command, cmd.DisplayName, block.blockType, cmdNum, totalCmds, "", 0, 0),
					CommandName: cmd.Command,
					BlockType:   block.blockType,
				})
			}
		}
	}

	return steps, nil
}

// formatStepNumber formats a step number string following the same logic as
// agent/taskexec CommandInfo step numbering.
// - Main block: "1" or "2.1" (if function has >1 sub-cmd)
// - Pre/post block: "pre:1" or "post:2.1"
func formatStepNumber(blockType string, cmdNum, subCmdNum, totalSubCmds int) string {
	var step string
	if subCmdNum > 0 && totalSubCmds > 1 {
		step = fmt.Sprintf("%d.%d", cmdNum, subCmdNum)
	} else {
		step = fmt.Sprintf("%d", cmdNum)
	}
	if blockType != "" {
		return fmt.Sprintf("%s:%s", blockType, step)
	}
	return step
}

// formatDisplayName formats a display name following the same logic as
// agent/command/registry.go:GetFullDisplayName().
func formatDisplayName(cmdName, displayName, blockType string, cmdNum, totalCmds int, funcName string, subCmdNum, totalSubCmds int) string {
	fullName := fmt.Sprintf("'%s'", cmdName)
	if displayName != "" {
		fullName = fmt.Sprintf("%s ('%s')", fullName, displayName)
	}
	if funcName != "" {
		fullName = fmt.Sprintf("%s in function '%s'", fullName, funcName)
	}
	if cmdNum > 0 && totalCmds > 0 {
		if subCmdNum > 0 && totalSubCmds > 1 {
			fullName = fmt.Sprintf("%s (step %d.%d of %d)", fullName, cmdNum, subCmdNum, totalCmds)
		} else {
			fullName = fmt.Sprintf("%s (step %d of %d)", fullName, cmdNum, totalCmds)
		}
	}
	if blockType != "" {
		fullName = fmt.Sprintf("%s in block '%s'", fullName, blockType)
	}
	return fullName
}
