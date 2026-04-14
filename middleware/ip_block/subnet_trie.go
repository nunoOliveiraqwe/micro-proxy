package ip_block

import (
	"fmt"
	"net/netip"
	"sync"
)

// SubnetTrie is a binary trie (prefix tree) for fast subnet matching.
//
// Instead of iterating through all subnets and calling Contains() on each — O(n),
// this walks the bit-representation of an IP address down a trie — O(k),
// where k is the number of bits (32 for IPv4, 128 for IPv6). Effectively constant time.
//
// This is the same algorithm routers use for longest-prefix matching.
// AI GENERATED, thanks claudio
type SubnetTrie struct {
	mu sync.RWMutex
	v4 *trieNode
	v6 *trieNode
}

type trieNode struct {
	children [2]*trieNode
	terminal bool
}

func NewSubnetTrie() *SubnetTrie {
	return &SubnetTrie{
		v4: &trieNode{},
		v6: &trieNode{},
	}
}

func (t *SubnetTrie) Insert(prefix netip.Prefix) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prefix = prefix.Masked() // normalize: zero out host bits
	addr := prefix.Addr()
	bits := prefix.Bits()

	var raw []byte
	var root *trieNode

	if addr.Is4() {
		a4 := addr.As4()
		raw = a4[:]
		root = t.v4
	} else {
		a16 := addr.As16()
		raw = a16[:]
		root = t.v6
	}

	node := root
	for i := 0; i < bits; i++ {
		byteIdx := i / 8
		bitIdx := uint(7 - (i % 8))
		bit := (raw[byteIdx] >> bitIdx) & 1

		if node.children[bit] == nil {
			node.children[bit] = &trieNode{}
		}
		node = node.children[bit]

		if node.terminal {
			return // a broader subnet already covers this range
		}
	}
	node.terminal = true

	node.children = [2]*trieNode{}
}

func (t *SubnetTrie) Contains(addr netip.Addr) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var raw []byte
	var root *trieNode

	if addr.Is4() {
		a4 := addr.As4()
		raw = a4[:]
		root = t.v4
	} else {
		a16 := addr.As16()
		raw = a16[:]
		root = t.v6
	}

	totalBits := len(raw) * 8
	node := root
	for i := 0; i < totalBits; i++ {
		byteIdx := i / 8
		bitIdx := uint(7 - (i % 8))
		bit := (raw[byteIdx] >> bitIdx) & 1

		child := node.children[bit]
		if child == nil {
			return false
		}
		if child.terminal {
			return true // matched a prefix
		}
		node = child
	}
	return false
}

// InsertIP adds a single IP address to the trie (as a /32 or /128 prefix).
func (t *SubnetTrie) InsertIP(addr netip.Addr) {
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	t.Insert(netip.PrefixFrom(addr, bits))
}

// InsertFromString parses a string as either a plain IP or CIDR and inserts it.
// Accepts both "1.2.3.4" and "10.0.0.0/8".
func (t *SubnetTrie) InsertFromString(ipOrCIDR string) error {
	// Try CIDR first
	if prefix, err := netip.ParsePrefix(ipOrCIDR); err == nil {
		t.Insert(prefix)
		return nil
	}
	// Fall back to plain IP
	addr, err := netip.ParseAddr(ipOrCIDR)
	if err != nil {
		return fmt.Errorf("invalid IP or CIDR: %s", ipOrCIDR)
	}
	t.InsertIP(addr)
	return nil
}


func (t *SubnetTrie) ContainsFromString(ipStr string) (bool, error) {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false, err
	}
	return t.Contains(addr), nil
}
