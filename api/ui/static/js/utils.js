// ---------- Toast notifications ----------
function showToast(message, type) {
    type = type || 'info';
    var container = document.getElementById('toast-container');
    var toast = document.createElement('div');
    toast.className = 'toast toast-' + type;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(function () {
        toast.classList.add('toast-fade-out');
        toast.addEventListener('animationend', function () {
            toast.remove();
        });
    }, 3000);
}

// ---------- SSE indicator ----------
var sseDot = document.getElementById('sse-dot');

function setSseStatus(status) {
    sseDot.className = 'sse-indicator ' + status;
    sseDot.title = 'SSE ' + status;
}

// ---------- Formatting helpers ----------
function fmtBytes(b) {
    if (b === null || b === undefined || isNaN(b) || b <= 0) return '0 B';
    var units = ['B', 'KB', 'MB', 'GB', 'TB'];
    var i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    if (i < 0) return '0 B';
    return (b / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + units[i];
}

function fmtNum(n) {
    return (n || 0).toLocaleString();
}

function timeLabel() {
    var d = new Date();
    return d.getHours().toString().padStart(2, '0') + ':' +
        d.getMinutes().toString().padStart(2, '0') + ':' +
        d.getSeconds().toString().padStart(2, '0');
}

function statusClass(code) {
    if (code >= 500) return 's5xx';
    if (code >= 400) return 's4xx';
    if (code >= 300) return 's3xx';
    return 's2xx';
}

function parsePort(connectionName) {
    if (!connectionName) return '–';
    var m = connectionName.match(/port-(\d+)/);
    return m ? m[1] : connectionName;
}

function matchesConnectionFilter(connName, filter) {
    if (!filter) return true;
    if (!connName) return false;
    if (connName === filter) return true;
    return connName.indexOf(filter + '-host-') === 0 ||
           connName.indexOf(filter + '-path-') === 0;
}

function blockIcon(mw) {
    switch (mw) {
        case 'honeypot':      return '🍯';
        case 'country-block': return '🌍';
        case 'ua-block':      return '🤖';
        case 'rate-limit':    return '⏱';
        case 'coraza-waf':    return '🛡️';
        case 'ip-block':      return '🚫';
        default:              return '🛡️';
    }
}

function blockMwIcon(mw) {
    switch (mw) {
        case 'honeypot':      return '🍯';
        case 'country-block': return '🌍';
        case 'ua-block':      return '🤖';
        case 'rate-limit':    return '⏱';
        case 'ip-block':      return '🚫';
        case 'coraza-waf':    return '🛡️';
        default:              return '🛡️';
    }
}

function badge(val) {
    return val ? '<span class="badge badge-success">Yes</span>' : '<span class="badge badge-muted">No</span>';
}

