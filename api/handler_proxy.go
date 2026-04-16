package api

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	mw "github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleGetProxies(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		logger.Debug("Fetching configured proxy servers")
		proxies := systemService.GetConfiguredProxyServers()
		if proxies == nil {
			logger.Error("Failed to retrieve configured proxy servers")
			http.Error(writer, "Failed to retrieve configured proxy servers", http.StatusInternalServerError)
			return
		}
		logger.Debug("Retrieved configured proxy servers", zap.Int("count", len(proxies)))
		WriteResponseAsJSON(proxies, writer)
	}
}

func handleStartProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		port := request.PathValue("serverId")
		logger.Info("Starting proxy server", zap.String("port", port))
		portInt, err := strconv.Atoi(port)
		if err != nil {
			logger.Error("Invalid port format", zap.String("port", port))
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StartProxy(portInt)
		if err != nil {
			logger.Error("Failed to start proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to start proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "started"}, writer)
	}
}

func handleStopProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		port := request.PathValue("serverId")
		logger.Info("Stopping proxy server", zap.String("port", port))
		portInt, err := strconv.Atoi(port)
		if err != nil {
			logger.Error("Invalid port format", zap.String("port", port))
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StopProxy(portInt)
		if err != nil {
			logger.Error("Failed to stop proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to stop proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "stopped"}, writer)
	}
}

func handleDeleteProxy(service app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		port := request.PathValue("serverId")
		logger.Info("Deleting proxy server", zap.String("port", port))
		portInt, err := strconv.Atoi(port)
		if err != nil {
			logger.Error("Invalid port format", zap.String("port", port))
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		// TODO: persist deletion to the config file so removed routes don't reappear on restart.
		err = service.DeleteProxy(portInt)
		if err != nil {
			logger.Error("Failed to delete proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to delete proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "deleted"}, writer)
	}
}

func handleGetGlobalMetrics(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		logger.Info("Fetching global proxy metrics")
		globalMetrics := systemService.GetGlobalMetricsManager().GetGlobalMetrics()
		WriteResponseAsJSON(globalMetrics, writer)
	}
}

func handleGetNetworkInterfaces(_ app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		ifaces, err := net.Interfaces()
		if err != nil {
			http.Error(writer, "Failed to list network interfaces", http.StatusInternalServerError)
			return
		}
		var result []NetworkInterfaceDTO
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
			ipv4, ipv6, _ := netutil.GetNetworkBindAddressesFromInterface(iface.Name)
			if ipv4 == "" && ipv6 == "" {
				continue
			}
			result = append(result, NetworkInterfaceDTO{
				Name: iface.Name,
				IPv4: ipv4,
				IPv6: ipv6,
			})
		}
		WriteResponseAsJSON(result, writer)
	}
}

func handleCreateHttpProxyServer(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := mw.GetRequestLoggerFromContext(r)
		req, err := DecodeJSONBody[CreateProxyServerRequest](r)
		if err != nil {
			logger.Error("Failed to decode listener request", zap.Error(err))
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Port <= 0 || req.Port > 65535 {
			http.Error(w, "Invalid port number", http.StatusBadRequest)
			return
		}
		if req.Bind < 1 || req.Bind > 3 {
			http.Error(w, "Invalid bind flag (1=IPv4, 2=IPv6, 3=both)", http.StatusBadRequest)
			return
		}
		if len(req.Routes) == 0 && req.Default == nil {
			http.Error(w, "At least one route or a default route is required", http.StatusBadRequest)
			return
		}
		conf, err := convertToHTTPListener(req)
		if err != nil {
			logger.Error("Invalid listener configuration", zap.Error(err))
			http.Error(w, "Invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		// TODO: persist the new listener to the config file so it survives restarts.
		// Currently this is memory-only — the UI-created route is lost on restart.
		if err := svc.AddHttpListener(conf); err != nil {
			logger.Error("Failed to add HTTP listener", zap.Error(err))
			http.Error(w, "Failed to create listener: "+err.Error(), http.StatusInternalServerError)
			return
		}
		logger.Info("HTTP listener created", zap.Int("port", req.Port))
		WriteResponseAsJSON(map[string]interface{}{
			"status": "created",
			"port":   req.Port,
		}, w)
	}
}

func convertToHTTPListener(req *CreateProxyServerRequest) (config.HTTPListener, error) {
	conf := config.HTTPListener{
		Port:      req.Port,
		Interface: req.Interface,
		Bind:      config.IpFlag(req.Bind),
	}
	if req.ReadTimeout != "" {
		d, err := time.ParseDuration(req.ReadTimeout) //i can't fucking believe this shit existed
		if err != nil {
			return conf, fmt.Errorf("invalid read_timeout %q: %w", req.ReadTimeout, err)
		}
		conf.ReadTimeout = d
	}
	if req.ReadHeaderTimeout != "" {
		d, err := time.ParseDuration(req.ReadHeaderTimeout)
		if err != nil {
			return conf, fmt.Errorf("invalid read_header_timeout %q: %w", req.ReadHeaderTimeout, err)
		}
		conf.ReadHeaderTimeout = d
	}
	if req.WriteTimeout != "" {
		d, err := time.ParseDuration(req.WriteTimeout)
		if err != nil {
			return conf, fmt.Errorf("invalid write_timeout %q: %w", req.WriteTimeout, err)
		}
		conf.WriteTimeout = d
	}
	if req.IdleTimeout != "" {
		d, err := time.ParseDuration(req.IdleTimeout)
		if err != nil {
			return conf, fmt.Errorf("invalid idle_timeout %q: %w", req.IdleTimeout, err)
		}
		conf.IdleTimeout = d
	}
	if req.TLS != nil {
		conf.TLS = &config.TLSConfig{
			UseAcme: req.TLS.UseAcme,
			Cert:    req.TLS.Cert,
			Key:     req.TLS.Key,
		}
	}
	if req.Default != nil {
		target := convertRouteTarget(*req.Default)
		conf.Default = &target
	}
	for _, r := range req.Routes {
		if r.Host == "" {
			return conf, fmt.Errorf("host-based route must have a non-empty host")
		}
		conf.Routes = append(conf.Routes, config.Route{
			Host:   r.Host,
			Target: convertRouteTarget(r.Target),
		})
	}
	return conf, nil
}

func convertRouteTarget(dto RouteTargetDTO) config.RouteTarget {
	target := config.RouteTarget{
		Backend: dto.Backend,
	}
	for _, m := range dto.Middlewares {
		target.Middlewares = append(target.Middlewares, mw.Config{
			Type:    m.Type,
			Options: m.Options,
		})
	}
	for _, p := range dto.Paths {
		pr := config.PathRule{
			Pattern:     p.Pattern,
			Backend:     p.Backend,
			DropQuery:   p.DropQuery,
			StripPrefix: p.StripPrefix,
		}
		for _, m := range p.Middlewares {
			pr.Middlewares = append(pr.Middlewares, mw.Config{
				Type:    m.Type,
				Options: m.Options,
			})
		}
		target.Paths = append(target.Paths, pr)
	}
	return target
}

func handleGetMiddlewareSchemas(_ app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		schemas := mw.GetMiddlewareSchemas()
		WriteResponseAsJSON(schemas, writer)
	}
}

func handleGetMetricForConnection(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := mw.GetRequestLoggerFromContext(request)
		serverId := request.PathValue("serverId")
		logger.Info("Fetching metric for connection", zap.String("serverId", serverId))
		metrics := systemService.GetGlobalMetricsManager().GetAllMetricsByServer(serverId)
		WriteResponseAsJSON(metrics, writer)
	}
}
