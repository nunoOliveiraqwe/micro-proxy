package server

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/manager"
)

var APPLICATION_ROUTE_BASE_PATH = "/api/v1"

type ApplicationHandlerFunc func(manager manager.SystemManager) http.HandlerFunc

type ApplicationRoute struct {
	Name               string
	Description        string
	Method             string
	Pattern            string
	IsAllowedAfterFTS  bool
	IsAllowedBeforeFTS bool
	IsSecure           bool
	HandlerFunc        ApplicationHandlerFunc
}

var routes = []ApplicationRoute{
	{
		Name:               "Healthcheck",
		Description:        "Healthcheck endpoint",
		Method:             "GET",
		Pattern:            "/healthcheck",
		IsAllowedAfterFTS:  true,
		IsAllowedBeforeFTS: true,
		IsSecure:           false,
		HandlerFunc:        handleHealthCheck,
	},
}
