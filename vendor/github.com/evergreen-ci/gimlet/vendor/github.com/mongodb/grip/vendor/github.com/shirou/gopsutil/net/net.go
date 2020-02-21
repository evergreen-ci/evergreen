package net

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/internal/common"
)

var invoke common.Invoker = common.Invoke{}

type IOCountersStat struct {
	Name        string `json:"name" bson:"name,omitempty"`               // interface name
	BytesSent   uint64 `json:"bytesSent" bson:"bytesSent,omitempty"`     // number of bytes sent
	BytesRecv   uint64 `json:"bytesRecv" bson:"bytesRecv,omitempty"`     // number of bytes received
	PacketsSent uint64 `json:"packetsSent" bson:"packetsSent,omitempty"` // number of packets sent
	PacketsRecv uint64 `json:"packetsRecv" bson:"packetsRecv,omitempty"` // number of packets received
	Errin       uint64 `json:"errin" bson:"errin,omitempty"`             // total number of errors while receiving
	Errout      uint64 `json:"errout" bson:"errout,omitempty"`           // total number of errors while sending
	Dropin      uint64 `json:"dropin" bson:"dropin,omitempty"`           // total number of incoming packets which were dropped
	Dropout     uint64 `json:"dropout" bson:"dropout,omitempty"`         // total number of outgoing packets which were dropped (always 0 on OSX and BSD)
	Fifoin      uint64 `json:"fifoin" bson:"fifoin,omitempty"`           // total number of FIFO buffers errors while receiving
	Fifoout     uint64 `json:"fifoout" bson:"fifoout,omitempty"`         // total number of FIFO buffers errors while sending

}

// Addr is implemented compatibility to psutil
type Addr struct {
	IP   string `json:"ip" bson:"ip,omitempty"`
	Port uint32 `json:"port" bson:"port,omitempty"`
}

type ConnectionStat struct {
	Fd     uint32  `json:"fd" bson:"fd,omitempty"`
	Family uint32  `json:"family" bson:"family,omitempty"`
	Type   uint32  `json:"type" bson:"type,omitempty"`
	Laddr  Addr    `json:"localaddr" bson:"localaddr,omitempty"`
	Raddr  Addr    `json:"remoteaddr" bson:"remoteaddr,omitempty"`
	Status string  `json:"status" bson:"status,omitempty"`
	Uids   []int32 `json:"uids" bson:"uids,omitempty"`
	Pid    int32   `json:"pid" bson:"pid,omitempty"`
}

// System wide stats about different network protocols
type ProtoCountersStat struct {
	Protocol string           `json:"protocol" bson:"protocol,omitempty"`
	Stats    map[string]int64 `json:"stats" bson:"stats,omitempty"`
}

// NetInterfaceAddr is designed for represent interface addresses
type InterfaceAddr struct {
	Addr string `json:"addr" bson:"addr,omitempty"`
}

type InterfaceStat struct {
	Index        int             `json:"index" bson:"index,omitempty"`
	MTU          int             `json:"mtu" bson:"mtu,omitempty"`                   // maximum transmission unit
	Name         string          `json:"name"`                                       // e.g., "en0", "lo0", "eth0.100" bson:"name"`         // e.g., "en0", "lo0", "eth0.100,omitempty"
	HardwareAddr string          `json:"hardwareaddr" bson:"hardwareaddr,omitempty"` // IEEE MAC-48, EUI-48 and EUI-64 form
	Flags        []string        `json:"flags" bson:"flags,omitempty"`               // e.g., FlagUp, FlagLoopback, FlagMulticast
	Addrs        []InterfaceAddr `json:"addrs" bson:"addrs,omitempty"`
}

type FilterStat struct {
	ConnTrackCount int64 `json:"conntrackCount" bson:"conntrackCount,omitempty"`
	ConnTrackMax   int64 `json:"conntrackMax" bson:"conntrackMax,omitempty"`
}

