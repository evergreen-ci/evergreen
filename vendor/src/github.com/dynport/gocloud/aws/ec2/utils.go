package ec2

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func IpForSsh(instance *Instance) (openIp string, e error) {
	cnt := 0
	type status struct {
		ip string
		ok bool
	}
	c := make(chan *status)
	for _, i := range []string{instance.PrivateIpAddress, instance.IpAddress} {
		cnt++
		go func(ip string, ch chan *status) {
			rsp := &status{ip: ip}
			defer func() { c <- rsp }()
			c, e := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, 22), 100*time.Millisecond)
			if e != nil {
				return
			}
			defer c.Close()
			rsp.ok = true
		}(i, c)

	}

	timeout := time.After(2 * time.Second)
	for cnt > 0 {
		select {
		case s := <-c:
			cnt--
			if s.ok {
				return s.ip, nil
			}
		case <-timeout:
			return "", fmt.Errorf("timed out")
		}
	}
	return "", nil
}

func waitForSsh(instance *Instance, timeoutDuration time.Duration) (string, error) {
	ticker := time.Tick(1 * time.Second)
	timeout := time.After(timeoutDuration)
	for {
		select {
		case <-ticker:
			ip, e := IpForSsh(instance)
			if e == nil {
				return ip, nil
			}
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for ssh")
		}
	}
}

func WaitForInstances(instances []*Instance, timeout time.Duration) error {
	cnt := 0
	type result struct {
		Error error
		Ip    string
	}
	finished := make(chan *result)
	for _, i := range instances {
		cnt++
		go func(inst *Instance) {
			ip, e := waitForSsh(inst, timeout)
			finished <- &result{Error: e, Ip: ip}
		}(i)
	}

	errors := []string{}
	for cnt > 0 {
		select {
		case r := <-finished:
			cnt--
			if r.Error != nil {
				errors = append(errors, r.Ip+": "+r.Error.Error())
			}
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, ", "))
	}
	return nil
}
