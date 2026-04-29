// ---------- Sparklines ----------
var sparklineBuffers = {};  // key → { history: [], count: 0 }
var SPARKLINE_POINTS = 30;

function drawSparkline(canvas, data) {
    var ctx = canvas.getContext('2d');
    var w = canvas.width;
    var h = canvas.height;
    ctx.clearRect(0, 0, w, h);
    if (data.length < 2) return;
    var max = Math.max.apply(null, data) || 1;
    ctx.beginPath();
    ctx.strokeStyle = '#7c6ef0';
    ctx.lineWidth = 1.5;
    for (var i = 0; i < data.length; i++) {
        var x = (i / (SPARKLINE_POINTS - 1)) * w;
        var y = h - (data[i] / max) * (h - 2) - 1;
        if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.stroke();
}

function ensureSparklineBuffers(proxies) {
    // Add buffers for new routes
    proxies.forEach(function (p) {
        if (!p.is_started) return;
        var key = connectionKey(p);
        if (!sparklineBuffers[key]) {
            sparklineBuffers[key] = {history: [], count: 0};
        }
    });
    // Remove buffers for stopped/removed routes
    var wantKeys = {};
    proxies.forEach(function (p) {
        if (p.is_started) wantKeys[connectionKey(p)] = true;
    });
    Object.keys(sparklineBuffers).forEach(function (k) {
        if (!wantKeys[k]) delete sparklineBuffers[k];
    });
}

function tickSparklines() {
    Object.keys(sparklineBuffers).forEach(function (key) {
        var buf = sparklineBuffers[key];
        buf.history.push(buf.count);
        buf.count = 0;
        if (buf.history.length > SPARKLINE_POINTS) buf.history.shift();
        var canvas = document.querySelector('.sparkline[data-connection="' + key + '"]');
        if (canvas) drawSparkline(canvas, buf.history);
    });
}

// ---------- Proxy routes ----------
function loadProxyRoutes() {
    fetch('/api/v1/proxy/routes', {credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return null;
            }
            if (!resp.ok) throw new Error(resp.statusText);
            return resp.json();
        })
        .then(function (proxies) {
            if (proxies === null) return;
            cachedProxySnapshots = proxies || [];
            updateConnectionSelector();
            renderProxyRoutes(proxies);
            ensureSparklineBuffers(proxies);
        })
        .catch(function () {
            document.getElementById('proxy-routes').innerHTML = '<p>Failed to load proxy routes.</p>';
        });
}

function updateConnectionSelector() {
    var sel = document.getElementById('connection-selector');
    var current = sel.value;

    // Build the dropdown from all connection names discovered via SSE.
    var names = Object.keys(perConnectionSnapshots).sort();

    // Group: port-level → [child names (hosts, paths, host+path)]
    var ports = {};
    names.forEach(function (name) {
        if (name === 'global') return;
        var portKey = extractPortKey(name);
        if (portKey === name) {
            // This is a port-level metric
            if (!ports[name]) ports[name] = [];
        } else {
            if (!ports[portKey]) ports[portKey] = [];
            ports[portKey].push(name);
        }
    });

    var html = '<option value="global">Global (all proxies)</option>';
    Object.keys(ports).sort().forEach(function (portKey) {
        var portLabel = portKey.replace(/^metric-port-/, ':');
        var children = ports[portKey];
        if (children.length > 0) {
            html += '<option value="' + portKey + '">' + portLabel + ' (all)</option>';
            children.sort().forEach(function (childKey) {
                var label = extractChildLabel(childKey, portKey);
                html += '<option value="' + childKey + '">\u00A0\u00A0\u2514 ' + portLabel + ' ' + label + '</option>';
            });
        } else {
            html += '<option value="' + portKey + '">' + portLabel + '</option>';
        }
    });

    sel.innerHTML = html;
    if (current && sel.querySelector('option[value="' + current + '"]')) sel.value = current;
}

function extractPortKey(name) {
    var hostIdx = name.indexOf('-host-');
    var pathIdx = name.indexOf('-path-');
    if (hostIdx === -1 && pathIdx === -1) return name;
    var idx = (hostIdx !== -1 && pathIdx !== -1) ? Math.min(hostIdx, pathIdx)
            : (hostIdx !== -1 ? hostIdx : pathIdx);
    return name.substring(0, idx);
}

