package ip_block

import (
	"sync/atomic"

	"go.uber.org/zap"
)

type IpList struct {
	trie *SubnetTrie
}

type IpFilter struct {
	allowList *IpList
	blockList atomic.Pointer[IpList]
}

// NewIpFilter creates an IpFilter from the given loader and wires up refresh
// if the loader supports it.
//
// Logic: if IP is in the allow list → always allowed (whitelist override).
// If IP is in the block list → blocked. Otherwise → allowed.
func NewIpFilter(loader IpLoader) (*IpFilter, error) {
	allowIps, err := loader.LoadAllowedIps()
	if err != nil {
		return nil, err
	}

	allowList, err := buildIpList(allowIps)
	if err != nil {
		return nil, err
	}

	filter := &IpFilter{
		allowList: allowList,
	}

	if loader.IsRefreshable() {
		// StartRefreshTimer does the initial fetch and starts the ticker
		initialIps, err := loader.StartRefreshTimer(func(ips []string) {
			if err := filter.ReplaceBlockList(ips); err != nil {
				zap.S().Errorf("IpFilter: failed to replace block list on refresh: %v", err)
			}
		})
		if err != nil {
			return nil, err
		}
		blockList, err := buildIpList(initialIps)
		if err != nil {
			return nil, err
		}
		filter.blockList.Store(blockList)
	} else {
		blockIps, err := loader.LoadBlockedIps()
		if err != nil {
			return nil, err
		}
		blockList, err := buildIpList(blockIps)
		if err != nil {
			return nil, err
		}
		filter.blockList.Store(blockList)
	}

	return filter, nil
}

func (f *IpFilter) ReplaceBlockList(ips []string) error {
	newList, err := buildIpList(ips)
	if err != nil {
		return err
	}
	f.blockList.Store(newList)
	zap.S().Infof("IpFilter: block list replaced (%d entries)", len(ips))
	return nil
}

func (f *IpFilter) IsBlocked(ip string) (bool, error) {
	// Allow list always wins
	if f.allowList != nil && f.allowList.trie != nil {
		allowed, err := f.allowList.Contains(ip)
		if err != nil {
			return false, err
		}
		if allowed {
			return false, nil
		}
	}

	bl := f.blockList.Load()
	if bl == nil {
		return false, nil
	}
	return bl.Contains(ip)
}

func buildIpList(entries []string) (*IpList, error) {
	trie := NewSubnetTrie()
	zap.S().Debugf("Parsing %d ip/subnets", len(entries))
	for _, entry := range entries {
		if err := trie.InsertFromString(entry); err != nil {
			zap.S().Errorf("Cannot parse %s: %v", entry, err)
			return nil, err
		}
	}
	return &IpList{trie: trie}, nil
}

func (l *IpList) Contains(ip string) (bool, error) {
	return l.trie.ContainsFromString(ip)
}
