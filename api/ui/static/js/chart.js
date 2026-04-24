// ---------- Metric history for charting ----------
var MAX_POINTS = 60;
var metricsHistory = {
    labels: [], bytesSent: [], bytesReceived: [], requestCount: [],
    errorCount: [], request2xx: [], request4xx: [], request5xx: [],
    latencyP99: [], reqPerSec: []
};

var selectedMetric = 'reqPerSec';
var chart = null;
var prevSnapshot = null;

var metricMeta = {
    reqPerSec: {label: 'Req/s', color: 'rgba(124,110,240,1)', bg: 'rgba(124,110,240,0.15)'},
    latencyP99: {label: 'Latency P99 (ms)', color: 'rgba(231,76,60,1)', bg: 'rgba(231,76,60,0.15)', isMs: true},
    bytesSent: {label: 'Bytes Sent', color: 'rgba(124,110,240,1)', bg: 'rgba(124,110,240,0.15)', isBytes: true},
    bytesReceived: {
        label: 'Bytes Received',
        color: 'rgba(52,152,219,1)',
        bg: 'rgba(52,152,219,0.15)',
        isBytes: true
    },
    requestCount: {label: 'Requests', color: 'rgba(46,204,113,1)', bg: 'rgba(46,204,113,0.15)'},
    errorCount: {label: 'Errors', color: 'rgba(231,76,60,1)', bg: 'rgba(231,76,60,0.15)'},
    request2xx: {label: '2xx', color: 'rgba(46,204,113,1)', bg: 'rgba(46,204,113,0.15)'},
    request4xx: {label: '4xx', color: 'rgba(243,156,18,1)', bg: 'rgba(243,156,18,0.15)'},
    request5xx: {label: '5xx', color: 'rgba(231,76,60,1)', bg: 'rgba(231,76,60,0.15)'}
};

function initChart() {
    var ctx = document.getElementById('metricsChart').getContext('2d');
    var meta = metricMeta[selectedMetric];
    var isDark = (document.documentElement.getAttribute('data-theme') || 'dark') === 'dark';
    var gridColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.08)';
    var tickColor = isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)';
    chart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: metricsHistory.labels,
            datasets: [{
                label: meta.label,
                data: metricsHistory[selectedMetric],
                borderColor: meta.color,
                backgroundColor: meta.bg,
                fill: true, tension: 0.35, pointRadius: 0, pointHitRadius: 6
            }]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            animation: {duration: 200},
            interaction: {mode: 'index', intersect: false},
            plugins: {legend: {display: false}},
            scales: {
                x: {
                    grid: {color: gridColor},
                    ticks: {color: tickColor, maxRotation: 0, maxTicksLimit: 10}
                },
                y: {
                    beginAtZero: true,
                    grid: {color: gridColor},
                    ticks: {
                        color: tickColor,
                        callback: function (value) {
                            var m = metricMeta[selectedMetric];
                            if (m && m.isBytes) return fmtBytes(value);
                            if (m && m.isMs) return value + ' ms';
                            return value;
                        }
                    }
                }
            }
        }
    });
}

function switchMetric(key) {
    selectedMetric = key;
    var meta = metricMeta[key];
    chart.data.datasets[0].label = meta.label;
    chart.data.datasets[0].data = metricsHistory[key];
    chart.data.datasets[0].borderColor = meta.color;
    chart.data.datasets[0].backgroundColor = meta.bg;
    chart.update();
}

document.getElementById('metric-selector').addEventListener('change', function () {
    switchMetric(this.value);
});

var selectedConnection = 'global';
var cachedProxySnapshots = [];
var metricsEventSource = null;
var latestSnapshot = null;
var perConnectionSnapshots = {};
var knownConnectionNames = {};
var chartTickInterval = null;
var CHART_TICK_MS = 1000;
var lastSnapshotTime = null;