function extractChildLabel(childKey, portKey) {
    var rest = childKey.substring(portKey.length);
    var label = '';
    var hostIdx = rest.indexOf('-host-');
    var pathIdx = rest.indexOf('-path-');
    if (hostIdx !== -1) {
        var hostStart = hostIdx + 6;
        var hostEnd = pathIdx !== -1 ? pathIdx : rest.length;
        var host = rest.substring(hostStart, hostEnd);
        label += host === '_default' ? '(default)' : host;
    }
    if (pathIdx !== -1) {
        var path = rest.substring(pathIdx + 6);
        label += (label ? ' ' : '') + path;
    }
    return label || rest;
}

function metricsSummary(met) {
    if (!met) return 'No metrics available';
    return 'Requests: ' + fmtNum(met.request_count) +
        '\nErrors: ' + fmtNum(met.error_count) +
        '\nP50: ' + fmtNum(met.p50_ms) + ' ms  P95: ' + fmtNum(met.p95_ms) + ' ms  P99: ' + fmtNum(met.p99_ms) + ' ms' +
        '\nBytes Sent: ' + fmtBytes(met.bytes_sent) +
        '\nBytes Received: ' + fmtBytes(met.bytes_received) +
        '\nTimeouts: ' + fmtNum(met.upstream_timeouts) +
        '\n2xx: ' + fmtNum(met.request_2xx_count) +
        '  3xx: ' + fmtNum(met.request_3xx_count) +
        '  4xx: ' + fmtNum(met.request_4xx_count) +
        '  5xx: ' + fmtNum(met.request_5xx_count);
}

// Track the last-rendered route fingerprint so we know when a full rebuild is needed.
var lastRouteFingerprint = '';

function routeFingerprint(proxies) {
    return proxies.slice().sort(function (a, b) {
        return a.port - b.port;
    }).map(function (p) {
        return p.port + ':' + (p.is_started ? '1' : '0') + ':' + (p.errorMessage || '');
    }).join('|');
}

function metricsHtmlFor(met) {
    if (met && met.request_count > 0) {
        return '<div class="route-metrics">' +
            '<span>' + fmtNum(met.request_count) + ' req</span>' +
            '<span>' + fmtNum(met.error_count) + ' err</span>' +
            '<span>P50 ' + fmtNum(met.p50_ms) + 'ms</span>' +
            '<span>P99 ' + fmtNum(met.p99_ms) + 'ms</span>' +
            '</div>';
    }
    return '<span style="color:var(--pico-muted-color);font-size:0.75rem">–</span>';
}

function redrawAllSparklines() {
    Object.keys(sparklineBuffers).forEach(function (key) {
        var buf = sparklineBuffers[key];
        var canvas = document.querySelector('.sparkline[data-connection="' + key + '"]');
        if (canvas && buf.history.length >= 2) drawSparkline(canvas, buf.history);
    });
}

function sortProxiesByPort(proxies) {
    return proxies.slice().sort(function (a, b) {
        return a.port - b.port;
    });
}

