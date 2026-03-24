package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
)

var APPLICATION_ROUTE_BASE_PATH = "/api/v1"

type ApplicationHandlerFunc func(svc app.SystemService) http.HandlerFunc

type ApplicationRoute struct {
	Name               string
	Description        string
	Method             string
	Pattern            string
	IsAllowedBeforeFts bool
	IsAllowedAfterFts  bool
	IsSecure           bool
	HandlerFunc        ApplicationHandlerFunc
}

var routes = []ApplicationRoute{
	{
		Name:               "Healthcheck",
		Description:        "Healthcheck endpoint",
		Method:             "GET",
		Pattern:            "/healthcheck",
		IsAllowedBeforeFts: true,
		IsAllowedAfterFts:  true,
		IsSecure:           false,
		HandlerFunc:        handleHealthCheck,
	},
	{
		Name:               "Get FTS status",
		Description:        "Gets the status of the first time setup process, which is used to determine if the first time setup process has been completed or not. This endpoint is used to check if the first time setup process has been completed or not",
		Method:             "GET",
		Pattern:            "/fts",
		IsAllowedAfterFts:  true,
		IsAllowedBeforeFts: true,
		IsSecure:           false,
		HandlerFunc:        handleGetFtsStatus,
	},
	{
		Name: "Complete FTS",
		Description: "Handles the completion of the first time setup, which included setting a " +
			"password for the admin user and creating the first user account. This endpoint is used to complete the first time setup process",
		Method:             "POST",
		Pattern:            "/fts",
		IsAllowedBeforeFts: true,
		IsAllowedAfterFts:  false,
		IsSecure:           false,
		HandlerFunc:        handleCompleteFts,
	},
	{
		Name:               "Auth Login",
		Description:        "Handles the login process",
		Method:             "POST",
		Pattern:            "/auth/login",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           false,
		HandlerFunc:        handleLogin,
	},
	{
		Name:               "Auth Logout",
		Description:        "Handles the logout process",
		Method:             "POST",
		Pattern:            "/auth/logout",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleLogout,
	},
	{
		Name:               "Auth Identity",
		Description:        "Gets the identity of the currntly logged in user",
		Method:             "GET",
		Pattern:            "/auth/user",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleIdentity,
	},
	{
		Name:               "Auth Change Password",
		Description:        "Changes Password for the currently logged in user",
		Method:             "POST",
		Pattern:            "/auth/user",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleChangePassword,
	},
	{
		Name:               "Active Proxy Routes",
		Description:        "Fetches the configured proxy routes",
		Method:             "GET",
		Pattern:            "/proxy/routes",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetProxies,
	},
	{
		Name:               "Stop an active proxy",
		Description:        "Stops an active proxy",
		Method:             "POST",
		Pattern:            "/proxy/routes/{connectionId}/stop",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleStopProxy,
	},
	{
		Name:               "Starts an stopped proxy",
		Description:        "Starts an stopped proxy",
		Method:             "POST",
		Pattern:            "/proxy/routes/{connectionId}/start",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleStartProxy,
	},
	{
		Name:               "Global proxy metrics",
		Description:        "Fetches the global proxy metrics",
		Method:             "GET",
		Pattern:            "/proxy/metrics",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetGlobalMetrics,
	},
	{
		Name:               "Global proxy metrics",
		Description:        "Fetches the global proxy metrics",
		Method:             "GET",
		Pattern:            "/proxy/metrics/{connectionId}",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetMetricForConnection,
	},
	{
		Name:               "SSE",
		Description:        "Subscribe to all metrics pertaining to the proxy",
		Method:             "GET",
		Pattern:            "/proxy/metrics/stream",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleSSEGlobalMetrics,
	},
	{
		Name:               "System Health",
		Description:        "Returns runtime health metrics (memory, goroutines, GC, uptime)",
		Method:             "GET",
		Pattern:            "/system/health",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetSystemHealth,
	},
	{
		Name:               "Recent Errors",
		Description:        "Returns the most recent 5xx error entries",
		Method:             "GET",
		Pattern:            "/proxy/errors",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetRecentErrors,
	},
	{
		Name:               "Recent Requests",
		Description:        "Returns the most recent proxied request entries",
		Method:             "GET",
		Pattern:            "/proxy/requests",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetRecentRequests,
	},
}
