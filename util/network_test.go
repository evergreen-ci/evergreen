package util

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBlockedInternalIP(t *testing.T) {
	for name, test := range map[string]struct {
		ip      string
		blocked bool
	}{
		"IPv4LinkLocalMetadataIsBlocked":         {ip: "169.254.169.254", blocked: true},
		"IPv6MetadataIsBlocked":                  {ip: "fd00:ec2::254", blocked: true},
		"IPv4LoopbackIsBlocked":                  {ip: "127.0.0.1", blocked: true},
		"IPv6LoopbackIsBlocked":                  {ip: "::1", blocked: true},
		"IPv6LinkLocalIsBlocked":                 {ip: "fe80::1", blocked: true},
		"RFC1918ClassAIsBlocked":                 {ip: "10.13.37.1", blocked: true},
		"RFC1918ClassBIsBlocked":                 {ip: "172.16.0.1", blocked: true},
		"RFC1918ClassCIsBlocked":                 {ip: "192.168.1.1", blocked: true},
		"CarrierGradeNATIsBlocked":               {ip: "100.64.0.1", blocked: true},
		"IPv6UniqueLocalIsBlocked":               {ip: "fc00::1", blocked: true},
		"IPv4UnspecifiedIsBlocked":               {ip: "0.0.0.0", blocked: true},
		"IPv4ThisNetworkIsBlocked":               {ip: "0.1.2.3", blocked: true},
		"IPv6UnspecifiedIsBlocked":               {ip: "::", blocked: true},
		"IPv4MulticastIsBlocked":                 {ip: "224.0.0.1", blocked: true},
		"IPv4BroadcastIsBlocked":                 {ip: "255.255.255.255", blocked: true},
		"IPv4ReservedIsBlocked":                  {ip: "240.0.0.1", blocked: true},
		"IPv4BenchmarkingIsBlocked":              {ip: "198.18.0.1", blocked: true},
		"IPv4ProtocolAssignmentsIsBlocked":       {ip: "192.0.0.1", blocked: true},
		"IPv4MappedPrivateIsBlocked":             {ip: "::ffff:10.0.0.1", blocked: true},
		"NAT64EmbeddedPrivateIsBlocked":          {ip: "64:ff9b::a00:1", blocked: true},
		"NAT64EmbeddedMetadataIsBlocked":         {ip: "64:ff9b::a9fe:a9fe", blocked: true},
		"NAT64LocalUseIsBlocked":                 {ip: "64:ff9b:1::a00:1", blocked: true},
		"SixToFourEmbeddedPrivateIsBlocked":      {ip: "2002:a00:1::1", blocked: true},
		"PublicIPv4IsAllowed":                    {ip: "8.8.8.8", blocked: false},
		"PublicIPv4NextToCGNATIsAllowed":         {ip: "100.128.0.1", blocked: false},
		"PublicIPv4NextToRFC1918ClassBIsAllowed": {ip: "172.32.0.1", blocked: false},
		"PublicIPv6IsAllowed":                    {ip: "2001:4860:4860::8888", blocked: false},
		"NAT64EmbeddedPublicIsAllowed":           {ip: "64:ff9b::808:808", blocked: false},
		"SixToFourEmbeddedPublicIsAllowed":       {ip: "2002:808:808::1", blocked: false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.blocked, IsBlockedInternalIP(netip.MustParseAddr(test.ip)))
		})
	}
}
