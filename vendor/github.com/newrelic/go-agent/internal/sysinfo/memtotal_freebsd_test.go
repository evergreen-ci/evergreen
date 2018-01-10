package sysinfo

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
)

var re = regexp.MustCompile(`hw\.physmem:\s*(\d+)`)

func freebsdSysctlMemoryBytes() (uint64, error) {
	out, err := exec.Command("/sbin/sysctl", "hw.physmem").Output()
	if err != nil {
		return 0, err
	}

	match := re.FindSubmatch(out)
	if match == nil {
		return 0, errors.New("memory size not found in sysctl output")
	}

	bts, err := strconv.ParseUint(string(match[1]), 10, 64)
	if err != nil {
		return 0, err
	}

	return bts, nil
}

func TestPhysicalMemoryBytes(t *testing.T) {
	mem, err := PhysicalMemoryBytes()
	if err != nil {
		t.Fatal(err)
	}

	mem2, err := freebsdSysctlMemoryBytes()
	if nil != err {
		t.Fatal(err)
	}

	if mem != mem2 {
		t.Error(mem, mem2)
	}
}
