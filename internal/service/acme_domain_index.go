package service

import "strings"

type domainIndex struct {
	root *domainNode
}

type domainNode struct {
	children map[string]*domainNode
	exact    *acmeEntry
	wildcard *acmeEntry
}

func newDomainIndex() *domainIndex {
	return &domainIndex{root: &domainNode{}}
}

func (idx *domainIndex) insert(d string, entry *acmeEntry) (prev *acmeEntry) {
	d = strings.ToLower(strings.TrimSpace(d))
	isWildcard := false
	if strings.HasPrefix(d, "*.") {
		isWildcard = true
		d = d[2:]
	}
	if d == "" || d == "*" {
		return nil
	}
	labels := reverseLabels(d)
	if len(labels) == 0 {
		return nil
	}

	cur := idx.root
	for _, lab := range labels {
		if cur.children == nil {
			cur.children = make(map[string]*domainNode)
		}
		next, ok := cur.children[lab]
		if !ok {
			next = &domainNode{}
			cur.children[lab] = next
		}
		cur = next
	}
	if isWildcard {
		prev = cur.wildcard
		cur.wildcard = entry
	} else {
		prev = cur.exact
		cur.exact = entry
	}
	return prev
}

func (idx *domainIndex) lookup(name string) *acmeEntry {
	if idx == nil || idx.root == nil || name == "" {
		return nil
	}
	labels := reverseLabels(name)
	if len(labels) == 0 {
		return nil
	}

	cur := idx.root
	var inherited *acmeEntry
	for _, lab := range labels {
		if cur.wildcard != nil {
			inherited = cur.wildcard
		}
		next, ok := cur.children[lab]
		if !ok {
			return inherited
		}
		cur = next
	}
	if cur.exact != nil {
		return cur.exact
	}
	return inherited
}

func reverseLabels(s string) []string {
	s = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return parts
}
