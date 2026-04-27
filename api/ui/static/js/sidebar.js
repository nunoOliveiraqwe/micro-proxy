// ---------- Sidebar collapse ----------
var sidebarEl = document.getElementById('app-sidebar');
var sidebarToggle = document.getElementById('sidebar-toggle');
var sidebarCollapsed = localStorage.getItem('sidebar-collapsed') === 'true';

function applySidebarState() {
    sidebarEl.classList.toggle('collapsed', sidebarCollapsed);
    sidebarToggle.textContent = sidebarCollapsed ? '▶' : '◀';
    sidebarToggle.title = sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar';
    if (typeof chart !== 'undefined' && chart) {
        setTimeout(function() { chart.resize(); }, 200);
    }
}
applySidebarState();

sidebarToggle.addEventListener('click', function () {
    sidebarCollapsed = !sidebarCollapsed;
    localStorage.setItem('sidebar-collapsed', sidebarCollapsed);
    applySidebarState();
});

// ---------- Theme toggle ----------
var themeToggleBtn = document.getElementById('theme-toggle');
var themeKnob = document.getElementById('theme-knob');
function getTheme() {
    return document.documentElement.getAttribute('data-theme') || 'dark';
}
function applyThemeState() {
    if (themeKnob) themeKnob.textContent = getTheme() === 'dark' ? '🌙' : '☀️';
}
applyThemeState();

if (themeToggleBtn) {
    themeToggleBtn.addEventListener('click', function () {
        var next = getTheme() === 'dark' ? 'light' : 'dark';
        document.documentElement.setAttribute('data-theme', next);
        localStorage.setItem('torii-theme', next);
        applyThemeState();
        updateChartTheme();
    });
}

function updateChartTheme() {
    if (typeof chart === 'undefined' || !chart) return;
    var isDark = getTheme() === 'dark';
    var gridColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.08)';
    var tickColor = isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)';
    chart.options.scales.x.grid.color = gridColor;
    chart.options.scales.x.ticks.color = tickColor;
    chart.options.scales.y.grid.color = gridColor;
    chart.options.scales.y.ticks.color = tickColor;
    chart.update();
}

// ---------- Page navigation ----------
var navLinks = document.querySelectorAll('.nav-link');
var pages = document.querySelectorAll('.page');
var currentPage = 'dashboard';
var healthInterval = null;

// ACME inner tabs
document.querySelectorAll('#acme-tabs .inner-tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
        var targetId = this.getAttribute('data-acme-tab');
        document.querySelectorAll('#acme-tabs .inner-tab').forEach(function (t) {
            t.classList.remove('active');
        });
        this.classList.add('active');
        document.querySelectorAll('.inner-tab-panel[id^="acme-tab-"]').forEach(function (panel) {
            panel.style.display = panel.id === targetId ? '' : 'none';
        });
        if (targetId === 'acme-tab-certs') {
            loadAcmeCertificates();
        }
        if (targetId === 'acme-tab-domains') {
            loadAcmeDomains();
        }
    });
});

// API Keys inner tabs
document.querySelectorAll('#api-keys-tabs .inner-tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
        var targetId = this.getAttribute('data-apikeys-tab');
        document.querySelectorAll('#api-keys-tabs .inner-tab').forEach(function (t) {
            t.classList.remove('active');
        });
        this.classList.add('active');
        document.querySelectorAll('.inner-tab-panel[id^="apikeys-tab-"]').forEach(function (panel) {
            panel.style.display = panel.id === targetId ? '' : 'none';
        });
        if (targetId === 'apikeys-tab-list') {
            loadApiKeys();
        }
    });
});

// Proxy inner tabs (Routes / Cache)
document.querySelectorAll('#proxy-tabs .inner-tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
        var targetId = this.getAttribute('data-proxy-tab');
        document.querySelectorAll('#proxy-tabs .inner-tab').forEach(function (t) {
            t.classList.remove('active');
        });
        this.classList.add('active');
        document.querySelectorAll('.inner-tab-panel[id^="proxy-tab-"]').forEach(function (panel) {
            panel.style.display = panel.id === targetId ? '' : 'none';
        });
        // Show Create Proxy button only on Routes tab
        document.getElementById('toggle-create-proxy').style.display =
            targetId === 'proxy-tab-routes' ? '' : 'none';
        if (targetId === 'proxy-tab-cache') {
            loadCacheInsights();
            if (!cacheInsightsInterval) cacheInsightsInterval = setInterval(loadCacheInsights, 5000);
        } else {
            if (cacheInsightsInterval) {
                clearInterval(cacheInsightsInterval);
                cacheInsightsInterval = null;
            }
        }
    });
});

function showPage(pageId) {
    currentPage = pageId;
    pages.forEach(function (p) {
        if (p.getAttribute('data-page') === pageId) {
            p.style.display = '';
            p.style.animation = 'none';
            p.offsetHeight;
            p.style.animation = '';
        } else {
            p.style.display = 'none';
        }
    });
    navLinks.forEach(function (a) {
        a.classList.toggle('active', a.getAttribute('data-page') === pageId);
    });
    document.querySelector('.app-main').scrollTo(0, 0);
    if (pageId === 'dashboard' && chart) chart.resize();

    if (pageId === 'system') {
        fetchSystemHealth();
        activityEntries = [];
        Promise.all([loadRecentRequests(), loadRecentErrors(), loadRecentBlocked()])
            .then(rebuildActivityFromFeeds);
        if (!healthInterval) healthInterval = setInterval(fetchSystemHealth, 5000);
    } else {
        if (healthInterval) {
            clearInterval(healthInterval);
            healthInterval = null;
        }
    }

    if (pageId === 'dashboard' || pageId === 'proxy-routes') {
        if (!routeInterval) {
            loadProxyRoutes();
            routeInterval = setInterval(loadProxyRoutes, 10000);
        }
    } else {
        if (routeInterval) {
            clearInterval(routeInterval);
            routeInterval = null;
        }
    }

    if (pageId === 'acme') {
        loadAcmeProviders().then(function () {
            loadAcmeConfig();
        });
    }

    if (pageId === 'proxy-routes') {
        initListenerWizard();
        lastRouteFingerprint = '';
        // Reset to Routes tab
        document.querySelectorAll('#proxy-tabs .inner-tab').forEach(function (t) {
            t.classList.toggle('active', t.getAttribute('data-proxy-tab') === 'proxy-tab-routes');
        });
        document.getElementById('proxy-tab-routes').style.display = '';
        document.getElementById('proxy-tab-cache').style.display = 'none';
        document.getElementById('toggle-create-proxy').style.display = '';
    } else {
        if (cacheInsightsInterval) {
            clearInterval(cacheInsightsInterval);
            cacheInsightsInterval = null;
        }
    }

    if (pageId === 'api-keys') {
        // Keys load on Existing Keys tab click
    } else {
        document.getElementById('api-key-created-banner').style.display = 'none';
        document.getElementById('api-key-created-value').textContent = '';
    }

}

navLinks.forEach(function (link) {
    link.addEventListener('click', function (e) {
        e.preventDefault();
        showPage(this.getAttribute('data-page'));
    });
});