function renderProxyRoutes(proxies) {
    var el = document.getElementById('proxy-routes');
    if (!proxies || proxies.length === 0) {
        lastRouteFingerprint = '';
        el.innerHTML =
            '<div class="empty-state"><div class="empty-icon">🔌</div>' +
            '<p><strong>No proxy routes configured</strong></p>' +
            '<p><small>Add HTTP routes in your configuration file and restart the server.</small></p></div>';
        return;
    }

    proxies = sortProxiesByPort(proxies);
    var fp = routeFingerprint(proxies);

    // Fast path: if the route structure hasn't changed, just update metrics cells in-place.
    if (fp === lastRouteFingerprint) {
        proxies.forEach(function (p) {
            var metricsCell = el.querySelector('[data-metrics-port="' + p.port + '"]');
            if (metricsCell) metricsCell.innerHTML = metricsHtmlFor(livePortMetric(p));
        });
        return;
    }

    // Full rebuild (route structure changed)
    lastRouteFingerprint = fp;
    var h = '<div class="overflow-auto"><table class="striped"><thead><tr>' +
        '<th>Port</th><th>Interface</th><th>Backends</th><th>HTTPS</th>' +
        '<th>ACME</th><th>Status</th><th>Traffic</th><th>Metrics</th><th>Actions</th>' +
        '</tr></thead><tbody>';
    proxies.forEach(function (p) {
        var actions = '<div class="proxy-actions">';
        if (p.is_started) {
            actions += '<button class="act-btn act-stop proxy-action-btn" data-port="' + p.port + '" data-action="stop" title="Stop">\u25A0\uFE0E</button>';
            actions += '<button class="act-btn act-edit proxy-edit-btn" data-port="' + p.port + '" title="Edit">\u270E\uFE0E</button>';
        } else {
            actions += '<button class="act-btn act-start proxy-action-btn" data-port="' + p.port + '" data-action="start" title="Start">\u25B6\uFE0E</button>';
            actions += '<button class="act-btn act-edit proxy-edit-btn" data-port="' + p.port + '" title="Edit">\u270E\uFE0E</button>';
            actions += '<button class="act-btn act-delete proxy-delete-btn" data-port="' + p.port + '" title="Delete">\u2715\uFE0E</button>';
        }
        actions += '</div>';
        var sparkHtml = p.is_started
            ? '<canvas class="sparkline" data-connection="' + connectionKey(p) + '" width="60" height="30"></canvas>'
            : '<span style="color:var(--pico-muted-color);font-size:0.75rem">–</span>';
        var routeCount = (p.routes && p.routes.length) || 0;
        var pathCount = 0;
        if (p.routes) p.routes.forEach(function (r) {
            pathCount += (r.paths && r.paths.length) || 0;
        });
        var toggleLabel = routeCount + ' route' + (routeCount !== 1 ? 's' : '');
        if (pathCount > 0) toggleLabel += ', ' + pathCount + ' path' + (pathCount !== 1 ? 's' : '');
        h += '<tr>' +
            '<td><code>' + p.port + '</code></td>' +
            '<td>' + (p.interface || '<em>all</em>') + '</td>' +
            '<td>' + (p.backend && p.backend.length
                ? p.backend.map(function (b) {
                    return '<code>' + b + '</code>';
                }).join('<br>')
                : '<em>none</em>') + '</td>' +
            '<td>' + badge(p.is_using_https) + '</td>' +
            '<td>' + badge(p.is_using_acme) + '</td>' +
            '<td>' + (p.errorMessage
                ? '<span class="badge badge-danger" data-tooltip="' + p.errorMessage.replace(/"/g, '&quot;') + '">Error</span>'
                : p.is_started
                    ? '<span class="badge badge-success">Running</span>'
                    : '<span class="badge badge-danger">Stopped</span>') + '</td>' +
            '<td>' + sparkHtml + '</td>' +
            '<td data-metrics-port="' + p.port + '">' + metricsHtmlFor(livePortMetric(p)) + '</td>' +
            '<td>' + actions + '</td>' +
            '</tr>';
        // Expandable detail row for routes
        if (routeCount > 0) {
            h += '<tr class="route-detail-row" data-detail-port="' + p.port + '" style="display:none;">' +
                '<td colspan="9" style="padding:0.5rem 1rem;background:var(--pico-card-background-color,#1a1a2e);">' +
                renderRouteDetail(p.routes) +
                '</td></tr>';
            // Insert toggle into the Port cell of the main row above
            h = h.replace(
                '<td><code>' + p.port + '</code></td>',
                '<td><code>' + p.port + '</code><br>' +
                '<a href="#" class="route-toggle-btn" data-toggle-port="' + p.port +
                '" style="font-size:0.7rem;cursor:pointer;">▶ ' + toggleLabel + '</a></td>'
            );
        }
    });
    h += '</tbody></table></div>';
    el.innerHTML = h;
    el.querySelectorAll('.proxy-action-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            toggleProxy(this.getAttribute('data-port'), this.getAttribute('data-action'));
        });
    });
    el.querySelectorAll('.proxy-delete-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            deleteProxy(this.getAttribute('data-port'));
        });
    });
    el.querySelectorAll('.proxy-edit-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            editProxy(parseInt(this.getAttribute('data-port')));
        });
    });
    el.querySelectorAll('.route-toggle-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            var port = this.getAttribute('data-toggle-port');
            var detailRow = el.querySelector('[data-detail-port="' + port + '"]');
            if (!detailRow) return;
            var isHidden = detailRow.style.display === 'none';
            detailRow.style.display = isHidden ? 'table-row' : 'none';
            this.textContent = (isHidden ? '▼ ' : '▶ ') + this.textContent.substring(2);
        });
    });
    // Immediately redraw sparklines so canvases aren't blank after rebuild
    redrawAllSparklines();
}

