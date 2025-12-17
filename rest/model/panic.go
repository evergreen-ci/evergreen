package model

import "time"

type PanicReport struct {
	Version                 string
	AgentVersion            string
	BuildRevision           string
	CurrentWorkingDirectory string
	ExecutablePath          string
	Arguments               []string
	OperatingSystem         string
	Architecture            string
	ConfigFilePath          string
	ConfigAbsFilePath       string
	StartTime, EndTime      time.Time
	User                    string
	Project                 string

	Panic any
}