// ConntrackStat has conntrack summary info
type ConntrackStat struct {
	Entries       uint32 `json:"entries" bson:"entries,omitempty"`               // Number of entries in the conntrack table
	Searched      uint32 `json:"searched" bson:"searched,omitempty"`             // Number of conntrack table lookups performed
	Found         uint32 `json:"found" bson:"found,omitempty"`                   // Number of searched entries which were successful
	New           uint32 `json:"new" bson:"new,omitempty"`                       // Number of entries added which were not expected before
	Invalid       uint32 `json:"invalid" bson:"invalid,omitempty"`               // Number of packets seen which can not be tracked
	Ignore        uint32 `json:"ignore" bson:"ignore,omitempty"`                 // Packets seen which are already connected to an entry
	Delete        uint32 `json:"delete" bson:"delete,omitempty"`                 // Number of entries which were removed
	DeleteList    uint32 `json:"delete_list" bson:"delete_list,omitempty"`       // Number of entries which were put to dying list
	Insert        uint32 `json:"insert" bson:"insert,omitempty"`                 // Number of entries inserted into the list
	InsertFailed  uint32 `json:"insert_failed" bson:"insert_failed,omitempty"`   // # insertion attempted but failed (same entry exists)
	Drop          uint32 `json:"drop" bson:"drop,omitempty"`                     // Number of packets dropped due to conntrack failure.
	EarlyDrop     uint32 `json:"early_drop" bson:"early_drop,omitempty"`         // Dropped entries to make room for new ones, if maxsize reached
	IcmpError     uint32 `json:"icmp_error" bson:"icmp_error,omitempty"`         // Subset of invalid. Packets that can't be tracked d/t error
	ExpectNew     uint32 `json:"expect_new" bson:"expect_new,omitempty"`         // Entries added after an expectation was already present
	ExpectCreate  uint32 `json:"expect_create" bson:"expect_create,omitempty"`   // Expectations added
	ExpectDelete  uint32 `json:"expect_delete" bson:"expect_delete,omitempty"`   // Expectations deleted
	SearchRestart uint32 `json:"search_restart" bson:"search_restart,omitempty"` // Conntrack table lookups restarted due to hashtable resizes
}

func NewConntrackStat(e uint32, s uint32, f uint32, n uint32, inv uint32, ign uint32, del uint32, dlst uint32, ins uint32, insfail uint32, drop uint32, edrop uint32, ie uint32, en uint32, ec uint32, ed uint32, sr uint32) *ConntrackStat {
	return &ConntrackStat{
		Entries:       e,
		Searched:      s,
		Found:         f,
		New:           n,
		Invalid:       inv,
		Ignore:        ign,
		Delete:        del,
		DeleteList:    dlst,
		Insert:        ins,
		InsertFailed:  insfail,
		Drop:          drop,
		EarlyDrop:     edrop,
		IcmpError:     ie,
		ExpectNew:     en,
		ExpectCreate:  ec,
		ExpectDelete:  ed,
		SearchRestart: sr,
	}
}

type ConntrackStatList struct {
	items []*ConntrackStat
}

func NewConntrackStatList() *ConntrackStatList {
	return &ConntrackStatList{
		items: []*ConntrackStat{},
	}
}

func (l *ConntrackStatList) Append(c *ConntrackStat) {
	l.items = append(l.items, c)
}

func (l *ConntrackStatList) Items() []ConntrackStat {
	items := make([]ConntrackStat, len(l.items), len(l.items))
	for i, el := range l.items {
		items[i] = *el
	}
	return items
}

