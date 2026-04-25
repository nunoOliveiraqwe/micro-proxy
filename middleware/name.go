package middleware

import (
	"context"
	"fmt"
	"strconv"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
)

func buildNameForConnection(ctx context.Context, prefix string) (string, error) {
	port := ctx.Value(ctxkeys.Port)
	if port == nil || port == "" {
		return "", fmt.Errorf("port not found in middleware options for %s resolution", prefix)
	}
	portStr, ok := port.(string)
	if !ok {
		_, isInt := port.(int)
		if !isInt {
			return "", fmt.Errorf("port is not of type string or int")
		}
		portStr = strconv.Itoa(port.(int))
	}
	hostStr := ""
	host := ctx.Value(ctxkeys.Host)
	if host != nil {
		hostStr2, ok := host.(string)
		if ok {
			hostStr = hostStr2
		}
	}
	pathStr := ""
	path := ctx.Value(ctxkeys.Path)
	if path != nil {
		pathStr2, ok := path.(string)
		if ok {
			pathStr = pathStr2
		}
	}
	conName := ProxyHostPathName(prefix, portStr, hostStr, pathStr)
	return conName, nil
}

func ProxyPathName(prefix, port, path string) string {
	if path == "" {
		return ProxyName(prefix, port)
	}
	return fmt.Sprintf("%s-port-%s-path-%s", prefix, port, path)
}

func ProxyHostPathName(prefix, port, host, path string) string {
	if host == "" {
		return ProxyPathName(prefix, port, path)
	}
	if path == "" {
		return ProxyHostName(prefix, port, host)
	}
	return fmt.Sprintf("%s-port-%s-host-%s-path-%s", prefix, port, host, path)
}

func ProxyHostName(prefix, port, host string) string {
	if host == "" {
		return ProxyName(prefix, port)
	}
	return fmt.Sprintf("%s-port-%s-host-%s", prefix, port, host)
}

func ProxyName(prefix, port string) string {
	return fmt.Sprintf("%s-port-%s", prefix, port)
}
