package jasper

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/pkg/errors"
)

// TODO: needs some documentation.

const (
	// Constants for error codes
	ERROR_SUCCESS           syscall.Errno = 0
	ERROR_FILE_NOT_FOUND    syscall.Errno = 2
	ERROR_ACCESS_DENIED     syscall.Errno = 5
	ERROR_INVALID_HANDLE    syscall.Errno = 6
	ERROR_INVALID_PARAMETER syscall.Errno = 87

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

	// Constants for job object limits
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE          = 0x2000
	JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION = 0x400
	JOB_OBJECT_LIMIT_ACTIVE_PROCESS             = 8
	JOB_OBJECT_LIMIT_JOB_MEMORY                 = 0x200
	JOB_OBJECT_LIMIT_JOB_TIME                   = 4
	JOB_OBJECT_LIMIT_PROCESS_MEMORY             = 0x100
	JOB_OBJECT_LIMIT_PROCESS_TIME               = 2
	JOB_OBJECT_LIMIT_WORKINGSET                 = 1
	JOB_OBJECT_LIMIT_AFFINITY                   = 0x00000010

	JobObjectInfoClassNameBasicProcessIdList       = 3
	JobObjectInfoClassNameExtendedLimitInformation = 9

	// Constants for exit codes
	STILL_ACTIVE = 0x103

	// Constants for access rights for event objects
	EVENT_ALL_ACCESS   = 0x1F0003
	EVENT_MODIFY_STATE = 0x0002

	// Constants representing the wait return value
	WAIT_OBJECT_0  uint32 = 0x00000000
	WAIT_ABANDONED        = 0x00000080
	WAIT_FAILED           = 0xFFFFFFFF
	WAIT_TIMEOUT          = 0x00000102

	// Max allowed length of process ID list
	MAX_PROCESS_ID_LIST_LENGTH = 1000
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procAssignProcessToJobObject  = modkernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle               = modkernel32.NewProc("CloseHandle")
	procCreateJobObjectW          = modkernel32.NewProc("CreateJobObjectW")
	procCreateEventW              = modkernel32.NewProc("CreateEventW")
	procOpenEventW                = modkernel32.NewProc("OpenEventW")
	procSetEvent                  = modkernel32.NewProc("SetEvent")
	procGetExitCodeProcess        = modkernel32.NewProc("GetExitCodeProcess")
	procOpenProcess               = modkernel32.NewProc("OpenProcess")
	procTerminateProcess          = modkernel32.NewProc("TerminateProcess")
	procQueryInformationJobObject = modkernel32.NewProc("QueryInformationJobObject")
	procTerminateJobObject        = modkernel32.NewProc("TerminateJobObject")
	procSetInformationJobObject   = modkernel32.NewProc("SetInformationJobObject")
	procWaitForSingleObject       = modkernel32.NewProc("WaitForSingleObject")
)

type Job struct {
	handle syscall.Handle
}

func NewWindowsJobObject(name string) (*Job, error) {
	utf16Name, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, NewWindowsError("UTF16PtrFromString", err)
	}
	hJob, err := CreateJobObject(utf16Name)
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

func (j *Job) Close() error {
	if j.handle != 0 {
		if err := CloseHandle(j.handle); err != nil {
			return NewWindowsError("CloseHandle", err)
		}
		j.handle = 0
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////
//
// All the methods below are boilerplate functions for accessing the Windows syscalls.
//
///////////////////////////////////////////////////////////////////////////////////////////

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

func GetExitCodeProcess(handle syscall.Handle, exitCode *uint32) error {
	r1, _, e1 := procGetExitCodeProcess.Call(uintptr(handle), uintptr(unsafe.Pointer(exitCode)))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

func TerminateProcess(handle syscall.Handle, exitCode uint32) error {
	r1, _, e1 := procTerminateProcess.Call(uintptr(handle), uintptr(exitCode))
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return e1
		} else {
			return syscall.EINVAL
		}
	}
	return nil
}

type SecurityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      uint32
}

func CreateJobObject(name *uint16) (syscall.Handle, error) {
	jobAttributes := &SecurityAttributes{}

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

func QueryInformationJobObjectProcessIdList(job syscall.Handle) (*JobObjectBasicProcessIdList, error) {
	info := JobObjectBasicProcessIdList{}
	_, err := QueryInformationJobObject(
		job,
		JobObjectInfoClassNameBasicProcessIdList,
		unsafe.Pointer(&info),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func QueryInformationJobObject(job syscall.Handle, infoClass uint32, info unsafe.Pointer, length uint32) (uint32, error) {
	var nLength uint32
	r1, _, e1 := procQueryInformationJobObject.Call(
		uintptr(job),
		uintptr(infoClass),
		uintptr(info),
		uintptr(length),
		uintptr(unsafe.Pointer(&nLength)),
	)
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return 0, e1
		} else {
			return 0, syscall.EINVAL
		}
	}
	return nLength, nil
}

func SetInformationJobObjectExtended(job syscall.Handle, info JobObjectExtendedLimitInformation) error {
	r1, _, e1 := procSetInformationJobObject.Call(uintptr(job), JobObjectInfoClassNameExtendedLimitInformation,
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

func WaitForSingleObject(object syscall.Handle, timeout time.Duration) (uint32, error) {
	timeoutMillis := int64(timeout * time.Millisecond)
	r1, _, e1 := procWaitForSingleObject.Call(
		uintptr(object),
		uintptr(uint32(timeoutMillis)),
	)
	waitStatus := uint32(r1)
	if waitStatus == WAIT_FAILED {
		if e1 != ERROR_SUCCESS {
			return waitStatus, e1
		} else {
			return waitStatus, syscall.EINVAL
		}
	}
	return waitStatus, nil
}

func getWaitStatusError(waitStatus uint32) error {
	switch waitStatus {
	case WAIT_OBJECT_0:
		return nil
	case WAIT_ABANDONED:
		return errors.New("mutex object was not released by the owning thread before it terminated")
	case WAIT_FAILED:
		return errors.New("wait failed")
	case WAIT_TIMEOUT:
		return errors.New("wait timed out before the object could be signaled")
	default:
		return errors.New("wait failed due to unknown reason")
	}
}

func CreateEvent(name *uint16) (syscall.Handle, error) {
	r1, _, e1 := procCreateEventW.Call(
		uintptr(unsafe.Pointer(nil)),
		uintptr(1),
		uintptr(0),
		uintptr(unsafe.Pointer(name)),
	)
	handle := syscall.Handle(r1)
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return handle, e1
		} else {
			return handle, syscall.EINVAL
		}
	}
	return handle, nil
}

func OpenEvent(name *uint16) (syscall.Handle, error) {
	r1, _, e1 := procOpenEventW.Call(
		uintptr(EVENT_MODIFY_STATE),
		uintptr(0),
		uintptr(unsafe.Pointer(name)),
	)
	handle := syscall.Handle(r1)
	if r1 == 0 {
		if e1 != ERROR_SUCCESS {
			return handle, e1
		} else {
			return handle, syscall.EINVAL
		}
	}
	return handle, nil
}

func SetEvent(event syscall.Handle) error {
	r1, _, e1 := procSetEvent.Call(uintptr(event))
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