function renderRouteDetail(routes) {
    if (!routes || routes.length === 0) return '<em>No routes</em>';
    var h = '<table style="width:100%;margin:0;font-size:0.8rem;">' +
        '<thead><tr><th>Host</th><th>Backend</th><th>Middleware</th></tr></thead><tbody>';
    routes.forEach(function (r) {


        var host = r.host || '<em>default</em>';

        var globalMw = (r.global_middlewares && r.global_middlewares.length) ? r.global_middlewares.join(', ') : '';
        var mw = (r.middlewares && r.middlewares.length) ? r.middlewares.join(', ') : '<em>none</em>';
        var fullMw = globalMw ? (globalMw + ', ' + mw) : mw;
        var backendAddr = (typeof r.backend === 'object' && r.backend) ? r.backend.address : (r.backend || '');
        h += '<tr><td>' + host + '</td><td><code>' + backendAddr + '</code></td><td>' + fullMw + '</td></tr>';
        if (r.paths && r.paths.length > 0) {
            r.paths.forEach(function (p) {
                var pmw = (p.middlewares && p.middlewares.length) ? p.middlewares.join(', ') : '<em>none</em>';
                var rawPb = (typeof p.backend === 'object' && p.backend) ? p.backend.address : p.backend;
                var pbackend = rawPb ? '<code>' + rawPb + '</code>' : 'Defined by parent';
                h += '<tr style="color:var(--pico-muted-color);">' +
                    '<td style="padding-left:1.5rem;">↳ <code>' + p.pattern + '</code></td>' +
                    '<td>' + pbackend + '</td>' +
                    '<td>' + pmw + '</td></tr>';
            });
        }
    });
    h += '</tbody></table>';
    return h;
}

function toggleProxy(port, action) {
    if (!confirm('Are you sure you want to ' + action + ' the proxy on port ' + port + '?')) return;
    lastRouteFingerprint = '';  // force full rebuild on next load
    fetch('/api/v1/proxy/routes/' + port + '/' + action, {method: 'POST', credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return;
            }
            if (!resp.ok) return resp.text().then(function (t) {
                showToast(t || 'Failed to ' + action + ' proxy', 'error');
            });
            showToast('Proxy on port ' + port + ' ' + (action === 'stop' ? 'stopped' : 'started'), 'success');
            loadProxyRoutes();
        })
        .catch(function () {
            showToast('Failed to ' + action + ' proxy on port ' + port, 'error');
        });
}

function deleteProxy(port) {
    if (!confirm('Delete the proxy on port ' + port + '?\n\nThis will permanently remove the listener and all its routes. This cannot be undone.')) return;
    lastRouteFingerprint = '';
    fetch('/api/v1/proxy/routes/' + port, {method: 'DELETE', credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return;
            }
            if (!resp.ok) return resp.text().then(function (t) {
                showToast(t || 'Failed to delete proxy', 'error');
            });
            showToast('Proxy on port ' + port + ' deleted.', 'success');
            loadProxyRoutes();
        })
        .catch(function () {
            showToast('Failed to delete proxy on port ' + port, 'error');
        });
}