function connectMetricsSSE() {
    if (metricsEventSource) metricsEventSource.close();
    setSseStatus('connecting');
    metricsEventSource = new EventSource('/api/v1/proxy/metrics/stream');
    metricsEventSource.addEventListener('metrics', function (e) {
        setSseStatus('connected');
        var snap = JSON.parse(e.data);
        var connName = snap.connection_name || '';
        perConnectionSnapshots[connName] = snap;
        if (!knownConnectionNames[connName]) {
            knownConnectionNames[connName] = true;
            updateConnectionSelector();
            updateLogFilterDropdowns();
        }
        if (selectedConnection === 'global' && connName === 'global') {
            applySnapshot(snap);
        } else if (connName === selectedConnection) {
            applySnapshot(snap);
        }
        var portMatch = connName.match(/^metric-port-(\d+)$/);
        if (portMatch) {
            var metricsCell = document.querySelector('[data-metrics-port="' + portMatch[1] + '"]');
            if (metricsCell) metricsCell.innerHTML = metricsHtmlFor(snap);
        }
    });
    metricsEventSource.addEventListener('proxy_error', function (e) {
        var entry = JSON.parse(e.data);
        addErrorToFeed(entry);
        addToActivityFeed('error', entry);
    });
    metricsEventSource.addEventListener('proxy_request', function (e) {
        var entry = JSON.parse(e.data);
        addRequestToFeed(entry);
        addToActivityFeed('request', entry);
        var connKey = entry.connection_name || '';
        if (sparklineBuffers[connKey]) {
            sparklineBuffers[connKey].count++;
        }
        var portKey = extractPortKey(connKey);
        if (portKey !== connKey && sparklineBuffers[portKey]) {
            sparklineBuffers[portKey].count++;
        }
    });
    metricsEventSource.addEventListener('proxy_blocked', function (e) {
        var entry = JSON.parse(e.data);
        addBlockToFeed(entry);
        addToActivityFeed('blocked', entry);
    });
    metricsEventSource.onopen = function () {
        setSseStatus('connected');
    };
    metricsEventSource.onerror = function () {
        setSseStatus('disconnected');
    };
}

function applySnapshot(snap) {
    var now = Date.now();
    latestSnapshot = snap;
    latestSnapshot._ts = now;
    updateStatCards(latestSnapshot);
    if (!prevSnapshot) {
        prevSnapshot = latestSnapshot;
        lastSnapshotTime = now;
    }
}

var currentRps = 0;

function startChartTick() {
    if (chartTickInterval) clearInterval(chartTickInterval);
    chartTickInterval = setInterval(tickChart, CHART_TICK_MS);
}

function tickChart() {
    metricsHistory.labels.push(timeLabel());

    if (latestSnapshot && prevSnapshot) {
        var deltaSec = CHART_TICK_MS / 1000;
        var deltaRequests = Math.max(0, latestSnapshot.request_count - prevSnapshot.request_count);
        currentRps = parseFloat((deltaRequests / deltaSec).toFixed(1));
        metricsHistory.bytesSent.push(Math.max(0, latestSnapshot.bytes_sent - prevSnapshot.bytes_sent));
        metricsHistory.bytesReceived.push(Math.max(0, latestSnapshot.bytes_received - prevSnapshot.bytes_received));
        metricsHistory.requestCount.push(deltaRequests);
        metricsHistory.errorCount.push(Math.max(0, latestSnapshot.error_count - prevSnapshot.error_count));
        metricsHistory.request2xx.push(Math.max(0, latestSnapshot.request_2xx_count - prevSnapshot.request_2xx_count));
        metricsHistory.request4xx.push(Math.max(0, latestSnapshot.request_4xx_count - prevSnapshot.request_4xx_count));
        metricsHistory.request5xx.push(Math.max(0, latestSnapshot.request_5xx_count - prevSnapshot.request_5xx_count));
        metricsHistory.latencyP99.push(latestSnapshot.p99_ms || 0);
        metricsHistory.reqPerSec.push(currentRps);
        prevSnapshot = latestSnapshot;
        lastSnapshotTime = Date.now();
    } else {
        currentRps = 0;
        metricsHistory.bytesSent.push(0);
        metricsHistory.bytesReceived.push(0);
        metricsHistory.requestCount.push(0);
        metricsHistory.errorCount.push(0);
        metricsHistory.request2xx.push(0);
        metricsHistory.request4xx.push(0);
        metricsHistory.request5xx.push(0);
        metricsHistory.latencyP99.push(0);
        metricsHistory.reqPerSec.push(0);
    }

    if (metricsHistory.labels.length > MAX_POINTS) {
        Object.keys(metricsHistory).forEach(function (k) {
            metricsHistory[k].shift();
        });
    }
    if (chart) chart.update();

    document.getElementById('stat-rps').textContent = currentRps.toFixed(1);

    document.getElementById('last-updated').innerHTML =
        '<span class="dot"></span>' + (latestSnapshot ? 'Last updated at ' : 'Waiting for data… ') + timeLabel();
}

