package model

import "time"

type PanicReport struct {
	Version                 string
	CurrentWorkingDirectory string
	ExecutablePath          string
	Arguments               []string
	OperatingSystem         string
	Architecture            string
	ConfigFilePath          string
	StartTime, EndTime      time.Time

	Panic any
}
