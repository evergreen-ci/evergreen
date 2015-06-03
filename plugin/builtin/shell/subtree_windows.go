package shell

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/plugin"
	"sync"
	"syscall"
	"unsafe"
)

// Map of TASK_ID to job object ID in windows
var (
	jobsMapping = make(map[string]*Job)
	jobsMutex   sync.Mutex
)

const (
	ERROR_SUCCESS syscall.Errno = 0
)
const (
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
)

// Constants for process permissions
const (
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
)

type SECURITY_ATTRIBUTES struct {
	Length             uint32
	SecurityDescriptor *byte
	InheritHandle      int32
}

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle              = modkernel32.NewProc("CloseHandle")
	procCreateJobObjectW         = modkernel32.NewProc("CreateJobObjectW")
	procOpenJobObjectW           = modkernel32.NewProc("OpenJobObjectW")
	procOpenProcess              = modkernel32.NewProc("OpenProcess")
	procTerminateJobObject       = modkernel32.NewProc("TerminateJobObject")
)

// This windows-specific specific implementation of trackProcess associates the given pid with a
// job object, which can later be used by "cleanup" to terminate all members of the job object at
// once. If a job object doesn't already exist, it will create one automatically, scoped by the
// task ID for which the shell process was started.
func trackProcess(taskId string, pid int, log plugin.Logger) error {
	jobsMutex.Lock()
	defer jobsMutex.Unlock()
	var job *Job
	var err error
	// If we have already created an existing job object for this task, find it
	if jobObj, hasKey := jobsMapping[taskId]; hasKey {
		job = jobObj
	} else {
		log.LogSystem(slogger.INFO, "tracking process with pid %v", pid)
		// Job object does not exist yet for this task, so we must create one
		job, err = NewJob(taskId)
		if err != nil {
			log.LogSystem(slogger.ERROR, "failed creating job object: %v", err)
			return err
		}
		jobsMapping[taskId] = job
	}
	err = job.AssignProcess(uint(pid))
	if err != nil {
		log.LogSystem(slogger.ERROR, "failed assigning process %v to job object: %v", pid, err)
	}
	return err
}

// cleanup() has a windows-specific implementation which finds the job object associated with the
// given task key, and if it exists, terminates it. This will guarantee that any shell processes
// started throughout the task run are destroyed, as long as they were captured in trackProcess.
func cleanup(key string, log plugin.Logger) error {
	jobsMutex.Lock()
	defer jobsMutex.Unlock()
	job, hasKey := jobsMapping[key]
	if !hasKey {
		return nil
	}

	err := job.Terminate(0)
	if err != nil {
		log.LogSystem(slogger.ERROR, "terminating job object failed: %v", err)
		return err
	}
	delete(jobsMapping, key)
	defer job.Close()
	return nil
}

// All the methods below are boilerplate functions for accessing the Windows syscalls for
// working with Job objects.

type Job struct {
	handle syscall.Handle
}

func NewJob(name string) (*Job, error) {
	hJob, err := CreateJobObject(nil, syscall.StringToUTF16Ptr(name))
	if err != nil {
		return nil, NewWindowsError("CreateJobObject", err)
	}
	return &Job{handle: hJob}, nil
}

func (self *Job) AssignProcess(pid uint) error {
	hProcess, err := OpenProcess(PROCESS_ALL_ACCESS, false, uint32(pid))
	if err != nil {
		return NewWindowsError("OpenProcess", err)
	}
	defer CloseHandle(hProcess)
	if err := AssignProcessToJobObject(self.handle, hProcess); err != nil {
		return NewWindowsError("AssignProcessToJobObject", err)
	}
	return nil
}

func (self *Job) Terminate(exitCode uint) error {
	if err := TerminateJobObject(self.handle, uint32(exitCode)); err != nil {
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

func (self *Job) Close() error {
	if self.handle != 0 {
		if err := CloseHandle(self.handle); err != nil {
			return NewWindowsError("CloseHandle", err)
		}
		self.handle = 0
	}
	return nil
}

func CreateJobObject(jobAttributes *SECURITY_ATTRIBUTES, name *uint16) (syscall.Handle, error) {
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

type WindowsError struct {
	functionName string
	innerError   error
}

func NewWindowsError(functionName string, innerError error) *WindowsError {
	return &WindowsError{functionName, innerError}
}

func (self *WindowsError) FunctionName() string {
	return self.functionName
}

func (self *WindowsError) InnerError() error {
	return self.innerError
}

func (self *WindowsError) Error() string {
	return fmt.Sprintf("gowin32: %s failed: %v", self.functionName, self.innerError)
}
