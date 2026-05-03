package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"go.uber.org/zap"
)

const (
	defaultReadTimeout       = 30 * time.Second
	defaultReadHeaderTimeout = 10 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 120 * time.Second
)

func applyDefaultTimeouts(conf *config.HTTPListener) {
	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = defaultReadTimeout
	}
	if conf.ReadHeaderTimeout == 0 {
		conf.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = defaultWriteTimeout
	}
	if conf.IdleTimeout == 0 {
		conf.IdleTimeout = defaultIdleTimeout
	}
}

func buildHandlerChain(ctx context.Context, serverId string, conf config.HTTPListener, dGlobal *GlobalDispatcher) (http.Handler, context.CancelFunc, []string, []RouteSnapshot, error) {
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, ctxkeys.Port, conf.Port)
	ctx = context.WithValue(ctx, ctxkeys.ServerID, serverId)

	hostHandler, backends, routeSnapshots, err := buildHostDispatcher(ctx, conf.Default, conf.Routes)
	if err != nil {
		cancel()
		return nil, nil, nil, nil, fmt.Errorf("failed to build host dispatcher: %w", err)
	}

	//global mw → route mw → path mw → proxy
	hostHandler = dGlobal.registerHandler(conf.Port, hostHandler.ServeHTTP)

	if dGlobal.globalMwNames != nil && len(dGlobal.globalMwNames) > 0 {
		for i := range routeSnapshots {
			routeSnapshots[i].GlobalMiddlewares = append([]string(nil), dGlobal.globalMwNames...)
		}
	}
	return hostHandler, cancel, backends, routeSnapshots, nil
}

func buildHttpServer(ctx context.Context, conf config.HTTPListener, dGlobal *GlobalDispatcher) (MicroHttpServer, error) {
	applyDefaultTimeouts(&conf)
	zap.S().Infof("Building HTTP server on port %d", conf.Port)
	zap.S().Info("Middleware order apply is global mw → route mw → path mw → proxy")
	var ipv4, ipv6 string
	if conf.Interface == "" {
		zap.S().Info("No interface specified, binding to all interfaces")
		ipv4 = "0.0.0.0"
		ipv6 = "::"
	} else {
		ifFace := conf.Interface
		var err error
		ipv4, ipv6, err = netutil.GetNetworkBindAddressesFromInterface(ifFace)
		if err != nil {
			return nil, fmt.Errorf("failed to get bind addresses from interface %s: %w", ifFace, err)
		}
	}
	if conf.Bind&config.Ipv4Flag != 0 && ipv4 == "" {
		return nil, fmt.Errorf("IPv4 bind interface %s has no valid IPv4 address", conf.Interface)
	}
	if conf.Bind&config.Ipv6Flag != 0 && ipv6 == "" {
		return nil, fmt.Errorf("IPv6 bind interface %s has no valid IPv6 address", conf.Interface)
	}
	serverId := fmt.Sprintf("http-%d", conf.Port)

	handler, cancelChain, backends, routeSnapshots, err := buildHandlerChain(ctx, serverId, conf, dGlobal)
	if err != nil {
		return nil, err
	}

	mwNames := collectMiddlewareNames(routeSnapshots)

	zap.S().Infof("Built HTTP server IPv4=%s IPv6=%s Port=%d", ipv4, ipv6, conf.Port)

	if conf.TLS != nil {
		return &ToriiHttpsServer{
			handler:           NewSwappableHandler(handler),
			cancelChain:       cancelChain,
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
			currentConfig:     conf,
		}, nil
	}

	return &ToriiHttpServer{
		handler:           NewSwappableHandler(handler),
		cancelChain:       cancelChain,
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
		currentConfig:     conf,
	}, nil
}
