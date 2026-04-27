package acme

import (
	"strings"

	"go.uber.org/zap"
)

func (m *LegoAcmeManager) SetDomainSupplier(fn func() []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainSupplier = fn
}

func (m *LegoAcmeManager) resolveDomains() []string {

	var all []string
	all = append(all, m.conf.Domains...)

	m.mu.RLock()
	supplier := m.domainSupplier
	m.mu.RUnlock()
	if supplier != nil {
		all = append(all, supplier()...)
	}

	if len(all) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(all))
	unique := make([]string, 0, len(all))
	for _, d := range all {
		dl := strings.ToLower(d)
		if _, ok := seen[dl]; !ok {
			seen[dl] = struct{}{}
			unique = append(unique, dl)
		}
	}

	wildcardParents := make(map[string]struct{})
	for _, d := range unique {
		if strings.HasPrefix(d, "*.") {
			wildcardParents[d[2:]] = struct{}{}
		}
	}

	if len(wildcardParents) == 0 {
		zap.S().Debugf("acme: resolved %d domain(s): %v", len(unique), unique)
		return unique
	}
	result := make([]string, 0, len(unique))
	for _, d := range unique {
		if strings.HasPrefix(d, "*.") {
			result = append(result, d)
			continue
		}
		if idx := strings.Index(d, "."); idx > 0 {
			parent := d[idx+1:]
			if _, covered := wildcardParents[parent]; covered {
				zap.S().Debugf("acme: skipping %s (covered by *.%s)", d, parent)
				continue
			}
		}
		result = append(result, d)
	}
	zap.S().Debugf("acme: resolved %d domain(s): %v", len(result), result)
	return result
}

func groupDomainBatches(domains []string) [][]string {
	var batches [][]string
	parentGroups := make(map[string][]string) // parent → [sub-domains]
	groupOrder := make([]string, 0)           // preserve deterministic order

	for _, d := range domains {
		if strings.HasPrefix(d, "*.") {
			batches = append(batches, []string{d})
			continue
		}
		idx := strings.Index(d, ".")
		if idx <= 0 {
			// bare TLD or single-label name — issue individually
			batches = append(batches, []string{d})
			continue
		}
		parent := d[idx+1:]
		if _, exists := parentGroups[parent]; !exists {
			groupOrder = append(groupOrder, parent)
		}
		parentGroups[parent] = append(parentGroups[parent], d)
	}

	for _, parent := range groupOrder {
		batches = append(batches, parentGroups[parent])
	}
	return batches
}
