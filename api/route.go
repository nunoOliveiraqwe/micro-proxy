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
		Name:               "Active Proxy Routes",
		Description:        "Fetches the configured proxy routes",
		Method:             "GET",
		Pattern:            "/proxy/routes",
		IsAllowedBeforeFts: false,
		IsAllowedAfterFts:  true,
		IsSecure:           true,
		HandlerFunc:        handleGetProxies,
	},
}
