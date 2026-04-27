// ---------- ACME / TLS ----------
var acmeProviders = [];
var acmeConfigExists = false;
var acmeDomainsInitialized = false;

function lockAcmeForm(locked) {
    var fields = ['acme-email', 'acme-dns-provider', 'acme-ca-dir-url', 'acme-renewal-interval'];
    fields.forEach(function (id) {
        document.getElementById(id).disabled = locked;
    });
    document.querySelectorAll('#acme-credential-fields input').forEach(function (inp) {
        inp.disabled = locked;
    });
    document.getElementById('acme-save-btn').style.display = locked ? 'none' : '';
    document.getElementById('acme-locked-notice').style.display = locked ? '' : 'none';
    document.getElementById('acme-reset-btn').style.display = locked ? '' : 'none';
}

function loadAcmeProviders() {
    return fetch('/api/v1/acme/providers', {credentials: 'same-origin'})
        .then(function (r) {
            return r.ok ? r.json() : [];
        })
        .then(function (list) {
            acmeProviders = list || [];
            var sel = document.getElementById('acme-dns-provider');
            while (sel.options.length > 1) sel.remove(1);
            acmeProviders.forEach(function (p) {
                var opt = document.createElement('option');
                opt.value = p.name;
                opt.textContent = p.name;
                sel.appendChild(opt);
            });
        });
}

function renderCredentialFields(providerName, values) {
    var container = document.getElementById('acme-credential-fields');
    container.innerHTML = '';
    var provider = acmeProviders.find(function (p) {
        return p.name === providerName;
    });
    if (!provider || !provider.fields) return;
    provider.fields.forEach(function (f) {
        var val = (values && values[f.key]) || '';
        var inputType = f.sensitive ? 'password' : 'text';
        var requiredAttr = f.required ? 'required ' : '';
        var placeholder = f.placeholder || '';
        var html = '<label for="acme-cred-' + f.key + '" style="font-size:0.85rem;margin-bottom:0.25rem;">' +
            f.label + (f.required ? '' : ' <small>(optional)</small>') +
            '<input type="' + inputType + '" id="acme-cred-' + f.key + '" ' +
            'data-cred-key="' + f.key + '" ' +
            'placeholder="' + placeholder + '" ' +
            requiredAttr +
            'value="' + val.replace(/"/g, '&quot;') + '" ' +
            'style="margin-bottom:0.5rem;" autocomplete="off">' +
            '</label>';
        container.insertAdjacentHTML('beforeend', html);
    });
}

document.getElementById('acme-dns-provider').addEventListener('change', function () {
    renderCredentialFields(this.value, {});
});

function loadAcmeConfig() {
    fetch('/api/v1/acme/config', {credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return null;
            }
            if (!resp.ok) throw new Error(resp.statusText);
            return resp.json();
        })
        .then(function (conf) {
            if (!conf) return;
            acmeConfigExists = conf.configured;
            document.getElementById('acme-enabled').checked = conf.enabled;
            document.getElementById('acme-enabled-label').textContent = conf.enabled ? 'Enabled' : 'Disabled';
            document.getElementById('acme-email').value = conf.email || '';
            document.getElementById('acme-dns-provider').value = conf.dnsProvider || '';
            document.getElementById('acme-ca-dir-url').value = conf.caDirUrl || '';
            document.getElementById('acme-renewal-interval').value = conf.renewalCheckInterval || '12h';
            renderCredentialFields(conf.dnsProvider, {});
            lockAcmeForm(acmeConfigExists);
        })
        .catch(function (err) {
            console.error('Failed to load ACME config', err);
        });
}

// ---------- ACME Domains Tab ----------

function loadAcmeDomains() {
    fetch('/api/v1/acme/config', {credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) { window.location.href = '/ui/login'; return null; }
            if (!resp.ok) throw new Error(resp.statusText);
            return resp.json();
        })
        .then(function (conf) {
            if (!conf) return;
            var container = document.getElementById('acme-domains-input');
            var domains = conf.domains || [];

            // Re-initialize the tag input each time we load
            container.innerHTML = '';
            container.classList.remove('mw-tag-input');
            acmeDomainsInitialized = false;

            lfInitTagInput(container, domains);
            acmeDomainsInitialized = true;
        })
        .catch(function (err) {
            console.error('Failed to load ACME domains', err);
        });
}

document.getElementById('acme-save-domains-btn').addEventListener('click', function () {
    var container = document.getElementById('acme-domains-input');
    var domains = lfCollectStringListValues(container);
    var resultEl = document.getElementById('acme-domains-result');
    resultEl.innerHTML = '';

    fetch('/api/v1/acme/domains', {
        method: 'PUT', credentials: 'same-origin',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({domains: domains})
    })
        .then(function (resp) {
            if (resp.status === 401) { window.location.href = '/ui/login'; return; }
            if (!resp.ok) return resp.text().then(function (t) {
                resultEl.innerHTML = '<div class="error-alert">' + (t || 'Failed to update domains.') + '</div>';
            });
            showToast('ACME domains updated.', 'success');
        })
        .catch(function () {
            resultEl.innerHTML = '<div class="error-alert">An error occurred. Please try again.</div>';
        });
});

// ---------- Certificates Tab ----------

