package trustedproxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/trustedproxy/resolver"
	"go.uber.org/zap"
)

func NewTrustedProxyResolver(cidrs []string, header string) (*Matcher, error) {
	trie := buildTrieTree(cidrs)
	if trie.IsEmpty() {
		return nil, fmt.Errorf("no valid trusted proxy CIDRs provided")
	}

	h := header
	if h == "" {
		h = "X-Forwarded-For"
	}

	r := &Matcher{header: h}
	r.trusted.Store(trie)
	return r, nil
}

func NewTrustedProxyResolverFromPreset(ctx context.Context, presetName string, extraCIDRs []string, header string, refreshInterval time.Duration) (*Matcher, error) {
	if presetName == "" {
		return NewTrustedProxyResolver(extraCIDRs, header)
	}

	preset, err := resolver.GetProxyResolver(strings.ToLower(presetName))
	if err != nil {
		return nil, fmt.Errorf("unknown trusted-proxy preset %q: %w", presetName, err)
	}

	effectiveHeader := header
	if effectiveHeader == "" {
		effectiveHeader = preset.DefaultHeader()
	}

	effectiveRefresh := refreshInterval
	if effectiveRefresh == 0 {
		effectiveRefresh = preset.DefaultRefresh()
	}

	remoteCIDRs := preset.Resolve()
	allCIDRs := append(remoteCIDRs, extraCIDRs...)
	trie := buildTrieTree(allCIDRs)
	if trie.IsEmpty() {
		return nil, fmt.Errorf("trusted-proxy preset %q: no valid CIDRs after fetch", presetName)
	}

	m := &Matcher{header: effectiveHeader}
	m.trusted.Store(trie)

	zap.S().Infof("trusted-proxy: preset %q loaded, header=%s, refresh=%s",
		presetName, effectiveHeader, effectiveRefresh)

	go func() {
		ticker := time.NewTicker(effectiveRefresh)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				zap.S().Infof("trusted-proxy: refresh goroutine for preset %q stopped", presetName)
				return
			case <-ticker.C:
				zap.S().Infof("trusted-proxy: refreshing preset %q CIDRs", presetName)
				fresh := preset.Resolve()
				if len(fresh) == 0 {
					zap.S().Errorf("trusted-proxy: refresh returned empty for preset %q — keeping old list", presetName)
					continue
				}
				merged := append(fresh, extraCIDRs...)
				newTrie := buildTrieTree(merged)
				if newTrie.IsEmpty() {
					zap.S().Errorf("trusted-proxy: refreshed trie empty for preset %q — keeping old list", presetName)
					continue
				}
				m.trusted.Store(newTrie)
				zap.S().Infof("trusted-proxy: preset %q refreshed", presetName)
			}
		}
	}()

	return m, nil
}

func buildTrieTree(cidrs []string) *netutil.SubnetTrie {
	trie := netutil.NewSubnetTrie()
	for _, cidr := range cidrs {
		err := trie.InsertFromString(cidr)
		if err != nil {
			zap.S().Warnf("trusted-proxy: invalid CIDR %q skipped: %v", cidr, err)
			continue
		}
	}
	return trie
}
