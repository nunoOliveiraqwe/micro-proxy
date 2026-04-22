// ---------- System Health ----------
function fetchSystemHealth() {
    fetch('/api/v1/system/health', {credentials: 'same-origin'})
        .then(function (resp) {
            if (!resp.ok) return null;
            return resp.json();
        })
        .then(function (h) {
            if (!h) return;

            var secs = Math.floor(h.uptime_seconds);
            var days = Math.floor(secs / 86400);
            secs %= 86400;
            var hrs = Math.floor(secs / 3600);
            secs %= 3600;
            var mins = Math.floor(secs / 60);
            secs %= 60;
            var parts = [];
            if (days) parts.push(days + 'd');
            if (hrs) parts.push(hrs + 'h');
            if (mins) parts.push(mins + 'm');
            parts.push(secs + 's');
            document.getElementById('health-uptime-val').textContent = parts.join(' ');

            var cpuEl = document.getElementById('health-cpu-val');
            if (h.cpu_util_percent >= 0) {
                cpuEl.textContent = h.cpu_util_percent.toFixed(1) + '%';
                cpuEl.style.color = h.cpu_util_percent > 90 ? '#e74c3c' : h.cpu_util_percent > 70 ? '#f39c12' : '#7c6ef0';
            } else {
                cpuEl.textContent = '–';
            }

            var grEl = document.getElementById('health-goroutines-val');
            grEl.textContent = h.goroutines;
            grEl.style.color = h.goroutines > 150 ? '#e74c3c' : h.goroutines > 80 ? '#f39c12' : '#7c6ef0';

            document.getElementById('health-rss-val').textContent = fmtBytes(h.process_rss_bytes);
            document.getElementById('health-rss-sub').textContent = h.process_mem_percent.toFixed(1) + '% of system';

            document.getElementById('health-sysmem-val').textContent = h.sys_mem_used_percent.toFixed(1) + '%';
            document.getElementById('health-sysmem-sub').textContent = fmtBytes(h.sys_mem_total_bytes) + ' total';

            document.getElementById('health-heap-val').textContent = fmtBytes(h.heap_alloc_bytes);

            var gcMs = h.gc_pause_total_ns / 1e6;
            document.getElementById('health-gc-pause-val').textContent = gcMs < 1000 ? gcMs.toFixed(1) + ' ms' : (gcMs / 1000).toFixed(2) + ' s';
        })
        .catch(function () {
        });
}

// ---------- Error Log Feed ----------
var MAX_ERROR_ENTRIES = 20;
var errorEntries = [];
var errorLogFilter = '';

