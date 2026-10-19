package util

import "net/netip"

// blockedInternalPrefixes are internal or reserved ranges not covered by the netip.Addr predicates.
var blockedInternalPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

var (
	nat64Prefix      = netip.MustParsePrefix("64:ff9b::/96")
	nat64LocalPrefix = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFourPrefix  = netip.MustParsePrefix("2002::/16")
)

// IsBlockedInternalIP reports whether an address is internal and must not be reached by requests to
// user-controlled URLs. Validation and dialing should both use it so they share the same policy.
func IsBlockedInternalIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	for _, prefix := range blockedInternalPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	// NAT64 gateways and 6to4 relays translate these to their embedded IPv4 address, so that address
	// determines where the request actually goes.
	if nat64LocalPrefix.Contains(ip) {
		return true
	}
	if nat64Prefix.Contains(ip) {
		b := ip.As16()
		return IsBlockedInternalIP(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	if sixToFourPrefix.Contains(ip) {
		b := ip.As16()
		return IsBlockedInternalIP(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
	}
	return false
}
