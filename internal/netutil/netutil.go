package netutil

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

func GetNetworkIfByName(ifName string) (*net.Interface, error) {
	zap.S().Debugf("Fetching network if by name %s", ifName)
	return net.InterfaceByName(ifName)
}

func GetLoopBackInterface() (*net.Interface, error) {
	zap.S().Debugf("Fetching loopback interface")
	l, err := net.InterfaceByName("lo")
	if err != nil {
		l, err = net.InterfaceByName("lo0")
		if err != nil {
			return nil, err
		}
		return l, nil
	}
	return l, nil
}

func GetNetworkBindAddressesFromInterface(ifName string) (ipv4, ipv6 string, err error) {
	zap.S().Debugf("Fetching network bind addresses from interface %s", ifName)
	iface, err := GetNetworkIfByName(ifName)
	if err != nil {
		return "", "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		zap.S().Warnf("failed to get addresses for interface %s: %v", ifName, err)
		return "", "", err
	}
	if addrs == nil || len(addrs) == 0 {
		return "", "", fmt.Errorf("no addresses found for interface %s", ifName)
	}

	for _, addr := range addrs {
		zap.S().Debugf("Checking address %s for interface %s", addr.String(), ifName)
		var ip net.IP

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			zap.S().Warnf("unsupported address type %T for interface %s", addr, ifName)
			continue
		}

		if ip == nil || ip.IsLinkLocalUnicast() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			if ipv4 == "" {
				ipv4 = v4.String()
			}
			continue
		}
		if ip.To16() != nil && ipv6 == "" && ip.IsLinkLocalUnicast() == false {
			ipv6 = ip.String()
		}
		if ipv4 != "" && ipv6 != "" {
			break
		}
	}

	if ipv4 == "" && ipv6 == "" {
		return "", "", fmt.Errorf("no valid ip addresses found for interface %s", ifName)
	}
	if ipv4 == "" || ipv6 == "" {
		zap.S().Warnf("interface %s is missing one address family (ipv4=%q, ipv6=%q)", ifName, ipv4, ipv6)
	}
	return ipv4, ipv6, nil
}

func GetIpFromRequestFromXForwarded(r *http.Request) (string, error) {
	zap.S().Debug("Fetching IP address from request")
	ips := r.Header.Get("X-Forwarded-For")
	splitIps := strings.Split(ips, ",")

	if len(splitIps) > 0 {
		zap.S().Debug("IP addresses found in X-Forwarded-For header: %v", splitIps)
		netIP := net.ParseIP(splitIps[len(splitIps)-1])
		if netIP != nil {
			return netIP.String(), nil
		}
	}
	return GetClientIP(r)
}

func GetClientIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("failed to parse RemoteAddr %q: %w", r.RemoteAddr, err)
	}
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}
	return ip, nil
}

func SplitHostPort(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0
	}
	return host, port
}