function editProxy(port) {
    // Fetch the original creation config for this proxy
    fetch('/api/v1/proxy/routes/' + port + '/config', {credentials: 'same-origin'})
        .then(function(resp) {
            if (resp.status === 401) { window.location.href = '/ui/login'; return null; }
            if (!resp.ok) return resp.text().then(function(t) { showToast(t || 'Failed to load proxy config', 'error'); return null; });
            return resp.json();
        })
        .then(function(data) {
            if (!data) return;
            lfEditOriginalConfig = JSON.parse(JSON.stringify(data));  // deep copy
            initListenerWizard();
            // Wait a tick for interfaces dropdown to populate, then fill form
            setTimeout(function() { lfPopulateForm(data); }, 300);
        })
        .catch(function() {
            showToast('Failed to load proxy config for port ' + port, 'error');
        });
}

function lfPopulateForm(data) {
    var savedOrigConfig = lfEditOriginalConfig;  // preserve before reset clears it
    lfResetForm();
    lfEditMode = true;
    lfEditPort = data.port;
    lfEditOriginalConfig = savedOrigConfig;

    // Update panel title and submit button
    document.querySelector('#create-proxy-panel > div:first-child h4').textContent = 'Edit HTTP Proxy Server \u2014 Port ' + data.port;
    document.getElementById('lf-submit').textContent = 'Save Changes';
    document.getElementById('lf-review-subtitle').textContent = 'Review the changes before applying. The impact indicator below shows whether this edit requires a restart.';

    // Step 1: Server settings
    document.getElementById('lf-port').value = data.port;
    document.getElementById('lf-port').readOnly = true;
    document.getElementById('lf-port').style.opacity = '0.6';

    // Interface
    var ifaceSel = document.getElementById('lf-interface');
    var ifaceVal = data.interface || '';
    var found = false;
    for (var i = 0; i < ifaceSel.options.length; i++) {
        if (ifaceSel.options[i].value === ifaceVal) { ifaceSel.value = ifaceVal; found = true; break; }
    }
    if (!found) {
        var opt = document.createElement('option');
        opt.value = ifaceVal; opt.textContent = ifaceVal || 'All Interfaces';
        ifaceSel.appendChild(opt); ifaceSel.value = ifaceVal;
    }

    // Bind
    if (data.bind) document.getElementById('lf-bind').value = data.bind;

    // TLS
    if (data.tls) {
        document.getElementById('lf-tls-enable').checked = true;
        document.getElementById('lf-tls-fields').style.display = '';
        if (data.tls.use_acme) {
            document.getElementById('lf-tls-acme').checked = true;
            document.getElementById('lf-tls-cert-fields').style.display = 'none';
        } else {
            document.getElementById('lf-tls-acme').checked = false;
            document.getElementById('lf-tls-cert-fields').style.display = '';
            if (data.tls.cert) document.getElementById('lf-tls-cert').value = data.tls.cert;
            if (data.tls.key) document.getElementById('lf-tls-key').value = data.tls.key;
        }
    }

    // Timeouts
    if (data.disable_http2) document.getElementById('lf-disable-http2').checked = true;
    if (data.read_timeout) document.getElementById('lf-timeout-read').value = data.read_timeout;
    if (data.read_header_timeout) document.getElementById('lf-timeout-read-header').value = data.read_header_timeout;
    if (data.write_timeout) document.getElementById('lf-timeout-write').value = data.write_timeout;
    if (data.idle_timeout) document.getElementById('lf-timeout-idle').value = data.idle_timeout;

    // Step 2: Default route
    if (data.default) {
        document.getElementById('lf-default-enable').checked = true;
        document.getElementById('lf-default-route-fields').style.display = '';
        document.getElementById('lf-default-backend').value = (data.default.backend && data.default.backend.address) || '';
        if (data.default.backend && data.default.backend.replace_host_header) {
            document.getElementById('lf-default-replace-host').checked = true;
        }
        if (data.default.disable_default_middlewares) {
            document.getElementById('lf-default-disable-defaults').checked = true;
        }
        if (data.default.middlewares && data.default.middlewares.length > 0 && lfDefaultMwChain) {
            data.default.middlewares.forEach(function(m) { lfDefaultMwChain.add(m.type, m.options || {}); });
        }
        if (data.default.trusted_proxies && lfDefaultTpSection) {
            lfPopulateTrustedProxies(lfDefaultTpSection, data.default.trusted_proxies);
        }
        if (data.default.paths && data.default.paths.length > 0) {
            data.default.paths.forEach(function(p) {
                var pathObj = lfCreatePathCard(document.getElementById('lf-default-paths'), lfDefaultPaths);
                pathObj.el.querySelector('.lf-path-pattern').value = p.pattern || '';
                pathObj.el.querySelector('.lf-path-backend').value = (p.backend && p.backend.address) || '';
                if (p.backend && p.backend.replace_host_header) pathObj.el.querySelector('.lf-path-replace-host').checked = true;
                if (p.drop_query) pathObj.el.querySelector('.lf-path-drop-query').checked = true;
                if (p.strip_prefix) pathObj.el.querySelector('.lf-path-strip-prefix').checked = true;
                if (p.disable_default_middlewares) {
                    var pddCb = pathObj.el.querySelector('.lf-path-disable-defaults');
                    if (pddCb) pddCb.checked = true;
                }
                if (p.middlewares && p.middlewares.length > 0) {
                    p.middlewares.forEach(function(m) { pathObj.mwChain.add(m.type, m.options || {}); });
                }
                if (p.trusted_proxies && pathObj.tpSection) {
                    lfPopulateTrustedProxies(pathObj.tpSection, p.trusted_proxies);
                }
            });
        }
    }

    // Step 2: Host routes
    if (data.routes && data.routes.length > 0) {
        data.routes.forEach(function(route) {
            var routeObj = lfCreateHostRouteCard();
            routeObj.el.querySelector('.lf-route-host').value = route.host || '';
            routeObj.el.querySelector('.lf-route-backend').value = (route.target && route.target.backend && route.target.backend.address) || '';
            if (route.target && route.target.backend && route.target.backend.replace_host_header) {
                routeObj.el.querySelector('.lf-route-replace-host').checked = true;
            }
            var target = route.target || {};
            if (target.disable_default_middlewares) {
                var ddCb = routeObj.el.querySelector('.lf-route-disable-defaults');
                if (ddCb) ddCb.checked = true;
            }
            if (target.middlewares && target.middlewares.length > 0) {
                target.middlewares.forEach(function(m) { routeObj.mwChain.add(m.type, m.options || {}); });
            }
            if (target.trusted_proxies && routeObj.tpSection) {
                lfPopulateTrustedProxies(routeObj.tpSection, target.trusted_proxies);
            }
            if (target.paths && target.paths.length > 0) {
                var pathsContainer = routeObj.el.querySelector('.lf-route-paths-container');
                target.paths.forEach(function(p) {
                    var pathObj = lfCreatePathCard(pathsContainer, routeObj.paths);
                    pathObj.el.querySelector('.lf-path-pattern').value = p.pattern || '';
                    pathObj.el.querySelector('.lf-path-backend').value = (p.backend && p.backend.address) || '';
                    if (p.backend && p.backend.replace_host_header) pathObj.el.querySelector('.lf-path-replace-host').checked = true;
                    if (p.drop_query) pathObj.el.querySelector('.lf-path-drop-query').checked = true;
                    if (p.strip_prefix) pathObj.el.querySelector('.lf-path-strip-prefix').checked = true;
                    if (p.disable_default_middlewares) {
                        var pddCb = pathObj.el.querySelector('.lf-path-disable-defaults');
                        if (pddCb) pddCb.checked = true;
                    }
                    if (p.middlewares && p.middlewares.length > 0) {
                        p.middlewares.forEach(function(m) { pathObj.mwChain.add(m.type, m.options || {}); });
                    }
                    if (p.trusted_proxies && pathObj.tpSection) {
                        lfPopulateTrustedProxies(pathObj.tpSection, p.trusted_proxies);
                    }
                });
            }
        });
    }

    // Open the panel
    var panel = document.getElementById('create-proxy-panel');
    panel.style.display = '';
    document.getElementById('toggle-create-proxy').textContent = '\u2212 Cancel';
    panel.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function badge(val) {
    return val ? '<span class="badge badge-success">Yes</span>' : '<span class="badge badge-muted">No</span>';
}
