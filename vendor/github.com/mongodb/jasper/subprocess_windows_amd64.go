package jasper

type JobObjectBasicProcessIdList struct {
	NumberOfAssignedProcesses uint32
	NumberOfProcessIdsInList  uint32
	ProcessIdList             [MAX_PROCESS_ID_LIST_LENGTH]uint64
}
