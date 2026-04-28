package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	mw "github.com/nunoOliveiraqwe/torii/middleware"
	"github.com/nunoOliveiraqwe/torii/proxy"
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
		// persist deletion to the config file so removed routes don't reappear on restart.
		err = service.DeleteProxy(portInt)
		if err != nil {
			logger.Error("Failed to delete proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to delete proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := service.PersistConfig(); err != nil {
			logger.Error("Proxy deleted but failed to persist config", zap.Error(err))
			WriteResponseAsJSON(map[string]string{"status": "deleted", "warning": "Changes applied in memory but failed to persist to disk: " + err.Error()}, writer)
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
		result := []NetworkInterfaceDTO{
			{Name: "All Interfaces", IPv4: "0.0.0.0", IPv6: "::"},
		}
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
		if req.Default != nil && req.Default.Backend.Address == "" && !hasTerminatingMiddlewareDTO(req.Default.Middlewares) {
			http.Error(w, "Default route requires a backend or a terminating middleware", http.StatusBadRequest)
			return
		}
		for _, route := range req.Routes {
			if route.Target.Backend.Address == "" && !hasTerminatingMiddlewareDTO(route.Target.Middlewares) {
				http.Error(w, fmt.Sprintf("Route %q requires a backend or a terminating middleware", route.Host), http.StatusBadRequest)
				return
			}
		}
		conf, err := convertToHTTPListener(req)
		if err != nil {
			logger.Error("Invalid listener configuration", zap.Error(err))
			http.Error(w, "Invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := svc.AddHttpListener(conf); err != nil {
			logger.Error("Failed to add HTTP listener", zap.Error(err))
			http.Error(w, "Failed to create listener: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := svc.PersistConfig(); err != nil {
			logger.Error("Listener created but failed to persist config", zap.Error(err))
			WriteResponseAsJSON(map[string]interface{}{
				"status":  "created",
				"port":    req.Port,
				"warning": "Changes applied in memory but failed to persist to disk: " + err.Error(),
			}, w)
			return
		}
		logger.Info("HTTP listener created", zap.Int("port", req.Port))
		WriteResponseAsJSON(map[string]interface{}{
			"status": "created",
			"port":   req.Port,
		}, w)
	}
}

func hasTerminatingMiddlewareDTO(middlewares []MiddlewareConfigDTO) bool {
	configs := make([]mw.Config, len(middlewares))
	for i, m := range middlewares {
		configs[i] = mw.Config{Type: m.Type, Options: m.Options}
	}
	return mw.HasTerminatingMiddleware(configs)
}

func convertToHTTPListener(req *CreateProxyServerRequest) (config.HTTPListener, error) {
	conf := config.HTTPListener{
		Port:         req.Port,
		Interface:    req.Interface,
		Bind:         config.IpFlag(req.Bind),
		DisableHTTP2: req.DisableHTTP2,
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
		Backend:         config.BackendConfig{Address: dto.Backend.Address, ReplaceHostHeader: dto.Backend.ReplaceHostHeader},
		DisableDefaults: dto.DisableDefaults,
		TrustedProxies:  convertTrustedProxies(dto.TrustedProxies),
	}
	for _, m := range dto.Middlewares {
		target.Middlewares = append(target.Middlewares, mw.Config{
			Type:    m.Type,
			Options: m.Options,
		})
	}
	for _, p := range dto.Paths {
		pr := config.PathRule{
			Pattern:         p.Pattern,
			DropQuery:       p.DropQuery,
			StripPrefix:     p.StripPrefix,
			DisableDefaults: p.DisableDefaults,
			TrustedProxies:  convertTrustedProxies(p.TrustedProxies),
		}
		if p.Backend != nil {
			pr.Backend = &config.BackendConfig{Address: p.Backend.Address, ReplaceHostHeader: p.Backend.ReplaceHostHeader}
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

func handleGetProxyConfig(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := mw.GetRequestLoggerFromContext(r)
		port := r.PathValue("serverId")
		portInt, err := strconv.Atoi(port)
		if err != nil {
			http.Error(w, "Invalid port format", http.StatusBadRequest)
			return
		}
		conf := svc.GetProxyConfig(portInt)
		if conf == nil {
			http.Error(w, "Proxy not found", http.StatusNotFound)
			return
		}
		resp := configToDTO(conf)
		logger.Info("Returning proxy config", zap.Int("port", portInt))
		WriteResponseAsJSON(resp, w)
	}
}

func configToDTO(conf *config.HTTPListener) CreateProxyServerRequest {
	dto := CreateProxyServerRequest{
		Port:         conf.Port,
		Bind:         int(conf.Bind),
		Interface:    conf.Interface,
		DisableHTTP2: conf.DisableHTTP2,
	}
	if conf.TLS != nil {
		dto.TLS = &TLSConfigDTO{
			UseAcme: conf.TLS.UseAcme,
			Cert:    conf.TLS.Cert,
			Key:     conf.TLS.Key,
		}
	}
	if conf.ReadTimeout > 0 {
		dto.ReadTimeout = conf.ReadTimeout.String()
	}
	if conf.ReadHeaderTimeout > 0 {
		dto.ReadHeaderTimeout = conf.ReadHeaderTimeout.String()
	}
	if conf.WriteTimeout > 0 {
		dto.WriteTimeout = conf.WriteTimeout.String()
	}
	if conf.IdleTimeout > 0 {
		dto.IdleTimeout = conf.IdleTimeout.String()
	}
	if conf.Default != nil {
		defDTO := routeTargetToDTO(*conf.Default)
		dto.Default = &defDTO
	}
	for _, route := range conf.Routes {
		dto.Routes = append(dto.Routes, RouteDTO{
			Host:   route.Host,
			Target: routeTargetToDTO(route.Target),
		})
	}
	return dto
}

func routeTargetToDTO(t config.RouteTarget) RouteTargetDTO {
	dto := RouteTargetDTO{
		Backend:         BackendConfigDTO{Address: t.Backend.Address, ReplaceHostHeader: t.Backend.ReplaceHostHeader},
		DisableDefaults: t.DisableDefaults,
		TrustedProxies:  trustedProxiesToDTO(t.TrustedProxies),
	}
	for _, m := range t.Middlewares {
		mDTO := MiddlewareConfigDTO{Type: m.Type, Options: m.Options}
		dto.Middlewares = append(dto.Middlewares, mDTO)
	}
	for _, p := range t.Paths {
		pDTO := PathRuleDTO{
			Pattern:         p.Pattern,
			DropQuery:       p.DropQuery,
			StripPrefix:     p.StripPrefix,
			DisableDefaults: p.DisableDefaults,
			TrustedProxies:  trustedProxiesToDTO(p.TrustedProxies),
		}
		if p.Backend != nil {
			pDTO.Backend = &BackendConfigDTO{Address: p.Backend.Address, ReplaceHostHeader: p.Backend.ReplaceHostHeader}
		}
		for _, m := range p.Middlewares {
			pDTO.Middlewares = append(pDTO.Middlewares, MiddlewareConfigDTO{Type: m.Type, Options: m.Options})
		}
		dto.Paths = append(dto.Paths, pDTO)
	}
	return dto
}

func handleEditProxy(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := mw.GetRequestLoggerFromContext(r)
		port := r.PathValue("serverId")
		portInt, err := strconv.Atoi(port)
		if err != nil {
			http.Error(w, "Invalid port format", http.StatusBadRequest)
			return
		}
		req, err := DecodeJSONBody[CreateProxyServerRequest](r)
		if err != nil {
			logger.Error("Failed to decode edit request", zap.Error(err))
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Port = portInt
		conf, err := convertToHTTPListener(req)
		if err != nil {
			logger.Error("Invalid listener configuration", zap.Error(err))
			http.Error(w, "Invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := svc.EditProxy(portInt, conf); err != nil {
			if errors.Is(err, proxy.ErrProxyNotFound) {
				http.Error(w, "Proxy not found", http.StatusNotFound)
				return
			}
			logger.Error("Failed to edit proxy", zap.Error(err))
			http.Error(w, "Failed to edit proxy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		logger.Info("Proxy edited", zap.Int("port", portInt))
		if err := svc.PersistConfig(); err != nil {
			logger.Error("Proxy edited but failed to persist config", zap.Error(err))
			WriteResponseAsJSON(map[string]interface{}{
				"status":  "updated",
				"port":    conf.Port,
				"warning": "Changes applied in memory but failed to persist to disk: " + err.Error(),
			}, w)
			return
		}
		WriteResponseAsJSON(map[string]interface{}{
			"status": "updated",
			"port":   conf.Port,
		}, w)
	}
}

func convertTrustedProxies(dto *TrustedProxiesDTO) *config.TrustedProxiesConfig {
	if dto == nil {
		return nil
	}
	if dto.Preset == "" && len(dto.Ranges) == 0 {
		return nil
	}
	tp := &config.TrustedProxiesConfig{
		Preset: dto.Preset,
		Ranges: dto.Ranges,
		Header: dto.Header,
	}
	if dto.RefreshInterval != "" {
		if d, err := time.ParseDuration(dto.RefreshInterval); err == nil {
			tp.RefreshInterval = d
		}
	}
	return tp
}

func trustedProxiesToDTO(tp *config.TrustedProxiesConfig) *TrustedProxiesDTO {
	if tp == nil {
		return nil
	}
	if tp.Preset == "" && len(tp.Ranges) == 0 {
		return nil
	}
	dto := &TrustedProxiesDTO{
		Preset: tp.Preset,
		Ranges: tp.Ranges,
		Header: tp.Header,
	}
	if tp.RefreshInterval > 0 {
		dto.RefreshInterval = tp.RefreshInterval.String()
	}
	return dto
}