function buildErrorRow(e) {
    var ts = new Date(e.timestamp);
    var time = ts.getHours().toString().padStart(2, '0') + ':' +
        ts.getMinutes().toString().padStart(2, '0') + ':' +
        ts.getSeconds().toString().padStart(2, '0');
    var row = document.createElement('div');
    row.className = 'error-entry';
    row.innerHTML =
        '<span class="error-time">' + time + '</span>' +
        '<span class="error-remote">' + (e.remote_address || '–') + '</span>' +
        '<span class="error-status">' + (e.status_code || '5xx') + '</span>' +
        '<span class="error-method">' + (e.method || '?') + '</span>' +
        '<span class="error-path" title="' + (e.path || '').replace(/"/g, '&quot;') + '">' + (e.path || '/') + '</span>' +
        '<span class="error-latency">' + (e.latency_ms || 0) + ' ms</span>';
    return row;
}

var errorFeedHeader = '<div class="error-entry req-header">' +
    '<span class="error-time">Time</span>' +
    '<span class="error-remote">Remote</span>' +
    '<span class="error-status">Code</span>' +
    '<span class="error-method">Method</span>' +
    '<span class="error-path">Path</span>' +
    '<span class="error-latency">Latency</span>' +
    '</div>';

function ensureErrorFeedHeader(el) {
    if (!el.querySelector('.req-header')) {
        el.innerHTML = errorFeedHeader;
    }
}

function addErrorToFeed(entry) {
    errorEntries.unshift(entry);
    if (errorEntries.length > MAX_ERROR_ENTRIES) errorEntries.pop();
    if (!matchesConnectionFilter(entry.connection_name, errorLogFilter)) return;
    var el = document.getElementById('error-feed');
    var empty = el.querySelector('.error-empty');
    if (empty) empty.remove();
    ensureErrorFeedHeader(el);
    var header = el.querySelector('.req-header');
    var row = buildErrorRow(entry);
    header.insertAdjacentElement('afterend', row);
    while (el.children.length > MAX_ERROR_ENTRIES + 1) {
        el.removeChild(el.lastElementChild);
    }
}

function renderErrorFeed() {
    var el = document.getElementById('error-feed');
    var filtered = errorEntries.filter(function (e) {
        return matchesConnectionFilter(e.connection_name, errorLogFilter);
    });
    if (filtered.length === 0) {
        el.innerHTML = '<div class="error-empty">No errors recorded</div>';
        return;
    }
    var html = errorFeedHeader;
    filtered.forEach(function (e) {
        html += buildErrorRow(e).outerHTML;
    });
    el.innerHTML = html;
}

function loadRecentErrors() {
    return fetch('/api/v1/proxy/errors', {credentials: 'same-origin'})
        .then(function (resp) {
            return resp.ok ? resp.json() : [];
        })
        .then(function (entries) {
            if (entries && entries.length) {
                errorEntries = entries.slice(0, MAX_ERROR_ENTRIES);
                renderErrorFeed();
            }
        })
        .catch(function () {
        });
}

// ---------- Request Log Feed ----------
var MAX_REQUEST_ENTRIES = 100;
var requestEntries = [];
var requestLogFilter = '';

function buildRequestRow(e) {
    var ts = new Date(e.timestamp);
    var time = ts.getHours().toString().padStart(2, '0') + ':' +
        ts.getMinutes().toString().padStart(2, '0') + ':' +
        ts.getSeconds().toString().padStart(2, '0');
    var sc = e.status_code || 0;
    var blockedTag = '';
    if (e.blocked_by) {
        var icon = blockIcon(e.blocked_by);
        var detail = e.blocked_by;
        if (e.block_reason) detail += ' (' + e.block_reason + ')';
        if (e.trickster_mode) detail += ' 🎭 trickster';
        var title = 'Blocked by ' + detail;
        if (e.block_client_ip) title += ' | IP: ' + e.block_client_ip;
        blockedTag = '<span class="req-blocked" style="color:#e67e22;font-size:0.75rem;font-weight:600;" title="' + title.replace(/"/g, '&quot;') + '">' + icon + ' ' + e.blocked_by + (e.trickster_mode ? ' 🎭' : '') + '</span>';
    }
    var row = document.createElement('div');
    row.className = 'req-entry';
    row.innerHTML =
        '<span class="req-time">' + time + '</span>' +
        '<span class="req-remote">' + (e.remote_address || '–') + '</span>' +
        '<span class="req-country">' + (e.country || '–') + '</span>' +
        '<span class="req-route">:' + parsePort(e.connection_name) + '</span>' +
        '<span class="req-path" title="' + (e.path || '').replace(/"/g, '&quot;') + '">' + (e.path || '/') + '</span>' +
        '<span class="req-method">' + (e.method || '?') + '</span>' +
        '<span class="req-status ' + statusClass(sc) + '">' + (sc || '–') + '</span>' +
        '<span class="req-latency">' + (e.latency_ms || 0) + ' ms</span>' +
        blockedTag;
    return row;
}

var requestFeedHeader = '<div class="req-entry req-header">' +
    '<span class="req-time">Time</span>' +
    '<span class="req-remote">Remote</span>' +
    '<span class="req-country">Country</span>' +
    '<span class="req-route">Route</span>' +
    '<span class="req-path">Path</span>' +
    '<span class="req-method">Method</span>' +
    '<span class="req-status">Code</span>' +
    '<span class="req-latency">Latency</span>' +
    '</div>';

function ensureRequestFeedHeader(el) {
    if (!el.querySelector('.req-header')) {
        el.innerHTML = requestFeedHeader;
    }
}

function addRequestToFeed(entry) {
    requestEntries.unshift(entry);
    if (requestEntries.length > MAX_REQUEST_ENTRIES) requestEntries.pop();
    if (!matchesConnectionFilter(entry.connection_name, requestLogFilter)) return;
    var el = document.getElementById('request-feed');
    var empty = el.querySelector('.error-empty');
    if (empty) empty.remove();
    ensureRequestFeedHeader(el);
    var header = el.querySelector('.req-header');
    var row = buildRequestRow(entry);
    header.insertAdjacentElement('afterend', row);
    while (el.children.length > MAX_REQUEST_ENTRIES + 1) {
        el.removeChild(el.lastElementChild);
    }
}

function renderRequestFeed() {
    var el = document.getElementById('request-feed');
    var filtered = requestEntries.filter(function (e) {
        return matchesConnectionFilter(e.connection_name, requestLogFilter);
    });
    if (filtered.length === 0) {
        el.innerHTML = '<div class="error-empty">No requests recorded</div>';
        return;
    }
    var html = requestFeedHeader;
    filtered.forEach(function (e) {
        html += buildRequestRow(e).outerHTML;
    });
    el.innerHTML = html;
}

function loadRecentRequests() {
    return fetch('/api/v1/proxy/requests', {credentials: 'same-origin'})
        .then(function (resp) {
            return resp.ok ? resp.json() : [];
        })
        .then(function (entries) {
            if (entries && entries.length) {
                requestEntries = entries.slice(0, MAX_REQUEST_ENTRIES);
                renderRequestFeed();
            }
        })
        .catch(function () {
        });
}

// ---------- Block Log Feed ----------
var MAX_BLOCK_ENTRIES = 50;
var blockEntries = [];
var blockLogFilter = '';

function buildBlockRow(e) {
    var ts = new Date(e.timestamp);
    var time = ts.getHours().toString().padStart(2, '0') + ':' +
        ts.getMinutes().toString().padStart(2, '0') + ':' +
        ts.getSeconds().toString().padStart(2, '0');
    var icon = blockMwIcon(e.blocking_middleware);
    var row = document.createElement('div');
    row.className = 'block-entry';
    row.innerHTML =
        '<span class="block-time">' + time + '</span>' +
        '<span class="block-remote">' + (e.remote_address || '–') + '</span>' +
        '<span class="block-mw" title="' + (e.block_reason || '').replace(/"/g, '&quot;') + '">' + icon + ' ' + (e.blocking_middleware || '?') + '</span>' +
        '<span class="block-method">' + (e.method || '?') + '</span>' +
        '<span class="block-path" title="' + (e.path || '').replace(/"/g, '&quot;') + '">' + (e.path || '/') + '</span>' +
        '<span class="block-reason" title="' + (e.block_reason || '').replace(/"/g, '&quot;') + '">' + (e.block_reason || '') + '</span>';
    return row;
}

var blockFeedHeader = '<div class="block-entry req-header">' +
    '<span class="block-time">Time</span>' +
    '<span class="block-remote">Remote</span>' +
    '<span class="block-mw">Middleware</span>' +
    '<span class="block-method">Method</span>' +
    '<span class="block-path">Path</span>' +
    '<span class="block-reason">Reason</span>' +
    '</div>';

function ensureBlockFeedHeader(el) {
    if (!el.querySelector('.req-header')) {
        el.innerHTML = blockFeedHeader;
    }
}

function addBlockToFeed(entry) {
    blockEntries.unshift(entry);
    if (blockEntries.length > MAX_BLOCK_ENTRIES) blockEntries.pop();
    if (!matchesConnectionFilter(entry.connection_name, blockLogFilter)) return;
    var el = document.getElementById('block-feed');
    var empty = el.querySelector('.error-empty');
    if (empty) empty.remove();
    ensureBlockFeedHeader(el);
    var header = el.querySelector('.req-header');
    var row = buildBlockRow(entry);
    header.insertAdjacentElement('afterend', row);
    while (el.children.length > MAX_BLOCK_ENTRIES + 1) {
        el.removeChild(el.lastElementChild);
    }
}

function renderBlockFeed() {
    var el = document.getElementById('block-feed');
    var filtered = blockEntries.filter(function (e) {
        return matchesConnectionFilter(e.connection_name, blockLogFilter);
    });
    if (filtered.length === 0) {
        el.innerHTML = '<div class="error-empty">No blocked requests</div>';
        return;
    }
    var html = blockFeedHeader;
    filtered.forEach(function (e) {
        html += buildBlockRow(e).outerHTML;
    });
    el.innerHTML = html;
}

function loadRecentBlocked() {
    return fetch('/api/v1/proxy/blocked', {credentials: 'same-origin'})
        .then(function (resp) {
            return resp.ok ? resp.json() : [];
        })
        .then(function (entries) {
            if (entries && entries.length) {
                blockEntries = entries.slice(0, MAX_BLOCK_ENTRIES);
                renderBlockFeed();
            }
        })
        .catch(function () {
        });
}

// ---------- Unified Activity Feed ----------
var MAX_ACTIVITY_ENTRIES = 200;
var activityEntries = [];
var feedShowRequests = true;
var feedShowErrors = true;
var feedShowBlocked = true;
var activityLogFilter = '';

function rebuildActivityFromFeeds() {
    activityEntries = [];
    requestEntries.forEach(function(e) {
        activityEntries.push({type:'request', ts:new Date(e.timestamp), entry:e});
    });
    errorEntries.forEach(function(e) {
        activityEntries.push({type:'error', ts:new Date(e.timestamp), entry:e});
    });
    blockEntries.forEach(function(e) {
        activityEntries.push({type:'blocked', ts:new Date(e.timestamp), entry:e});
    });
    activityEntries.sort(function(a,b){ return b.ts - a.ts; });
    if (activityEntries.length > MAX_ACTIVITY_ENTRIES) activityEntries.length = MAX_ACTIVITY_ENTRIES;
    renderActivityFeed();
}

function activityBadge(type) {
    switch (type) {
        case 'request': return '<span class="activity-badge">🟢</span>';
        case 'error':   return '<span class="activity-badge">🔴</span>';
        case 'blocked': return '<span class="activity-badge">🟠</span>';
        default:        return '<span class="activity-badge">⚪</span>';
    }
}

var activityFeedHeader = '<div class="activity-entry activity-header">' +
    '<span class="activity-badge"></span>' +
    '<span class="act-time">Time</span>' +
    '<span class="act-remote">Address</span>' +
    '<span class="act-method">Method</span>' +
    '<span class="act-status">Code</span>' +
    '<span class="act-latency">Latency</span>' +
    '<span class="act-path">Path</span>' +
    '<span class="act-reason">Reason</span>' +
    '</div>';

function ensureActivityFeedHeader(el) {
    if (!el.querySelector('.activity-header')) {
        el.insertAdjacentHTML('afterbegin', activityFeedHeader);
    }
}

function fmtActivityTime(ts) {
    var d = new Date(ts);
    return d.getHours().toString().padStart(2,'0') + ':' +
           d.getMinutes().toString().padStart(2,'0') + ':' +
           d.getSeconds().toString().padStart(2,'0');
}

function buildActivityRow(item) {
    var e = item.entry;
    var time = fmtActivityTime(e.timestamp);
    var row = document.createElement('div');
    row.className = 'activity-entry';
    row.setAttribute('data-activity-type', item.type);

    if (item.type === 'request') {
        var sc = e.status_code || 0;
        var scClass = sc >= 500 ? 's5xx' : sc >= 400 ? 's4xx' : sc >= 300 ? 's3xx' : 's2xx';
        row.innerHTML = activityBadge('request') +
            '<span class="act-time">' + time + '</span>' +
            '<span class="act-remote">' + (e.remote_address || '–') + '</span>' +
            '<span class="act-method">' + (e.method || '?') + '</span>' +
            '<span class="act-status ' + scClass + '">' + (sc || '–') + '</span>' +
            '<span class="act-latency">' + (e.latency_ms || 0) + ' ms</span>' +
            '<span class="act-path" title="' + (e.path||'').replace(/"/g,'&quot;') + '">' + (e.path || '/') + '</span>' +
            '<span class="act-reason"></span>';
    } else if (item.type === 'error') {
        row.innerHTML = activityBadge('error') +
            '<span class="act-time">' + time + '</span>' +
            '<span class="act-remote">' + (e.remote_address || '–') + '</span>' +
            '<span class="act-method">' + (e.method || '?') + '</span>' +
            '<span class="act-status s5xx">' + (e.status_code || '5xx') + '</span>' +
            '<span class="act-latency">' + (e.latency_ms || 0) + ' ms</span>' +
            '<span class="act-path" title="' + (e.path||'').replace(/"/g,'&quot;') + '">' + (e.path || '/') + '</span>' +
            '<span class="act-reason"></span>';
    } else if (item.type === 'blocked') {
        var icon = blockMwIcon(e.blocking_middleware);
        var mwLabel = e.blocking_middleware || '?';
        var reason = e.block_reason || '';
        var fullDetail = mwLabel + (reason ? ': ' + reason : '');
        row.innerHTML = activityBadge('blocked') +
            '<span class="act-time">' + time + '</span>' +
            '<span class="act-remote">' + (e.remote_address || '–') + '</span>' +
            '<span class="act-method">' + (e.method || '?') + '</span>' +
            '<span class="act-status" style="color:#e67e22;">' + icon + '</span>' +
            '<span class="act-latency"></span>' +
            '<span class="act-path" title="' + (e.path||'').replace(/"/g,'&quot;') + '">' + (e.path || '/') + '</span>' +
            '<span class="act-reason" title="' + fullDetail.replace(/"/g,'&quot;') + '">' + fullDetail + '</span>';
    }
    return row;
}

function addToActivityFeed(type, entry) {
    var ts = new Date(entry.timestamp);
    activityEntries.unshift({type: type, ts: ts, entry: entry});
    if (activityEntries.length > MAX_ACTIVITY_ENTRIES) activityEntries.pop();

    if (type === 'request' && !feedShowRequests) return;
    if (type === 'error' && !feedShowErrors) return;
    if (type === 'blocked' && !feedShowBlocked) return;
    if (!matchesConnectionFilter(entry.connection_name, activityLogFilter)) return;

    var el = document.getElementById('activity-feed');
    var empty = el.querySelector('.error-empty');
    if (empty) empty.remove();
    ensureActivityFeedHeader(el);

    var header = el.querySelector('.activity-header');
    var row = buildActivityRow({type: type, ts: ts, entry: entry});
    header.insertAdjacentElement('afterend', row);

    while (el.children.length > MAX_ACTIVITY_ENTRIES + 1) {
        el.removeChild(el.lastElementChild);
    }
}

function renderActivityFeed() {
    var el = document.getElementById('activity-feed');
    var filtered = activityEntries.filter(function(item) {
        if (item.type === 'request' && !feedShowRequests) return false;
        if (item.type === 'error' && !feedShowErrors) return false;
        if (item.type === 'blocked' && !feedShowBlocked) return false;
        return matchesConnectionFilter(item.entry.connection_name, activityLogFilter);
    });
    if (filtered.length === 0) {
        el.innerHTML = '<div class="error-empty">No activity recorded</div>';
        return;
    }
    el.innerHTML = activityFeedHeader;
    filtered.forEach(function(item) {
        el.appendChild(buildActivityRow(item));
    });
}

// Wire up filter link toggles
document.querySelectorAll('.feed-link').forEach(function(link) {
    link.addEventListener('click', function(e) {
        e.preventDefault();
        this.classList.toggle('active');
        var type = this.getAttribute('data-feed-type');
        if (type === 'requests') feedShowRequests = this.classList.contains('active');
        if (type === 'errors') feedShowErrors = this.classList.contains('active');
        if (type === 'blocked') feedShowBlocked = this.classList.contains('active');
        renderActivityFeed();
    });
});

// ---------- Log Filter Dropdowns ----------
function buildLogFilterOptions() {
    var names = Object.keys(perConnectionSnapshots).sort();
    var html = '<option value="">All connections</option>';
    var ports = {};
    names.forEach(function (name) {
        if (name === 'global') return;
        var portKey = extractPortKey(name);
        if (portKey === name) {
            if (!ports[name]) ports[name] = [];
        } else {
            if (!ports[portKey]) ports[portKey] = [];
            ports[portKey].push(name);
        }
    });
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
    return html;
}

function updateLogFilterDropdowns() {
    var html = buildLogFilterOptions();
    var sysSel = document.getElementById('system-log-filter');
    var sysCur = sysSel.value;
    sysSel.innerHTML = html;
    if (sysCur && sysSel.querySelector('option[value="' + sysCur + '"]')) sysSel.value = sysCur;
}

document.getElementById('system-log-filter').addEventListener('change', function () {
    var filter = this.value;
    requestLogFilter = filter;
    errorLogFilter = filter;
    blockLogFilter = filter;
    activityLogFilter = filter;
    renderActivityFeed();
});