// Summary returns a single-element list with totals from all list items.
func (l *ConntrackStatList) Summary() []ConntrackStat {
	summary := NewConntrackStat(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	for _, cs := range l.items {
		summary.Entries += cs.Entries
		summary.Searched += cs.Searched
		summary.Found += cs.Found
		summary.New += cs.New
		summary.Invalid += cs.Invalid
		summary.Ignore += cs.Ignore
		summary.Delete += cs.Delete
		summary.DeleteList += cs.DeleteList
		summary.Insert += cs.Insert
		summary.InsertFailed += cs.InsertFailed
		summary.Drop += cs.Drop
		summary.EarlyDrop += cs.EarlyDrop
		summary.IcmpError += cs.IcmpError
		summary.ExpectNew += cs.ExpectNew
		summary.ExpectCreate += cs.ExpectCreate
		summary.ExpectDelete += cs.ExpectDelete
		summary.SearchRestart += cs.SearchRestart
	}
	return []ConntrackStat{*summary}
}

var constMap = map[string]int{
	"unix": syscall.AF_UNIX,
	"TCP":  syscall.SOCK_STREAM,
	"UDP":  syscall.SOCK_DGRAM,
	"IPv4": syscall.AF_INET,
	"IPv6": syscall.AF_INET6,
}

func (n IOCountersStat) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func (n ConnectionStat) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func (n ProtoCountersStat) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func (a Addr) String() string {
	s, _ := json.Marshal(a)
	return string(s)
}

func (n InterfaceStat) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func (n InterfaceAddr) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func (n ConntrackStat) String() string {
	s, _ := json.Marshal(n)
	return string(s)
}

func Interfaces() ([]InterfaceStat, error) {
	return InterfacesWithContext(context.Background())
}

func InterfacesWithContext(ctx context.Context) ([]InterfaceStat, error) {
	is, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	ret := make([]InterfaceStat, 0, len(is))
	for _, ifi := range is {

		var flags []string
		if ifi.Flags&net.FlagUp != 0 {
			flags = append(flags, "up")
		}
		if ifi.Flags&net.FlagBroadcast != 0 {
			flags = append(flags, "broadcast")
		}
		if ifi.Flags&net.FlagLoopback != 0 {
			flags = append(flags, "loopback")
		}
		if ifi.Flags&net.FlagPointToPoint != 0 {
			flags = append(flags, "pointtopoint")
		}
		if ifi.Flags&net.FlagMulticast != 0 {
			flags = append(flags, "multicast")
		}

		r := InterfaceStat{
			Index:        ifi.Index,
			Name:         ifi.Name,
			MTU:          ifi.MTU,
			HardwareAddr: ifi.HardwareAddr.String(),
			Flags:        flags,
		}
		addrs, err := ifi.Addrs()
		if err == nil {
			r.Addrs = make([]InterfaceAddr, 0, len(addrs))
			for _, addr := range addrs {
				r.Addrs = append(r.Addrs, InterfaceAddr{
					Addr: addr.String(),
				})
			}

		}
		ret = append(ret, r)
	}

	return ret, nil
}

func getIOCountersAll(n []IOCountersStat) ([]IOCountersStat, error) {
	r := IOCountersStat{
		Name: "all",
	}
	for _, nic := range n {
		r.BytesRecv += nic.BytesRecv
		r.PacketsRecv += nic.PacketsRecv
		r.Errin += nic.Errin
		r.Dropin += nic.Dropin
		r.BytesSent += nic.BytesSent
		r.PacketsSent += nic.PacketsSent
		r.Errout += nic.Errout
		r.Dropout += nic.Dropout
	}

	return []IOCountersStat{r}, nil
}

func parseNetLine(line string) (ConnectionStat, error) {
	f := strings.Fields(line)
	if len(f) < 8 {
		return ConnectionStat{}, fmt.Errorf("wrong line,%s", line)
	}

	if len(f) == 8 {
		f = append(f, f[7])
		f[7] = "unix"
	}

	pid, err := strconv.Atoi(f[1])
	if err != nil {
		return ConnectionStat{}, err
	}
	fd, err := strconv.Atoi(strings.Trim(f[3], "u"))
	if err != nil {
		return ConnectionStat{}, fmt.Errorf("unknown fd, %s", f[3])
	}
	netFamily, ok := constMap[f[4]]
	if !ok {
		return ConnectionStat{}, fmt.Errorf("unknown family, %s", f[4])
	}
	netType, ok := constMap[f[7]]
	if !ok {
		return ConnectionStat{}, fmt.Errorf("unknown type, %s", f[7])
	}

	var laddr, raddr Addr
	if f[7] == "unix" {
		laddr.IP = f[8]
	} else {
		laddr, raddr, err = parseNetAddr(f[8])
		if err != nil {
			return ConnectionStat{}, fmt.Errorf("failed to parse netaddr, %s", f[8])
		}
	}

	n := ConnectionStat{
		Fd:     uint32(fd),
		Family: uint32(netFamily),
		Type:   uint32(netType),
		Laddr:  laddr,
		Raddr:  raddr,
		Pid:    int32(pid),
	}
	if len(f) == 10 {
		n.Status = strings.Trim(f[9], "()")
	}

	return n, nil
}

func parseNetAddr(line string) (laddr Addr, raddr Addr, err error) {
	parse := func(l string) (Addr, error) {
		host, port, err := net.SplitHostPort(l)
		if err != nil {
			return Addr{}, fmt.Errorf("wrong addr, %s", l)
		}
		lport, err := strconv.Atoi(port)
		if err != nil {
			return Addr{}, err
		}
		return Addr{IP: host, Port: uint32(lport)}, nil
	}

	addrs := strings.Split(line, "->")
	if len(addrs) == 0 {
		return laddr, raddr, fmt.Errorf("wrong netaddr, %s", line)
	}
	laddr, err = parse(addrs[0])
	if len(addrs) == 2 { // remote addr exists
		raddr, err = parse(addrs[1])
		if err != nil {
			return laddr, raddr, err
		}
	}

	return laddr, raddr, err
}
