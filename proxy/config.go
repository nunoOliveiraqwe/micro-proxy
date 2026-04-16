package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"go.uber.org/zap"
)

func buildHandlerChain(ctx context.Context, serverId string, conf config.HTTPListener, global *config.GlobalConfig) (http.Handler, []string, []string, []RouteSnapshot, error) {
	ctx = context.WithValue(ctx, ctxkeys.Port, conf.Port)
	ctx = context.WithValue(ctx, ctxkeys.ServerID, serverId)

	hostHandler, mwNames, backends, routeSnapshots, err := buildHostDispatcher(ctx, conf.Default, conf.Routes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to build host dispatcher: %w", err)
	}

	//global mw → route mw → path mw → proxy
	handler, err := buildGlobalDispatcher(ctx, global, hostHandler)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to build global dispatcher: %w", err)
	}
	return handler, mwNames, backends, routeSnapshots, nil
}

func buildHttpServer(ctx context.Context, conf config.HTTPListener, global *config.GlobalConfig) (MicroHttpServer, error) {
	zap.S().Infof("Building HTTP server on port %d", conf.Port)
	zap.S().Info("Middleware order apply is global mw → route mw → path mw → proxy")
	ifFace := "lo"
	if conf.Interface == "" {
		zap.S().Warn("No interface in configuration, defaulting to loopback")
		i, err := netutil.GetLoopBackInterface()
		if err != nil {
			return nil, fmt.Errorf("failed to get loopback interface: %w", err)
		}
		ifFace = i.Name
	} else {
		ifFace = conf.Interface
	}

	ipv4, ipv6, err := netutil.GetNetworkBindAddressesFromInterface(ifFace)
	if err != nil {
		return nil, fmt.Errorf("failed to get bind addresses from interface %s: %w", ifFace, err)
	}
	if conf.Bind&config.Ipv4Flag != 0 && ipv4 == "" {
		return nil, fmt.Errorf("IPv4 bind interface %s has no valid IPv4 address", conf.Interface)
	}
	if conf.Bind&config.Ipv6Flag != 0 && ipv6 == "" {
		return nil, fmt.Errorf("IPv6 bind interface %s has no valid IPv6 address", conf.Interface)
	}
	serverId := fmt.Sprintf("http-%d", conf.Port)

	handler, mwNames, backends, routeSnapshots, err := buildHandlerChain(ctx, serverId, conf, global)
	if err != nil {
		return nil, err
	}

	zap.S().Infof("Built HTTP server IPv4=%s IPv6=%s Port=%d", ipv4, ipv6, conf.Port)

	if conf.TLS != nil {
		return &ToriiHttpsServer{
			handler:           NewSwappableHandler(handler),
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
			disableHTTP2:      conf.DisableHTTP2,
			middlewareChain:   mwNames,
			backends:          backends,
			routes:            routeSnapshots,
		}, nil
	}

	return &ToriiHttpServer{
		handler:           NewSwappableHandler(handler),
		serverId:          serverId,
		readTimeout:       conf.ReadTimeout,
		readHeaderTimeout: conf.ReadHeaderTimeout,
		writeTimeout:      conf.WriteTimeout,
		idleTimeout:       conf.IdleTimeout,
		isStarted:         atomic.Bool{},
		bindPort:          conf.Port,
		iPV4BindInterface: ipv4,
		iPV6BindInterface: ipv6,
		disableH2C:        conf.DisableHTTP2,
		middlewareChain:   mwNames,
		backends:          backends,
		routes:            routeSnapshots,
	}, nil
}
