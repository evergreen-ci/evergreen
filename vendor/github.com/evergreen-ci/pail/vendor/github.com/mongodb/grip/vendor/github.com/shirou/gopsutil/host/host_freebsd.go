// +build freebsd

package host

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/internal/common"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"
)

const (
	UTNameSize = 16 /* see MAXLOGNAME in <sys/param.h> */
	UTLineSize = 8
	UTHostSize = 16
)

func Info() (*InfoStat, error) {
	ret := &InfoStat{
		OS:             runtime.GOOS,
		PlatformFamily: "freebsd",
	}

	hostname, err := os.Hostname()
	if err == nil {
		ret.Hostname = hostname
	}

	platform, family, version, err := PlatformInformation()
	if err == nil {
		ret.Platform = platform
		ret.PlatformFamily = family
		ret.PlatformVersion = version
		ret.KernelVersion = version
	}

	system, role, err := Virtualization()
	if err == nil {
		ret.VirtualizationSystem = system
		ret.VirtualizationRole = role
	}

	boot, err := BootTime()
	if err == nil {
		ret.BootTime = boot
		ret.Uptime = uptime(boot)
	}

	procs, err := process.Pids()
	if err == nil {
		ret.Procs = uint64(len(procs))
	}

	hostid, err := unix.Sysctl("kern.hostuuid")
	if err == nil && hostid != "" {
		ret.HostID = strings.ToLower(hostid)
	}

	return ret, nil
}

// cachedBootTime must be accessed via atomic.Load/StoreUint64
var cachedBootTime uint64

func BootTime() (uint64, error) {
	t := atomic.LoadUint64(&cachedBootTime)
	if t != 0 {
		return t, nil
	}
	buf, err := unix.SysctlRaw("kern.boottime")
	if err != nil {
		return 0, err
	}

	tv := *(*syscall.Timeval)(unsafe.Pointer((&buf[0])))
	atomic.StoreUint64(&cachedBootTime, uint64(tv.Sec))

	return t, nil
}

func uptime(boot uint64) uint64 {
	return uint64(time.Now().Unix()) - boot
}

func Uptime() (uint64, error) {
	boot, err := BootTime()
	if err != nil {
		return 0, err
	}
	return uptime(boot), nil
}

func Users() ([]UserStat, error) {
	utmpfile := "/var/run/utx.active"
	if !common.PathExists(utmpfile) {
		utmpfile = "/var/run/utmp" // before 9.0
		return getUsersFromUtmp(utmpfile)
	}

	var ret []UserStat
	file, err := os.Open(utmpfile)
	if err != nil {
		return ret, err
	}
	defer file.Close()

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return ret, err
	}

	entrySize := sizeOfUtmpx
	count := len(buf) / entrySize

	for i := 0; i < count; i++ {
		b := buf[i*sizeOfUtmpx : (i+1)*sizeOfUtmpx]
		var u Utmpx
		br := bytes.NewReader(b)
		err := binary.Read(br, binary.LittleEndian, &u)
		if err != nil || u.Type != 4 {
			continue
		}
		sec := (binary.LittleEndian.Uint32(u.Tv.Sec[:])) / 2 // TODO:
		user := UserStat{
			User:     common.IntToString(u.User[:]),
			Terminal: common.IntToString(u.Line[:]),
			Host:     common.IntToString(u.Host[:]),
			Started:  int(sec),
		}

		ret = append(ret, user)
	}

	return ret, nil

}

func PlatformInformation() (string, string, string, error) {
	platform := ""
	family := ""
	version := ""
	uname, err := exec.LookPath("uname")
	if err != nil {
		return "", "", "", err
	}

	out, err := invoke.Command(uname, "-s")
	if err == nil {
		platform = strings.ToLower(strings.TrimSpace(string(out)))
	}

	out, err = invoke.Command(uname, "-r")
	if err == nil {
		version = strings.ToLower(strings.TrimSpace(string(out)))
	}

	return platform, family, version, nil
}

func Virtualization() (string, string, error) {
	return "", "", common.ErrNotImplementedError
}

// before 9.0
func getUsersFromUtmp(utmpfile string) ([]UserStat, error) {
	var ret []UserStat
	file, err := os.Open(utmpfile)
	if err != nil {
		return ret, err
	}
	defer file.Close()

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return ret, err
	}

	u := Utmp{}
	entrySize := int(unsafe.Sizeof(u))
	count := len(buf) / entrySize

	for i := 0; i < count; i++ {
		b := buf[i*entrySize : i*entrySize+entrySize]
		var u Utmp
		br := bytes.NewReader(b)
		err := binary.Read(br, binary.LittleEndian, &u)
		if err != nil || u.Time == 0 {
			continue
		}
		user := UserStat{
			User:     common.IntToString(u.Name[:]),
			Terminal: common.IntToString(u.Line[:]),
			Host:     common.IntToString(u.Host[:]),
			Started:  int(u.Time),
		}

		ret = append(ret, user)
	}

	return ret, nil
}

func SensorsTemperatures() ([]TemperatureStat, error) {
	return []TemperatureStat{}, common.ErrNotImplementedError
}

func KernelVersion() (string, error) {
	_, _, version, err := PlatformInformation()
	return version, err
}
