package ip_block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIpList_MixedIPsAndSubnets(t *testing.T) {
	// Feed it plain IPs and CIDRs — one structure handles both
	entries := []string{
		"192.168.1.53",    // single IP  → /32
		"10.0.0.0/8",      // subnet
		"172.16.0.0/12",   // subnet
		"8.8.8.8",         // single IP  → /32
	}

	list, err := buildIpList(entries)
	require.NoError(t, err)

	// Exact IP matches
	ok, _ := list.Contains("192.168.1.53")
	assert.True(t, ok, "exact IP should match")

	ok, _ = list.Contains("8.8.8.8")
	assert.True(t, ok, "exact IP should match")

	// Subnet matches
	ok, _ = list.Contains("10.0.0.1")
	assert.True(t, ok, "should be inside 10.0.0.0/8")

	ok, _ = list.Contains("10.255.255.255")
	assert.True(t, ok, "should be inside 10.0.0.0/8")

	ok, _ = list.Contains("172.20.5.1")
	assert.True(t, ok, "should be inside 172.16.0.0/12")

	// Non-matches
	ok, _ = list.Contains("192.168.1.54")
	assert.False(t, ok, "different IP, not in any subnet")

	ok, _ = list.Contains("1.1.1.1")
	assert.False(t, ok, "not in any entry")

	ok, _ = list.Contains("172.32.0.1")
	assert.False(t, ok, "outside 172.16.0.0/12")
}

func TestIpList_EmptyList(t *testing.T) {
	list, err := buildIpList([]string{})
	require.NoError(t, err)

	ok, _ := list.Contains("1.2.3.4")
	assert.False(t, ok)
}

func TestIpList_InvalidEntry(t *testing.T) {
	_, err := buildIpList([]string{"not-valid"})
	assert.Error(t, err)
}
