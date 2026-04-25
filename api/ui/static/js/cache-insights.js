// ---------- Cache Insights ----------
var cacheInsightsInterval = null;

function loadCacheInsights() {
    fetch('/api/v1/cache/insights', {credentials: 'same-origin'})
        .then(function (r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
        })
        .then(renderCacheInsights)
        .catch(function (err) {
            var container = document.getElementById('cache-insights-container');
            container.innerHTML = '<div class="error-empty">Failed to load cache data: ' + err.message + '</div>';
        });
}

function renderCacheInsights(caches) {
    var container = document.getElementById('cache-insights-container');
    if (!caches || caches.length === 0) {
        container.innerHTML = '<div class="error-empty">No caches registered</div>';
        return;
    }

    var html = '';
    for (var i = 0; i < caches.length; i++) {
        var c = caches[i];
        var usagePct = c.max_entries > 0 ? ((c.current_entries / c.max_entries) * 100).toFixed(1) : '0.0';

        html += '<div class="stat-card" style="text-align:left;margin-bottom:1.5rem;">';
        html += '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem;">';
        html += '<h4 style="margin:0;">' + escapeHtml(c.name) + '</h4>';
        html += '<span style="font-size:0.75rem;color:var(--pico-muted-color);">' + fmtNum(c.current_entries) + ' / ' + fmtNum(c.max_entries) + ' entries (' + usagePct + '%)</span>';
        html += '</div>';

        // Capacity bar
        html += '<div class="cache-capacity-bar" style="margin-bottom:1.25rem;">';
        html += '<div class="cache-capacity-fill" style="width:' + Math.min(parseFloat(usagePct), 100) + '%;"></div>';
        html += '</div>';

        // Stats
        html += '<div class="stats-grid">';

        // Hits
        html += '<div class="stat-card">';
        html += '<p class="stat-value" style="color:#e74c3c">' + fmtNum(c.hits) + '</p>';
        html += '<p class="stat-label">Cache Hits</p>';
        html += '</div>';

        // M1 rate
        html += '<div class="stat-card">';
        html += '<p class="stat-value">' + c.m1_rate.toFixed(2) + '</p>';
        html += '<p class="stat-label">Rate</p>';
        html += '<p class="stat-sub">entries/sec (1m avg)</p>';
        html += '</div>';

        // Insertions
        html += '<div class="stat-card">';
        html += '<p class="stat-value">' + fmtNum(c.insertion_total) + '</p>';
        html += '<p class="stat-label">Insertions</p>';
        html += '</div>';

        html += '</div>'; // stats-grid

        // Keys (flagged IPs)
        html += '<details open style="margin-top:1rem;">';
        html += '<summary style="cursor:pointer;font-size:0.85rem;font-weight:600;">Cached Keys (' + (c.keys ? c.keys.length : 0) + ')</summary>';
        if (c.keys && c.keys.length > 0) {
            html += '<div class="cache-keys-list">';
            for (var j = 0; j < c.keys.length; j++) {
                html += '<code class="cache-key-tag">' + escapeHtml(c.keys[j]) + '</code>';
            }
            html += '</div>';
        } else {
            html += '<p style="font-size:0.8rem;color:var(--pico-muted-color);margin:0.5rem 0 0;">No entries cached</p>';
        }
        html += '</details>';

        html += '</div>'; // stat-card
    }

    container.innerHTML = html;
}

function escapeHtml(str) {
    var div = document.createElement('div');
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
}



