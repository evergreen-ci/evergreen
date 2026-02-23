package util

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	ERROR_SUCCESS syscall.Errno = 0

	DELETE                   = 0x00010000
	READ_CONTROL             = 0x00020000
	WRITE_DAC                = 0x00040000
	WRITE_OWNER              = 0x00080000
	SYNCHRONIZE              = 0x00100000
	STANDARD_RIGHTS_REQUIRED = 0x000F0000
	STANDARD_RIGHTS_READ     = READ_CONTROL
	STANDARD_RIGHTS_WRITE    = READ_CONTROL
	STANDARD_RIGHTS_EXECUTE  = READ_CONTROL
	STANDARD_RIGHTS_ALL      = 0x001F0000
	SPECIFIC_RIGHTS_ALL      = 0x0000FFFF
	ACCESS_SYSTEM_SECURITY   = 0x01000000
	MAXIMUM_ALLOWED          = 0x02000000

	// Constants for process permissions
	PROCESS_TERMINATE                 = 0x0001
	PROCESS_CREATE_THREAD             = 0x0002
	PROCESS_SET_SESSIONID             = 0x0004
	PROCESS_VM_OPERATION              = 0x0008
	PROCESS_VM_READ                   = 0x0010
	PROCESS_VM_WRITE                  = 0x0020
	PROCESS_DUP_HANDLE                = 0x0040
	PROCESS_CREATE_PROCESS            = 0x0080
	PROCESS_SET_QUOTA                 = 0x0100
	PROCESS_SET_INFORMATION           = 0x0200
	PROCESS_QUERY_INFORMATION         = 0x0400
	PROCESS_SUSPEND_RESUME            = 0x0800
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_ALL_ACCESS                = STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0xFFFF

	// constants for job object limits
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE          = 0x2000
	JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION = 0x400
	JOB_OBJECT_LIMIT_ACTIVE_PROCESS             = 8
	JOB_OBJECT_LIMIT_JOB_MEMORY                 = 0x200
	JOB_OBJECT_LIMIT_JOB_TIME                   = 4
	JOB_OBJECT_LIMIT_PROCESS_MEMORY             = 0x100
	JOB_OBJECT_LIMIT_PROCESS_TIME               = 2
	JOB_OBJECT_LIMIT_WORKINGSET                 = 1
	JOB_OBJECT_LIMIT_AFFINITY                   = 0x00000010

	jobObjectInfoClassNameExtendedLimitInformation = 9
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle              = modkernel32.NewProc("CloseHandle")
	procCreateJobObjectW         = modkernel32.NewProc("CreateJobObjectW")
	procOpenProcess              = modkernel32.NewProc("OpenProcess")
	procTerminateJobObject       = modkernel32.NewProc("TerminateJobObject")
	setinformationJobObject      = modkernel32.NewProc("SetInformationJobObject")

	processMapping = newProcessRegistry()
)

////////////////////////////////////////////////////////////////////////
//
// implementation of the internals of our process mapping registry
//
////////////////////////////////////////////////////////////////////////

type processRegistry struct {
	jobs map[string]*Job
	mu   sync.Mutex
}

func newProcessRegistry() *processRegistry {
	return &processRegistry{
		jobs: make(map[string]*Job),
	}
}

func (r *processRegistry) getJob(taskId string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	j, ok := r.jobs[taskId]
	if !ok {
		var err error
		j, err = NewJob(taskId)
		if err != nil {
			return nil, errors.Wrapf(err, "creating job object for task '%s'", taskId)
		}

		r.jobs[taskId] = j

		return j, nil
	}

	return j, nil
}

func (r *processRegistry) removeJob(taskId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[taskId]
	if !ok {
		return nil
	}

	var err error
	defer func() {
		err = errors.Wrapf(job.Close(), "closing job object for task '%s'", taskId)
	}()

	delete(r.jobs, taskId)

	return err
}

////////////////////////////////////////////////////////////////////////
//
// Functions used to manage processes used by the shell command
//
////////////////////////////////////////////////////////////////////////

// This windows-specific specific implementation of trackProcess associates the given pid with a
// job object, which can later be used by "cleanup" to terminate all members of the job object at
// once. If a job object doesn't already exist, it will create one automatically, scoped by the
// task ID for which the shell process was started.
func TrackProcess(taskId string, pid int, logger grip.Journaler) {
	job, err := processMapping.getJob(taskId)
	if err != nil {
		logger.Errorf("Failed to get job object: %s", err)
		return
	}

	logger.Infof("Tracking process with PID %d.", pid)

	if err = job.AssignProcess(uint(pid)); err != nil {
		logger.Errorf("Failed assigning process with PID %d to job object: %s.", pid, err)
		return
	}
}