function loadAcmeCertificates() {
    fetch('/api/v1/acme/certificates', {credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return null;
            }
            if (!resp.ok) throw new Error(resp.statusText);
            return resp.json();
        })
        .then(function (certs) {
            if (!certs) return;
            var tbody = document.getElementById('acme-certs-body');
            if (!certs.length) {
                tbody.innerHTML = '<tr><td colspan="4" class="text-center" style="padding:1.5rem;color:var(--pico-muted-color);">No certificates yet.</td></tr>';
                return;
            }
            var html = '';
            certs.forEach(function (c) {
                var expires = new Date(c.expiresAt);
                var created = new Date(c.createdAt);
                var now = new Date();
                var daysLeft = Math.ceil((expires - now) / (1000 * 60 * 60 * 24));
                var badgeClass = daysLeft <= 7 ? 'badge-danger' : daysLeft <= 30 ? 'badge-info' : 'badge-success';
                var statusBadge = c.active
                    ? '<span class="badge badge-success">Active</span>'
                    : '<span class="badge badge-muted">Orphaned</span>';
                html += '<tr>' +
                    '<td><code>' + c.domain + '</code></td>' +
                    '<td>' + statusBadge + '</td>' +
                    '<td><span class="badge ' + badgeClass + '">' + daysLeft + 'd</span> ' + expires.toLocaleDateString() + '</td>' +
                    '<td>' + created.toLocaleDateString() + '</td>' +
                    '</tr>';
            });
            tbody.innerHTML = html;
        })
        .catch(function (err) {
            console.error('Failed to load ACME certificates', err);
        });
}

// ---------- Toggle ----------

document.getElementById('acme-enabled').addEventListener('change', function () {
    var enabled = this.checked;
    document.getElementById('acme-enabled-label').textContent = enabled ? 'Enabled' : 'Disabled';
    if (!acmeConfigExists) return;
    fetch('/api/v1/acme/config', {
        method: 'PATCH', credentials: 'same-origin',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({enabled: enabled})
    })
        .then(function (resp) {
            if (!resp.ok) return resp.text().then(function (t) {
                showToast(t || 'Failed to toggle ACME.', 'error');
            });
            showToast('ACME ' + (enabled ? 'enabled' : 'disabled') + '.', 'success');
        })
        .catch(function () {
            showToast('Failed to toggle ACME state.', 'error');
        });
});

// ---------- Save Config (first-time setup) ----------

document.getElementById('acme-config-form').addEventListener('submit', function (e) {
    e.preventDefault();
    var resultEl = document.getElementById('acme-config-result');
    resultEl.innerHTML = '';

    var providerName = document.getElementById('acme-dns-provider').value;

    var configMap = {};
    document.querySelectorAll('#acme-credential-fields [data-cred-key]').forEach(function (inp) {
        configMap[inp.getAttribute('data-cred-key')] = inp.value;
    });

    var body = {
        email: document.getElementById('acme-email').value,
        caDirUrl: document.getElementById('acme-ca-dir-url').value,
        renewalCheckInterval: document.getElementById('acme-renewal-interval').value || '12h',
        enabled: document.getElementById('acme-enabled').checked,
        domains: [],
        dns_provider_config_request: {
            provider: providerName,
            configurationMap: configMap
        }
    };
    fetch('/api/v1/acme/config', {
        method: 'POST', credentials: 'same-origin',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(body)
    })
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return;
            }
            if (!resp.ok) return resp.text().then(function (t) {
                resultEl.innerHTML = '<div class="error-alert">' + (t || 'Failed to save configuration.') + '</div>';
            });
            showToast('ACME configuration saved and applied.', 'success');
            loadAcmeConfig();
        })
        .catch(function () {
            resultEl.innerHTML = '<div class="error-alert">An error occurred. Please try again.</div>';
        });
});

// ---------- Reset ----------

document.getElementById('acme-reset-btn').addEventListener('click', function () {
    if (!confirm('This will permanently delete ALL ACME data:\n\n• Configuration\n• Account keys\n• Certificates\n\nThis cannot be undone. Continue?')) return;
    fetch('/api/v1/acme', {method: 'DELETE', credentials: 'same-origin'})
        .then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/ui/login';
                return;
            }
            if (!resp.ok) return resp.text().then(function (t) {
                showToast(t || 'Failed to reset ACME data.', 'error');
            });
            showToast('All ACME data has been reset.', 'success');
            acmeConfigExists = false;
            document.getElementById('acme-enabled').checked = false;
            document.getElementById('acme-enabled-label').textContent = 'Disabled';
            document.getElementById('acme-email').value = '';
            document.getElementById('acme-dns-provider').value = '';
            document.getElementById('acme-ca-dir-url').value = '';
            document.getElementById('acme-renewal-interval').value = '';
            document.getElementById('acme-credential-fields').innerHTML = '';
            document.getElementById('acme-config-result').innerHTML = '';

            // Reset the domains tab
            var dc = document.getElementById('acme-domains-input');
            dc.innerHTML = '';
            dc.classList.remove('mw-tag-input');
            acmeDomainsInitialized = false;

            lockAcmeForm(false);
        })
        .catch(function () {
            showToast('An error occurred while resetting ACME data.', 'error');
        });
});
