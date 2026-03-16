package util

import (
	"fmt"
	"net"

	"go.uber.org/zap"
)

func GetNetworkIfByName(ifName string) (*net.Interface, error) {
	zap.S().Debugf("Fetching network if by name %s", ifName)
	return net.InterfaceByName(ifName)
}

func GetNetworkBindAddressesFromInterface(ifName string) (ipv4, ipv6 string, err error) { //named params to piss the golang community somehow
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
		zap.S().Errorf("No addresses found for interface %s", ifName)
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

		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			if ipv4 == "" {
				ipv4 = v4.String()
			}
			continue
		}
		if ip.To16() != nil && ipv6 == "" {
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
