package proxy

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"go.uber.org/zap"
)

func buildHttpServer(ctx context.Context, conf config.HTTPListener, global *config.GlobalConfig) (MicroHttpServer, error) {
	zap.S().Infof("Building HTTP server on port %d", conf.Port)
	ifFace := "lo"
	if conf.Interface == "" {
		zap.S().Warn("No interface in configuration, defaulting to loopback")
		i, err := netutil.GetLoopBackInterface()
		if err != nil {
			return nil, fmt.Errorf("failed to get loopback interface: %w", err)
		}
		ifFace = i.Name
	}

	ipv4, ipv6, err := netutil.GetNetworkBindAddressesFromInterface(ifFace)
	if err != nil {
		return nil, fmt.Errorf("failed to get bind addresses from interface %s: %w", ifFace, err)
	}
	if conf.Bind&config.Ipv4Flag == 1 && ipv4 == "" {
		return nil, fmt.Errorf("IPv4 bind interface %s has no valid IPv4 address", conf.Interface)
	}
	if conf.Bind&config.Ipv6Flag == 1 && ipv6 == "" {
		return nil, fmt.Errorf("IPv6 bind interface %s has no valid IPv6 address", conf.Interface)
	}
	ctx = context.WithValue(ctx, "port", conf.Port)
	serverId := fmt.Sprintf("http-%d", conf.Port)
	ctx = context.WithValue(ctx, "serverId", serverId)
	hostHandler, mwNames, backends, routeSnapshots, err := buildHostDispatcher(ctx, conf.Default, conf.Routes)
	if err != nil {
		return nil, fmt.Errorf("failed to build host dispatcher: %w", err)
	}

	handler, err := buildGlobalDispatcher(ctx, global, hostHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to build global dispatcher: %w", err)
	}

	zap.S().Infof("Built HTTP server IPv4=%s IPv6=%s Port=%d", ipv4, ipv6, conf.Port)

	if conf.TLS != nil {
		return &ToriiHttpsServer{
			handler:           handler,
			serverId:          serverId,
			readTimeout:       conf.ReadTimeout,
			readHeaderTimeout: conf.ReadHeaderTimeout,
			writeTimeout:      conf.WriteTimeout,
			idleTimeout:       conf.IdleTimeout,
			isStarted:         atomic.Bool{},
			bindPort:          conf.Port,
			iPV4BindInterface: ipv4,
			iPV6BindInterface: ipv6,
			useAcme:           conf.TLS.UseAcme,
			keyFilePath:       conf.TLS.Key,
			certFilepath:      conf.TLS.Cert,
			middlewareChain:   mwNames,
			backends:          backends,
			routes:            routeSnapshots,
		}, nil
	}

	return &ToriiHttpServer{
		handler:           handler,
		serverId:          serverId,
		readTimeout:       conf.ReadTimeout,
		readHeaderTimeout: conf.ReadHeaderTimeout,
		writeTimeout:      conf.WriteTimeout,
		idleTimeout:       conf.IdleTimeout,
		isStarted:         atomic.Bool{},
		bindPort:          conf.Port,
		iPV4BindInterface: ipv4,
		iPV6BindInterface: ipv6,
		middlewareChain:   mwNames,
		backends:          backends,
		routes:            routeSnapshots,
	}, nil
}