// cleanup() has a windows-specific implementation which finds the job object associated with the
// given task key, and if it exists, terminates it. This will guarantee that any shell processes
// started throughout the task run are destroyed, as long as they were captured in trackProcess.
func KillSpawnedProcs(ctx context.Context, key, workingDir, _ string, logger grip.Journaler) error {
	job, err := processMapping.getJob(key)
	if err != nil {
		return nil
	}

	if err := job.Terminate(0); err != nil {
		logger.Errorf("Failed to terminate job object '%s': %s.", key, err)
		return errors.Wrapf(err, "terminating job object '%s'", key)
	}

	return errors.Wrapf(processMapping.removeJob(key), "removing job object '%s' from internal Evergreen tracking mechanism", key)
}

///////////////////////////////////////////////////////////////////////////////////////////
//
// All the methods below are boilerplate functions for accessing the Windows syscalls for
// working with Job objects.
//
///////////////////////////////////////////////////////////////////////////////////////////

type Job struct {
	handle syscall.Handle
}

type IoCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type JobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit uint64
	PerJobUserTimeLimit     uint64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type JobObjectExtendedLimitInformation struct {
	BasicLimitInformation JobObjectBasicLimitInformation
	IoInfo                IoCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func NewJob(name string) (*Job, error) {
	hJob, err := CreateJobObject(syscall.StringToUTF16Ptr(name))
	if err != nil {
		return nil, NewWindowsError("CreateJobObject", err)
	}

	if err := SetInformationJobObjectExtended(hJob, JobObjectExtendedLimitInformation{
		BasicLimitInformation: JobObjectBasicLimitInformation{
			LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}); err != nil {
		return nil, NewWindowsError("SetInformationJobObject", err)
	}

	return &Job{handle: hJob}, nil
}

func (j *Job) AssignProcess(pid uint) error {
	hProcess, err := OpenProcess(PROCESS_ALL_ACCESS, false, uint32(pid))
	if err != nil {
		return NewWindowsError("OpenProcess", err)
	}
	defer CloseHandle(hProcess)
	if err := AssignProcessToJobObject(j.handle, hProcess); err != nil {
		return NewWindowsError("AssignProcessToJobObject", err)
	}
	return nil
}

func (j *Job) Terminate(exitCode uint) error {
	if err := TerminateJobObject(j.handle, uint32(exitCode)); err != nil {
		return NewWindowsError("TerminateJobObject", err)
	}
	return nil
}

func OpenProcess(desiredAccess uint32, inheritHandle bool, processId uint32) (syscall.Handle, error) {
	var inheritHandleRaw int32
	if inheritHandle {
		inheritHandleRaw = 1
	} else {
		inheritHandleRaw = 0
	}
	r1, _, e1 := procOpenProcess.Call(
		uintptr(desiredAccess),
		uintptr(inheritHandleRaw),
		uintptr(processId))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return 0, e1
		} else {
			return 0, syscall.EINVAL
		}
	}
	return syscall.Handle(r1), nil
}

func (j *Job) Close() error {
	if j.handle != 0 {
		if err := CloseHandle(j.handle); err != nil {
			return NewWindowsError("CloseHandle", err)
		}
		j.handle = 0
	}
	return nil
}

func CreateJobObject(name *uint16) (syscall.Handle, error) {
	jobAttributes := &struct {
		Length             uint32
		SecurityDescriptor *byte
		InheritHandle      int32
	}{}

	r1, _, e1 := procCreateJobObjectW.Call(
		uintptr(unsafe.Pointer(jobAttributes)),
		uintptr(unsafe.Pointer(name)))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return 0, e1
		} else {
			return 0, syscall.EINVAL
		}
	}
	return syscall.Handle(r1), nil
}

func AssignProcessToJobObject(job syscall.Handle, process syscall.Handle) error {
	r1, _, e1 := procAssignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

func TerminateJobObject(job syscall.Handle, exitCode uint32) error {
	r1, _, e1 := procTerminateJobObject.Call(uintptr(job), uintptr(exitCode))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

func CloseHandle(object syscall.Handle) error {
	r1, _, e1 := procCloseHandle.Call(uintptr(object))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

func SetInformationJobObjectExtended(job syscall.Handle, info JobObjectExtendedLimitInformation) error {
	r1, _, e1 := setinformationJobObject.Call(uintptr(job), jobObjectInfoClassNameExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uintptr(uint32(unsafe.Sizeof(info))))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

type WindowsError struct {
	functionName string
	innerError   error
}

func NewWindowsError(functionName string, innerError error) *WindowsError {
	return &WindowsError{functionName, innerError}
}

func (we *WindowsError) FunctionName() string {
	return we.functionName
}

func (we *WindowsError) InnerError() error {
	return we.innerError
}

func (we *WindowsError) Error() string {
	return fmt.Sprintf("gowin32: %s failed: %v", we.functionName, we.innerError)
}

// SetNice is a no-op in Windows (Linux-only).
func SetNice(int, int) error {
	return nil
}
