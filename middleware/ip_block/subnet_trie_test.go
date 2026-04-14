package ip_block

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubnetTrie_IPv4_SingleSubnet(t *testing.T) {
	trie := NewSubnetTrie()
	trie.Insert(netip.MustParsePrefix("192.168.1.0/24"))

	assert.True(t, trie.Contains(netip.MustParseAddr("192.168.1.1")))
	assert.True(t, trie.Contains(netip.MustParseAddr("192.168.1.254")))
	assert.True(t, trie.Contains(netip.MustParseAddr("192.168.1.0")))
	assert.False(t, trie.Contains(netip.MustParseAddr("192.168.2.1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("10.0.0.1")))
}

func TestSubnetTrie_IPv4_MultipleSubnets(t *testing.T) {
	trie := NewSubnetTrie()
	trie.Insert(netip.MustParsePrefix("192.168.1.0/24"))
	trie.Insert(netip.MustParsePrefix("10.0.0.0/8"))
	trie.Insert(netip.MustParsePrefix("172.16.0.0/12"))

	// In 192.168.1.0/24
	assert.True(t, trie.Contains(netip.MustParseAddr("192.168.1.53")))

	// In 10.0.0.0/8
	assert.True(t, trie.Contains(netip.MustParseAddr("10.0.0.1")))
	assert.True(t, trie.Contains(netip.MustParseAddr("10.255.255.255")))

	// In 172.16.0.0/12
	assert.True(t, trie.Contains(netip.MustParseAddr("172.16.0.1")))
	assert.True(t, trie.Contains(netip.MustParseAddr("172.31.255.255")))

	// Not in any
	assert.False(t, trie.Contains(netip.MustParseAddr("192.168.2.1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("8.8.8.8")))
	assert.False(t, trie.Contains(netip.MustParseAddr("172.32.0.1")))
}

func TestSubnetTrie_BroaderPrefixSubsumesNarrower(t *testing.T) {
	trie := NewSubnetTrie()
	// Insert narrow first, then broad
	trie.Insert(netip.MustParsePrefix("10.0.1.0/24"))
	trie.Insert(netip.MustParsePrefix("10.0.0.0/8"))

	// The /8 covers everything, including things outside the /24
	assert.True(t, trie.Contains(netip.MustParseAddr("10.0.1.5")))
	assert.True(t, trie.Contains(netip.MustParseAddr("10.99.99.99")))
	assert.False(t, trie.Contains(netip.MustParseAddr("11.0.0.1")))
}

func TestSubnetTrie_NarrowAfterBroadIsNoOp(t *testing.T) {
	trie := NewSubnetTrie()
	// Insert broad first, then narrow — narrow should short-circuit
	trie.Insert(netip.MustParsePrefix("10.0.0.0/8"))
	trie.Insert(netip.MustParsePrefix("10.0.1.0/24"))

	assert.True(t, trie.Contains(netip.MustParseAddr("10.0.1.5")))
	assert.True(t, trie.Contains(netip.MustParseAddr("10.99.99.99")))
}

func TestSubnetTrie_SingleHost(t *testing.T) {
	trie := NewSubnetTrie()
	trie.Insert(netip.MustParsePrefix("1.2.3.4/32"))

	assert.True(t, trie.Contains(netip.MustParseAddr("1.2.3.4")))
	assert.False(t, trie.Contains(netip.MustParseAddr("1.2.3.5")))
}

func TestSubnetTrie_IPv6(t *testing.T) {
	trie := NewSubnetTrie()
	trie.Insert(netip.MustParsePrefix("fd00::/8"))

	assert.True(t, trie.Contains(netip.MustParseAddr("fd00::1")))
	assert.True(t, trie.Contains(netip.MustParseAddr("fdff::1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("fe80::1")))
}

func TestSubnetTrie_MixedIPv4AndIPv6(t *testing.T) {
	trie := NewSubnetTrie()
	trie.Insert(netip.MustParsePrefix("10.0.0.0/8"))
	trie.Insert(netip.MustParsePrefix("fd00::/8"))

	assert.True(t, trie.Contains(netip.MustParseAddr("10.0.0.1")))
	assert.True(t, trie.Contains(netip.MustParseAddr("fd00::1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("192.168.1.1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("fe80::1")))
}

func TestSubnetTrie_InsertFromString(t *testing.T) {
	trie := NewSubnetTrie()
	require.NoError(t, trie.InsertFromString("192.168.0.0/16"))

	ok, err := trie.ContainsFromString("192.168.1.1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = trie.ContainsFromString("10.0.0.1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSubnetTrie_InsertFromString_InvalidInput(t *testing.T) {
	trie := NewSubnetTrie()
	assert.Error(t, trie.InsertFromString("not-a-cidr"))

	_, err := trie.ContainsFromString("not-an-ip")
	assert.Error(t, err)
}

func TestSubnetTrie_EmptyTrie(t *testing.T) {
	trie := NewSubnetTrie()
	assert.False(t, trie.Contains(netip.MustParseAddr("1.2.3.4")))
	assert.False(t, trie.Contains(netip.MustParseAddr("::1")))
}

func TestSubnetTrie_InsertIP(t *testing.T) {
	trie := NewSubnetTrie()
	trie.InsertIP(netip.MustParseAddr("1.2.3.4"))

	assert.True(t, trie.Contains(netip.MustParseAddr("1.2.3.4")))
	assert.False(t, trie.Contains(netip.MustParseAddr("1.2.3.5")))
}

func TestSubnetTrie_InsertFromString_PlainIP(t *testing.T) {
	trie := NewSubnetTrie()
	require.NoError(t, trie.InsertFromString("1.2.3.4"))     // plain IP, not CIDR
	require.NoError(t, trie.InsertFromString("10.0.0.0/8"))   // CIDR

	// Plain IP match
	assert.True(t, trie.Contains(netip.MustParseAddr("1.2.3.4")))
	assert.False(t, trie.Contains(netip.MustParseAddr("1.2.3.5")))

	// Subnet match
	assert.True(t, trie.Contains(netip.MustParseAddr("10.0.0.1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("11.0.0.1")))
}

func TestSubnetTrie_InsertFromString_PlainIPv6(t *testing.T) {
	trie := NewSubnetTrie()
	require.NoError(t, trie.InsertFromString("::1"))

	assert.True(t, trie.Contains(netip.MustParseAddr("::1")))
	assert.False(t, trie.Contains(netip.MustParseAddr("::2")))
}

