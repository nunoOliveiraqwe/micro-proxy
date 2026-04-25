// ---------- Bootstrap ----------
initChart();
connectMetricsSSE();
startChartTick();
loadProxyRoutes();
loadIdentity();
loadRecentBlocked();  // seed threat defense cards on dashboard
setInterval(tickSparklines, 1000);

var routeInterval = setInterval(loadProxyRoutes, 10000);

