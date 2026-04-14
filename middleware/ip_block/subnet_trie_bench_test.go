package ip_block

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tier limits from AbuseIPDB docs.
// https://docs.abuseipdb.com/#blacklist-endpoint
const (
	standardTier = 10_000
	basicTier    = 100_000
	premiumTier  = 500_000
)

func randomIPv4() netip.Addr {
	var b [4]byte
	b[0] = byte(rand.IntN(224)) + 1 // 1-224, avoid 0.x.x.x and multicast
	b[1] = byte(rand.IntN(256))
	b[2] = byte(rand.IntN(256))
	b[3] = byte(rand.IntN(256))
	return netip.AddrFrom4(b)
}

func buildTrie(n int) (*SubnetTrie, []netip.Addr) {
	trie := NewSubnetTrie()
	addrs := make([]netip.Addr, n)
	for i := range addrs {
		addr := randomIPv4()
		addrs[i] = addr
		trie.InsertIP(addr)
	}
	return trie, addrs
}

// --- Benchmarks: Insert ---

func BenchmarkTrieInsert_Standard(b *testing.B) {
	benchmarkTrieInsert(b, standardTier)
}

func BenchmarkTrieInsert_Basic(b *testing.B) {
	benchmarkTrieInsert(b, basicTier)
}

func BenchmarkTrieInsert_Premium(b *testing.B) {
	benchmarkTrieInsert(b, premiumTier)
}

func benchmarkTrieInsert(b *testing.B, n int) {
	addrs := make([]netip.Addr, n)
	for i := range addrs {
		addrs[i] = randomIPv4()
	}

	b.ResetTimer()
	for range b.N {
		trie := NewSubnetTrie()
		for _, addr := range addrs {
			trie.InsertIP(addr)
		}
	}
}

func BenchmarkTrieLookupHit_Standard(b *testing.B) {
	benchmarkTrieLookupHit(b, standardTier)
}

func BenchmarkTrieLookupHit_Basic(b *testing.B) {
	benchmarkTrieLookupHit(b, basicTier)
}

func BenchmarkTrieLookupHit_Premium(b *testing.B) {
	benchmarkTrieLookupHit(b, premiumTier)
}

func benchmarkTrieLookupHit(b *testing.B, n int) {
	trie, addrs := buildTrie(n)
	b.ResetTimer()
	for i := range b.N {
		trie.Contains(addrs[i%len(addrs)])
	}
}

func BenchmarkTrieLookupMiss_Standard(b *testing.B) {
	benchmarkTrieLookupMiss(b, standardTier)
}

func BenchmarkTrieLookupMiss_Basic(b *testing.B) {
	benchmarkTrieLookupMiss(b, basicTier)
}

func BenchmarkTrieLookupMiss_Premium(b *testing.B) {
	benchmarkTrieLookupMiss(b, premiumTier)
}

func benchmarkTrieLookupMiss(b *testing.B, n int) {
	trie, _ := buildTrie(n)
	// generate addresses that are very unlikely in the trie
	miss := make([]netip.Addr, 1000)
	for i := range miss {
		miss[i] = randomIPv4()
	}
	b.ResetTimer()
	for i := range b.N {
		trie.Contains(miss[i%len(miss)])
	}
}

// --- Correctness at scale ---

func TestSubnetTrie_AllTiers(t *testing.T) {
	for _, tier := range []struct {
		name  string
		count int
	}{
		{"Standard", standardTier},
		{"Basic", basicTier},
		{"Premium", premiumTier},
	} {
		t.Run(fmt.Sprintf("%s_%d_IPs", tier.name, tier.count), func(t *testing.T) {
			trie, addrs := buildTrie(tier.count)

			// spot-check: every 1000th inserted IP must be found
			for i := 0; i < len(addrs); i += 1000 {
				assert.True(t, trie.Contains(addrs[i]),
					"expected trie to contain %s", addrs[i])
			}
		})
	}
}
