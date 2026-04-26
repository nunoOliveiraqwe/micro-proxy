package resolver

import (
	"fmt"
	"time"
)

type ProxyPresetResolver interface {
	Resolve() []string
	DefaultHeader() string
	DefaultRefresh() time.Duration
}
type resolverRegistry map[string]ProxyPresetResolver

var registry = make(resolverRegistry)

func AddProxyResolver(key string, p ProxyPresetResolver) error {
	return registry.addProxyResolver(key, p)
}

func GetProxyResolver(key string) (ProxyPresetResolver, error) {
	return registry.getProxyResolver(key)
}

func ContainsPreset(key string) bool {
	return registry.containsPreset(key)
}

func GetAllAvailablePresets() []string {
	return registry.getAllAvailablePresets()
}

func (r resolverRegistry) addProxyResolver(key string, p ProxyPresetResolver) error {
	if key == "" {
		return fmt.Errorf("resolver key cannot be nil")
	}
	_, ok := r[key]
	if ok {
		return fmt.Errorf("resolver with key %s already exists", key)
	}
	r[key] = p
	return nil
}

func (r resolverRegistry) getProxyResolver(key string) (ProxyPresetResolver, error) {
	if key == "" {
		return nil, fmt.Errorf("resolver key cannot be nil")
	}
	resolver, ok := r[key]
	if !ok {
		return nil, fmt.Errorf("resolver with key %s not found", key)
	}
	return resolver, nil
}

func (r resolverRegistry) containsPreset(key string) bool {
	if key == "" {
		return false
	}
	_, ok := r[key]
	return ok
}

func (r resolverRegistry) getAllAvailablePresets() []string {
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	return keys
}
