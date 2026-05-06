package resolve

import (
	"fmt"
	"strings"
)

type ResolverRegistry struct {
	valueResolverRegistry   map[string]ValueResolver
	requestResolverRegistry map[string]RequestResolver
}

type ResolverInfo struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

var registry *ResolverRegistry

func init() {
	registry = &ResolverRegistry{
		valueResolverRegistry:   make(map[string]ValueResolver),
		requestResolverRegistry: make(map[string]RequestResolver),
	}
	f := FileResolver{}
	e := EnvResolver{}
	registry.valueResolverRegistry["$file"] = &f
	registry.valueResolverRegistry["$env"] = &e

	for k, v := range requestVars {
		registry.requestResolverRegistry[k] = v
	}
}

func GetRequestResolver(key string) RequestResolver {
	if key == "" {
		return nil
	}
	return registry.requestResolverRegistry[key]
}

func ResolveValue(raw string) (string, error) { //acceptable formats:
	// $env:VAR_NAME
	// $file:/path/to/file

	if strings.HasPrefix(raw, "$") {
		if len(raw) == 1 {
			return "", fmt.Errorf("invalid resolver format: %s", raw)
		}
	} else {
		//not a resolver, return as is
		return raw, nil
	}

	firstIndexOf := strings.Index(raw, ":")
	if firstIndexOf == -1 {
		return "", fmt.Errorf("invalid resolver format, missing ':': %s", raw)
	}
	key := raw[:firstIndexOf]
	value := raw[firstIndexOf+1:]

	resolver := registry.valueResolverRegistry[key]

	if resolver == nil {
		return "", fmt.Errorf("no resolver registered for %q", key)
	}
	return resolver.Resolve(value)
}

var requestVarInfo = []ResolverInfo{
	{Key: "$remote_addr", Description: "Client network address from the request, including the source port."},
	{Key: "$host", Description: "Host requested by the client, usually from the Host header."},
	{Key: "$method", Description: "HTTP method used by the request, such as GET or POST."},
	{Key: "$uri", Description: "Request URI sent by the client, including path and query string."},
	{Key: "$scheme", Description: "Request scheme inferred by Torii: http or https."},
}

var valueResolverInfo = []ResolverInfo{
	{Key: "$env:ENV_VAR", Description: "Value of the specified environment variable."},
	{Key: "$file:/path/to/file", Description: "Contents of the specified file. (Docker secrets like /run/secrets/* are supported.)"},
}

func GetAllResolverInfo() []ResolverInfo {
	return append(requestVarInfo, valueResolverInfo...)
}