function connectionKey(proxy) {
    return 'metric-port-' + proxy.port;
}

function findPortMetric(proxy) {
    var key = connectionKey(proxy);
    if (!proxy.metrics) return null;
    for (var i = 0; i < proxy.metrics.length; i++) {
        if (proxy.metrics[i].connection_name === key) return proxy.metrics[i];
    }
    return proxy.metrics.length > 0 ? proxy.metrics[0] : null;
}

function livePortMetric(proxy) {
    return perConnectionSnapshots[connectionKey(proxy)] || findPortMetric(proxy);
}

function resetStatCards() {
    currentRps = 0;
    document.getElementById('stat-rps').textContent = '–';
    document.getElementById('stat-total-requests').textContent = '';
    document.getElementById('stat-errors').textContent = '–';
    document.getElementById('stat-error-rate').textContent = '';
    document.getElementById('stat-p50').textContent = '–';
    document.getElementById('stat-p95').textContent = '–';
    document.getElementById('stat-p99').textContent = '–';
    document.getElementById('stat-bytes-sent').textContent = '–';
    document.getElementById('stat-bytes-recv').textContent = '–';
    document.getElementById('stat-2xx').textContent = '–';
    document.getElementById('stat-3xx').textContent = '–';
    document.getElementById('stat-4xx').textContent = '–';
    document.getElementById('stat-5xx').textContent = '–';
    document.getElementById('stat-blocked-total').textContent = '–';
    document.getElementById('stat-blocked-honeypot').textContent = '–';
    document.getElementById('stat-blocked-ua').textContent = '–';
    document.getElementById('stat-blocked-country').textContent = '–';
    document.getElementById('stat-blocked-ip').textContent = '–';
    document.getElementById('stat-blocked-ratelimit').textContent = '–';
    document.getElementById('stat-blocked-waf').textContent = '–';
}

function updateStatCards(m) {
    document.querySelectorAll('.stat-value.loading').forEach(function (el) {
        el.classList.remove('loading');
    });

    document.getElementById('stat-rps').textContent = currentRps.toFixed(1);
    document.getElementById('stat-total-requests').textContent = fmtNum(m.request_count) + ' total';

    document.getElementById('stat-errors').textContent = fmtNum(m.error_count);
    document.getElementById('stat-bytes-sent').textContent = fmtBytes(m.bytes_sent);
    document.getElementById('stat-bytes-recv').textContent = fmtBytes(m.bytes_received);

    var rate = m.request_count > 0 ? ((m.error_count / m.request_count) * 100).toFixed(1) + '%' : '0%';
    document.getElementById('stat-error-rate').textContent = rate + ' error rate';

    document.getElementById('stat-p50').textContent = fmtNum(m.p50_ms);
    document.getElementById('stat-p95').textContent = fmtNum(m.p95_ms);
    document.getElementById('stat-p99').textContent = fmtNum(m.p99_ms);

    document.getElementById('stat-2xx').textContent = fmtNum(m.request_2xx_count);
    document.getElementById('stat-3xx').textContent = fmtNum(m.request_3xx_count);
    document.getElementById('stat-4xx').textContent = fmtNum(m.request_4xx_count);
    document.getElementById('stat-5xx').textContent = fmtNum(m.request_5xx_count);

    var bbm = m.blocked_by_middleware || {};
    document.getElementById('stat-blocked-total').textContent = fmtNum(m.blocked_total);
    document.getElementById('stat-blocked-honeypot').textContent = fmtNum(bbm['honeypot'] || 0);
    document.getElementById('stat-blocked-ua').textContent = fmtNum(bbm['ua-block'] || 0);
    document.getElementById('stat-blocked-country').textContent = fmtNum(bbm['country-block'] || 0);
    document.getElementById('stat-blocked-ip').textContent = fmtNum(bbm['ip-filter'] || 0);
    document.getElementById('stat-blocked-ratelimit').textContent = fmtNum(bbm['rate-limit'] || 0);
    document.getElementById('stat-blocked-waf').textContent = fmtNum(bbm['coraza-waf'] || 0);
}

function resetChartHistory() {
    Object.keys(metricsHistory).forEach(function (k) {
        metricsHistory[k].length = 0;
    });
    prevSnapshot = null;
    latestSnapshot = null;
    lastSnapshotTime = null;
    resetStatCards();
    if (chart) chart.update();
}

document.getElementById('connection-selector').addEventListener('change', function () {
    selectedConnection = this.value;
    resetChartHistory();
});

