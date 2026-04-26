// ---------- Create Listener Wizard ----------

var lfSchemas = [];
var lfInterfaces = [];
var lfDefaultMwChain = null;
var lfDefaultTpSection = null;
var lfHostRoutes = [];  // [{id, el, mwChain, paths:[{id, el, mwChain}]}]
var lfDefaultPaths = []; // [{id, el, mwChain}]
var lfIdCounter = 0;
var lfCurrentStep = 1;
var lfEditMode = false;
var lfEditPort = null;
var lfEditOriginalConfig = null;  // stores the original config fetched for edit

// --- Wizard step navigation ---
function lfShowStep(step) {
    lfCurrentStep = step;
    for (var i = 1; i <= 3; i++) {
        var panel = document.getElementById('lf-panel-' + i);
        if (panel) panel.style.display = i === step ? '' : 'none';
    }
    document.querySelectorAll('#lf-step-bar .lf-step').forEach(function(btn) {
        var s = parseInt(btn.getAttribute('data-lf-step'));
        btn.classList.toggle('active', s === step);
        btn.classList.toggle('completed', s < step);
    });
    if (step === 3) lfBuildReview();
}

document.querySelectorAll('#lf-step-bar .lf-step').forEach(function(btn) {
    btn.addEventListener('click', function() {
        var step = parseInt(this.getAttribute('data-lf-step'));
        if (step === 3) {
            if (!lfValidateStep2()) return;
        }
        if (step >= 2) {
            if (!lfValidateStep1()) return;
        }
        lfShowStep(step);
    });
});

document.getElementById('lf-next-1').addEventListener('click', function() {
    if (lfValidateStep1()) lfShowStep(2);
});
document.getElementById('lf-back-2').addEventListener('click', function() { lfShowStep(1); });
document.getElementById('lf-next-2').addEventListener('click', function() {
    if (lfValidateStep2()) lfShowStep(3);
});
document.getElementById('lf-back-3').addEventListener('click', function() { lfShowStep(2); });

function lfValidateStep1() {
    var port = document.getElementById('lf-port').value;
    var iface = document.getElementById('lf-interface').value;
    if (!port || parseInt(port) <= 0 || parseInt(port) > 65535) {
        showToast('Please enter a valid port (1-65535).', 'error'); return false;
    }
    if (iface === '__unselected__') {
        showToast('Please select a network interface.', 'error'); return false;
    }
    return true;
}

function lfHasTerminatingMiddleware(mwChain) {
    if (!mwChain || !lfSchemas) return false;
    var items = mwChain.collect();
    return items.some(function(m) {
        var schema = lfSchemas.find(function(s) { return s.name === m.type; });
        return schema && schema.terminates;
    });
}

function lfValidateStep2() {
    var hasDefault = document.getElementById('lf-default-enable').checked;
    var hasRoutes = lfHostRoutes.length > 0;
    if (!hasDefault && !hasRoutes) {
        showToast('Configure at least a default route or one host-based route.', 'error');
        return false;
    }
    if (hasDefault) {
        var backend = document.getElementById('lf-default-backend').value.trim();
        if (!backend && !lfHasTerminatingMiddleware(lfDefaultMwChain)) {
            showToast('Default route requires a backend URL or a terminating middleware (e.g. Redirect).', 'error'); return false;
        }
    }
    for (var i = 0; i < lfHostRoutes.length; i++) {
        var r = lfHostRoutes[i];
        var host = r.el.querySelector('.lf-route-host').value.trim();
        var bk = r.el.querySelector('.lf-route-backend').value.trim();
        if (!host) { showToast('Route #' + (i+1) + ' requires a host.', 'error'); return false; }
        if (!bk && !lfHasTerminatingMiddleware(r.mwChain)) { showToast('Route "' + host + '" requires a backend URL or a terminating middleware (e.g. Redirect).', 'error'); return false; }
    }
    return true;
}

// --- TLS toggles ---
document.getElementById('lf-tls-enable').addEventListener('change', function() {
    document.getElementById('lf-tls-fields').style.display = this.checked ? '' : 'none';
});
document.getElementById('lf-tls-acme').addEventListener('change', function() {
    document.getElementById('lf-tls-cert-fields').style.display = this.checked ? 'none' : '';
});

// --- Default route toggle ---
document.getElementById('lf-default-enable').addEventListener('change', function() {
    document.getElementById('lf-default-route-fields').style.display = this.checked ? '' : 'none';
});

// --- Load interfaces ---
function lfLoadInterfaces() {
    fetch('/api/v1/system/interfaces', {credentials: 'same-origin'})
        .then(function(r) { return r.ok ? r.json() : []; })
        .then(function(ifaces) {
            lfInterfaces = ifaces || [];
            var sel = document.getElementById('lf-interface');
            sel.innerHTML = '<option value="__unselected__">Select interface…</option>';
            ifaces.forEach(function(i) {
                var opt = document.createElement('option');
                opt.value = i.ipv4 === '0.0.0.0' ? '' : i.name;
                var desc = i.name;
                if (i.ipv4 || i.ipv6) desc += ' (' + [i.ipv4, i.ipv6].filter(Boolean).join(' / ') + ')';
                opt.textContent = desc;
                sel.appendChild(opt);
            });
        });
}

// --- Load middleware schemas ---
function lfLoadSchemas() {
    return fetch('/api/v1/proxy/middlewares/schema', {credentials: 'same-origin'})
        .then(function(r) { return r.ok ? r.json() : []; })
        .then(function(schemas) {
            lfSchemas = schemas || [];
            // Initialize default route middleware chain
            if (!lfDefaultMwChain) {
                lfDefaultMwChain = new MwChainBuilder(document.getElementById('lf-default-mw'), lfSchemas);
            }
            if (!lfDefaultTpSection) {
                lfDefaultTpSection = lfCreateTrustedProxiesSection(document.getElementById('lf-default-tp'));
            }
        });
}

// --- Init wizard ---
function initListenerWizard() {
    lfLoadInterfaces();
    if (lfSchemas.length === 0) lfLoadSchemas();
}

// ========== Reusable Middleware Chain Builder ==========

function MwChainBuilder(containerEl, schemas) {
    var self = this;
    var _dragSrc = null;

    var dropZone = document.createElement('div');
    dropZone.className = 'mw-drop-zone';
    var emptyEl = document.createElement('div');
    emptyEl.className = 'mw-drop-zone-empty';
    emptyEl.textContent = 'No middleware configured';
    dropZone.appendChild(emptyEl);

    var addRow = document.createElement('div');
    addRow.style.cssText = 'margin-top:0.4rem;';
    var addSelect = document.createElement('select');
    addSelect.style.cssText = 'width:auto;min-width:180px;margin:0;padding:0.3rem 0.5rem;font-size:0.78rem;';
    lfBuildMwSelectOptions(addSelect, schemas);
    addRow.appendChild(addSelect);

    containerEl.appendChild(dropZone);
    containerEl.appendChild(addRow);

    addSelect.addEventListener('change', function() {
        if (!this.value) return;
        self.add(this.value);
        this.value = '';
    });

    // Drag-and-drop for reordering within chain
    dropZone.addEventListener('dragover', function(e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        dropZone.classList.add('drag-over');
        if (!_dragSrc) return;
        var afterEl = lfGetDragAfter(dropZone, e.clientY);
        if (afterEl) { dropZone.insertBefore(_dragSrc, afterEl); }
        else { dropZone.appendChild(_dragSrc); }
    });
    dropZone.addEventListener('dragleave', function(e) {
        if (!dropZone.contains(e.relatedTarget)) dropZone.classList.remove('drag-over');
    });
    dropZone.addEventListener('drop', function(e) {
        e.preventDefault();
        dropZone.classList.remove('drag-over');
        if (_dragSrc) { _dragSrc.classList.remove('dragging'); _dragSrc = null; renumber(); }
    });

    function renumber() {
        var items = dropZone.querySelectorAll('.mw-chain-item');
        items.forEach(function(item, i) {
            item.querySelector('.mw-chain-num').textContent = (i + 1) + '.';
        });
        emptyEl.style.display = items.length === 0 ? '' : 'none';
        // Update the badge in the parent details summary
        var details = containerEl.closest('.lf-mw-section');
        if (details) {
            var badge = details.querySelector('summary .lf-mw-badge');
            if (badge) badge.textContent = items.length > 0 ? items.length + ' middleware' + (items.length > 1 ? 's' : '') : '';
        }
    }

    this.add = function(type, existingOptions) {
        var schema = schemas.find(function(s) { return s.name === type; });
        if (!schema) return;

        var item = document.createElement('div');
        item.className = 'mw-chain-item';
        item.draggable = true;
        item.setAttribute('data-mw-type', type);

        var header = document.createElement('div');
        header.className = 'mw-chain-item-header';
        header.innerHTML = '<span class="mw-chain-num"></span><span class="mw-chain-name">' + type + '</span>';

        var cfgBtn = document.createElement('button');
        cfgBtn.type = 'button';
        cfgBtn.className = 'mw-chain-cfg-btn';
        cfgBtn.textContent = schema.fields.length > 0 ? '⚙' : '—';
        cfgBtn.disabled = schema.fields.length === 0;
        cfgBtn.title = schema.fields.length > 0 ? 'Configure' : 'No configurable options';
        header.appendChild(cfgBtn);

        var rmBtn = document.createElement('button');
        rmBtn.type = 'button';
        rmBtn.className = 'mw-chain-rm-btn';
        rmBtn.textContent = '✕';
        header.appendChild(rmBtn);
        item.appendChild(header);

        if (schema.fields.length > 0) {
            var opts = document.createElement('div');
            opts.className = 'mw-chain-opts';
            lfRenderMwOptions(opts, schema.fields, existingOptions || {});
            item.appendChild(opts);
            cfgBtn.addEventListener('click', function() {
                var isHidden = getComputedStyle(opts).display === 'none';
                opts.style.display = isHidden ? 'block' : 'none';
                cfgBtn.textContent = isHidden ? '▾' : '⚙';
            });
        }

        rmBtn.addEventListener('click', function() { item.remove(); renumber(); });

        item.addEventListener('dragstart', function(e) {
            _dragSrc = item;
            item.classList.add('dragging');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', '');
        });
        item.addEventListener('dragend', function() {
            item.classList.remove('dragging');
            _dragSrc = null;
            renumber();
        });

        emptyEl.style.display = 'none';
        dropZone.appendChild(item);
        renumber();
    };

    this.collect = function() {
        var chain = [];
        dropZone.querySelectorAll('.mw-chain-item').forEach(function(item) {
            var type = item.getAttribute('data-mw-type');
            var options = {};
            item.querySelectorAll('[data-option-key]').forEach(function(el) {
                var key = el.getAttribute('data-option-key');
                var otype = el.getAttribute('data-option-type');
                var val;
                if (otype === 'bool') { val = el.checked; }
                else if (otype === 'select') { val = el.value; }
                else if (otype === 'multiselect') {
                    val = [];
                    el.querySelectorAll('input:checked').forEach(function(cb) { val.push(cb.value); });
                    if (val.length === 0) return;
                } else if (otype === 'stringlist') {
                    val = lfCollectStringListValues(el);
                    if (val.length === 0) return;
                } else if (otype === 'map') {
                    val = lfCollectMapTagValues(el);
                    if (Object.keys(val).length === 0) return;
                } else if (otype === 'int') {
                    if (el.value === '') return;
                    val = parseInt(el.value, 10);
                } else if (otype === 'float') {
                    if (el.value === '') return;
                    val = parseFloat(el.value);
                } else {
                    val = el.value; if (!val) return;
                }
                lfSetNestedVal(options, key, val);
            });
            chain.push({type: type, options: Object.keys(options).length > 0 ? options : undefined});
        });
        return chain;
    };

    this.clear = function() {
        dropZone.querySelectorAll('.mw-chain-item').forEach(function(el) { el.remove(); });
        emptyEl.style.display = '';
    };

    this.destroy = function() {
        containerEl.innerHTML = '';
    };
}

// --- Shared MW helpers ---
function lfBuildMwSelectOptions(sel, schemas) {
    sel.innerHTML = '<option value="">+ Add middleware…</option>';
    (schemas || []).forEach(function(s) {
        var opt = document.createElement('option');
        opt.value = s.name;
        opt.textContent = s.name;
        opt.title = s.description;
        sel.appendChild(opt);
    });
}

function lfGetDragAfter(container, y) {
    var els = Array.from(container.querySelectorAll('.mw-chain-item:not(.dragging)'));
    var result = null;
    var closest = Number.POSITIVE_INFINITY;
    els.forEach(function(el) {
        var box = el.getBoundingClientRect();
        var offset = y - box.top - box.height / 2;
        if (offset < 0 && offset > -closest) { closest = -offset; result = el; }
    });
    return result;
}

function lfGetNestedVal(obj, key) {
    if (!obj) return undefined;
    var parts = key.split('.');
    var cur = obj;
    for (var i = 0; i < parts.length; i++) {
        if (cur === undefined || cur === null) return undefined;
        cur = cur[parts[i]];
    }
    return cur;
}

function lfSetNestedVal(obj, key, val) {
    var parts = key.split('.');
    var cur = obj;
    for (var i = 0; i < parts.length - 1; i++) {
        if (!cur[parts[i]]) cur[parts[i]] = {};
        cur = cur[parts[i]];
    }
    cur[parts[parts.length - 1]] = val;
}

function lfRenderMwOptions(container, fields, values) {
    var lastGroup = '';
    fields.forEach(function(f) {
        if (f.group && f.group !== lastGroup) {
            lastGroup = f.group;
            var gl = document.createElement('div');
            gl.className = 'mw-group-label';
            gl.textContent = f.group;
            container.appendChild(gl);
        }
        var val = lfGetNestedVal(values, f.key);
        if (val === undefined && f.default !== undefined) val = f.default;

        var wrap = document.createElement('div');

        if (f.type === 'bool') {
            var lbl = document.createElement('label');
            lbl.style.cssText = 'display:inline-flex;align-items:center;gap:0.3rem;font-size:0.78rem;cursor:pointer;margin-bottom:0.3rem;';
            lbl.innerHTML = '<input type="checkbox" data-option-key="' + f.key + '" data-option-type="bool"' + (val ? ' checked' : '') + '> ' + f.label;
            wrap.appendChild(lbl);
        } else if (f.type === 'select') {
            var lbl2 = document.createElement('label');
            lbl2.textContent = f.label + (f.required ? '' : ' (optional)');
            var sel = document.createElement('select');
            sel.setAttribute('data-option-key', f.key);
            sel.setAttribute('data-option-type', 'select');
            (f.choices || []).forEach(function(c) {
                var opt = document.createElement('option');
                opt.value = c; opt.textContent = c;
                if (val === c) opt.selected = true;
                sel.appendChild(opt);
            });
            wrap.appendChild(lbl2);
            wrap.appendChild(sel);
        } else if (f.type === 'multiselect') {
            var lbl3 = document.createElement('label');
            lbl3.textContent = f.label;
            wrap.appendChild(lbl3);
            var ms = document.createElement('div');
            ms.className = 'mw-multiselect';
            ms.setAttribute('data-option-key', f.key);
            ms.setAttribute('data-option-type', 'multiselect');
            var selArr = Array.isArray(val) ? val : [];
            (f.choices || []).forEach(function(c) {
                var cb = document.createElement('label');
                cb.innerHTML = '<input type="checkbox" value="' + c + '"' + (selArr.indexOf(c) >= 0 ? ' checked' : '') + '> ' + c;
                ms.appendChild(cb);
            });
            wrap.appendChild(ms);
        } else if (f.type === 'stringlist') {
            var lbl4 = document.createElement('label');
            lbl4.textContent = f.label + (f.required ? '' : ' (optional)');
            wrap.appendChild(lbl4);
            var sl = document.createElement('div');
            sl.className = 'mw-stringlist';
            sl.setAttribute('data-option-key', f.key);
            sl.setAttribute('data-option-type', 'stringlist');
            var arr = Array.isArray(val) ? val : [];
            lfInitTagInput(sl, arr);
            wrap.appendChild(sl);
        } else if (f.type === 'map') {
            var lbl5 = document.createElement('label');
            lbl5.textContent = f.label + ' (key:value)' + (f.required ? '' : ' (optional)');
            wrap.appendChild(lbl5);
            var mp = document.createElement('div');
            mp.setAttribute('data-option-key', f.key);
            mp.setAttribute('data-option-type', 'map');
            var initMap = {};
            if (val && typeof val === 'object' && !Array.isArray(val)) {
                initMap = val;
            }
            lfInitMapTagInput(mp, initMap);
            wrap.appendChild(mp);
        } else {
            var lbl6 = document.createElement('label');
            lbl6.textContent = f.label + (f.required ? '' : ' (optional)');
            var inp = document.createElement('input');
            inp.type = (f.type === 'int' || f.type === 'float') ? 'number' : 'text';
            if (f.type === 'float') inp.step = 'any';
            inp.setAttribute('data-option-key', f.key);
            inp.setAttribute('data-option-type', f.type);
            inp.placeholder = f.placeholder || '';
            if (val !== undefined && val !== null) inp.value = val;
            if (f.required) inp.required = true;
            wrap.appendChild(lbl6);
            wrap.appendChild(inp);
        }

        if (f.help_text) {
            var help = document.createElement('p');
            help.className = 'mw-help';
            help.textContent = f.help_text;
            wrap.appendChild(help);
        }
        container.appendChild(wrap);
    });
}

function lfSplitTagInput(raw) {
    return raw.split(/[\t,\n\r]+/).map(function(v) { return v.trim(); }).filter(Boolean);
}

function lfPushUnique(arr, val) {
    if (arr.indexOf(val) === -1) arr.push(val);
}

function lfAddTag(container, value) {
    var v = (value || '').trim();
    if (!v) return;
    var exists = false;
    container.querySelectorAll('.mw-tag').forEach(function(tag) {
        if (tag.getAttribute('data-value') === v) exists = true;
    });
    if (exists) return;

    var tag = document.createElement('span');
    tag.className = 'mw-tag';
    tag.setAttribute('data-value', v);

    var label = document.createElement('span');
    label.textContent = v;
    tag.appendChild(label);

    var btn = document.createElement('button');
    btn.type = 'button';
    btn.textContent = 'x';
    btn.setAttribute('aria-label', 'Remove');
    btn.addEventListener('click', function() { tag.remove(); });
    tag.appendChild(btn);

    var list = container.querySelector('.mw-tag-list');
    if (list) list.appendChild(tag);
}

function lfInitTagInput(container, values) {
    container.classList.add('mw-tag-input');

    var tagList = document.createElement('div');
    tagList.className = 'mw-tag-list';
    var input = document.createElement('input');
    input.type = 'text';
    input.className = 'mw-tag-editor';
    input.placeholder = 'Type and press Enter, comma, or Tab';

    container.appendChild(tagList);
    container.appendChild(input);

    (values || []).forEach(function(v) { lfAddTag(container, v); });

    input.addEventListener('keydown', function(e) {
        if ((e.key === ',' || e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey) {
            e.preventDefault();
            var raw = input.value;
            lfSplitTagInput(raw).forEach(function(v) { lfAddTag(container, v); });
            input.value = '';
            return;
        }
        if (e.key === 'Backspace' && input.value === '') {
            var tags = container.querySelectorAll('.mw-tag');
            var last = tags[tags.length - 1];
            if (last) last.remove();
        }
    });

    input.addEventListener('blur', function() {
        var raw = input.value;
        lfSplitTagInput(raw).forEach(function(v) { lfAddTag(container, v); });
        input.value = '';
    });

    input.addEventListener('paste', function(e) {
        var text = (e.clipboardData || window.clipboardData).getData('text');
        if (!text) return;
        if (/[\t,\n\r]/.test(text)) {
            e.preventDefault();
            lfSplitTagInput(text).forEach(function(v) { lfAddTag(container, v); });
        }
    });
}

function lfCollectStringListValues(container) {
    var values = [];
    var tags = container.querySelectorAll('.mw-tag');
    if (tags.length > 0) {
        tags.forEach(function(tag) {
            var v = tag.getAttribute('data-value');
            if (v) lfPushUnique(values, v);
        });
    }

    var editor = container.querySelector('.mw-tag-editor');
    if (editor && editor.value.trim()) {
        lfSplitTagInput(editor.value).forEach(function(v) { lfPushUnique(values, v); });
    }

    if (tags.length > 0 || editor) return values;

    container.querySelectorAll('.mw-stringlist-row input').forEach(function(inp) {
        var v = inp.value.trim();
        if (v) lfPushUnique(values, v);
    });
    return values;
}

function lfAddMapTag(container, key, value) {
    var display = key + ':' + value;
    // check for duplicate
    var existing = container.querySelectorAll('.mw-tag');
    for (var i = 0; i < existing.length; i++) {
        if (existing[i].getAttribute('data-key') === key) {
            existing[i].setAttribute('data-value', value);
            existing[i].querySelector('span').textContent = display;
            return;
        }
    }
    var tagList = container.querySelector('.mw-tag-list');
    var tag = document.createElement('span');
    tag.className = 'mw-tag';
    tag.setAttribute('data-key', key);
    tag.setAttribute('data-value', value);
    tag.innerHTML = '<span>' + display.replace(/</g, '&lt;') + '</span><button type="button">✕</button>';
    tag.querySelector('button').addEventListener('click', function() { tag.remove(); });
    tagList.appendChild(tag);
}

function lfParseMapEntry(text) {
    var idx = text.indexOf(':');
    if (idx < 1) return null;
    var k = text.substring(0, idx).trim();
    var v = text.substring(idx + 1).trim();
    if (!k) return null;
    return { key: k, value: v };
}

function lfInitMapTagInput(container, initMap) {
    container.className = 'mw-tag-input';
    var tagList = document.createElement('span');
    tagList.className = 'mw-tag-list';
    container.appendChild(tagList);

    var input = document.createElement('input');
    input.type = 'text';
    input.className = 'mw-tag-editor';
    input.placeholder = 'key:value then Enter';
    container.appendChild(input);

    Object.keys(initMap).forEach(function(k) {
        lfAddMapTag(container, k, initMap[k]);
    });

    input.addEventListener('keydown', function(e) {
        if (e.key === 'Backspace' && !input.value) {
            var tags = tagList.querySelectorAll('.mw-tag');
            if (tags.length) tags[tags.length - 1].remove();
            return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
            var text = input.value.trim();
            if (!text) { if (e.key !== 'Tab') e.preventDefault(); return; }
            e.preventDefault();
            var entry = lfParseMapEntry(text);
            if (entry) lfAddMapTag(container, entry.key, entry.value);
            input.value = '';
        }
    });

    input.addEventListener('blur', function() {
        var text = input.value.trim();
        if (!text) return;
        var entry = lfParseMapEntry(text);
        if (entry) { lfAddMapTag(container, entry.key, entry.value); input.value = ''; }
    });

    input.addEventListener('paste', function(e) {
        var text = (e.clipboardData || window.clipboardData).getData('text');
        if (!text) return;
        if (/[\t\n\r]/.test(text)) {
            e.preventDefault();
            text.split(/[\t\n\r]+/).map(function(v) { return v.trim(); }).filter(Boolean).forEach(function(v) {
                var entry = lfParseMapEntry(v);
                if (entry) lfAddMapTag(container, entry.key, entry.value);
            });
        }
    });
}

function lfCollectMapTagValues(container) {
    var result = {};
    container.querySelectorAll('.mw-tag').forEach(function(tag) {
        var k = tag.getAttribute('data-key');
        var v = tag.getAttribute('data-value');
        if (k) result[k] = v || '';
    });
    // also grab anything left in the editor
    var editor = container.querySelector('.mw-tag-editor');
    if (editor && editor.value.trim()) {
        var entry = lfParseMapEntry(editor.value.trim());
        if (entry) result[entry.key] = entry.value;
    }
    return result;
}

function lfAddStringListRow(container, val) {
    var row = document.createElement('div');
    row.className = 'mw-stringlist-row';
    row.innerHTML = '<input type="text" value="' + (val || '').replace(/"/g, '&quot;') + '" style="margin:0;font-size:0.78rem;padding:0.25rem 0.4rem;">' +
        '<button type="button">✕</button>';
    row.querySelector('button').addEventListener('click', function() { row.remove(); });
    container.appendChild(row);
}

function lfAddMapRow(container, key, val) {
    var row = document.createElement('div');
    row.className = 'mw-map-row';
    row.innerHTML = '<input type="text" placeholder="Key" value="' + (key || '').replace(/"/g, '&quot;') + '" style="margin:0;font-size:0.78rem;padding:0.25rem 0.4rem;">' +
        '<input type="text" placeholder="Value" value="' + (val || '').replace(/"/g, '&quot;') + '" style="margin:0;font-size:0.78rem;padding:0.25rem 0.4rem;">' +
        '<button type="button">✕</button>';
    row.querySelector('button').addEventListener('click', function() { row.remove(); });
    container.appendChild(row);
}

// ========== Path Rules ==========

function lfUpdatePathBadge(pathsContainer) {
    var details = pathsContainer.closest('.lf-mw-section');
    if (details) {
        var badge = details.querySelector('summary .lf-mw-badge');
        var count = pathsContainer.querySelectorAll('.lf-path-card').length;
        if (badge) badge.textContent = count > 0 ? count + ' rule' + (count > 1 ? 's' : '') : '';
    }
}

function lfCreatePathCard(pathsContainer, pathsList) {
    lfIdCounter++;
    var pathId = 'lf-path-' + lfIdCounter;
    var card = document.createElement('div');
    card.className = 'lf-path-card';
    card.id = pathId;

    card.innerHTML =
        '<div class="lf-path-header">' +
        '<span>Path Rule</span>' +
        '<button type="button" class="lf-remove-btn">✕ Remove</button>' +
        '</div>' +
        '<div class="lf-grid-2">' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Pattern' +
        '<input type="text" class="lf-path-pattern" placeholder="e.g. /api/*" style="margin-bottom:0.4rem;font-size:0.8rem;padding:0.3rem 0.5rem;"></label>' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Backend <small>(optional, overrides route)</small>' +
        '<input type="text" class="lf-path-backend" placeholder="e.g. http://192.168.1.50:3000" style="margin-bottom:0.4rem;font-size:0.8rem;padding:0.3rem 0.5rem;"></label>' +
        '</div>' +
        '<div style="display:flex;gap:1.5rem;margin-bottom:0.5rem;">' +
        '<label style="display:inline-flex;align-items:center;gap:0.3rem;font-size:0.78rem;cursor:pointer;margin:0;">' +
        '<input type="checkbox" class="lf-path-strip-prefix"> Strip prefix</label>' +
        '<label style="display:inline-flex;align-items:center;gap:0.3rem;font-size:0.78rem;cursor:pointer;margin:0;">' +
        '<input type="checkbox" class="lf-path-drop-query"> Drop query</label>' +
        '<label style="display:inline-flex;align-items:center;gap:0.3rem;font-size:0.78rem;cursor:pointer;margin:0;">' +
        '<input type="checkbox" class="lf-path-replace-host"> Replace Host header</label>' +
        '</div>' +
        '<details class="lf-mw-section">' +
        '<summary>Path Middleware Chain <span class="lf-mw-badge lf-path-mw-badge"></span></summary>' +
        '<fieldset style="margin:0.5rem 0 0.5rem;padding:0;border:none;">' +
        '<label style="display:inline-flex;align-items:center;gap:0.4rem;font-size:0.78rem;cursor:pointer;">' +
        '<input type="checkbox" class="lf-path-disable-defaults" role="switch"> Disable default middlewares' +
        '</label>' +
        '<p style="font-size:0.72rem;color:var(--pico-muted-color);margin:0.15rem 0 0;">When unchecked, RequestId, RequestLog and Metrics are automatically prepended.</p>' +
        '</fieldset>' +
        '<div class="lf-path-mw-container"></div>' +
        '</details>';

    var mwContainer = card.querySelector('.lf-path-mw-container');
    var mwChain = new MwChainBuilder(mwContainer, lfSchemas);

    // Trusted proxies section for this path
    var tpContainer = document.createElement('div');
    tpContainer.className = 'lf-path-tp-container';
    var pathMwDetails = card.querySelector('.lf-mw-section');
    pathMwDetails.parentNode.insertBefore(tpContainer, pathMwDetails);
    var tpSection = lfCreateTrustedProxiesSection(tpContainer);

    card.querySelector('.lf-remove-btn').addEventListener('click', function() {
        for (var i = 0; i < pathsList.length; i++) {
            if (pathsList[i].id === pathId) { pathsList.splice(i, 1); break; }
        }
        card.remove();
        lfUpdatePathBadge(pathsContainer);
    });

    pathsContainer.appendChild(card);
    var pathObj = { id: pathId, el: card, mwChain: mwChain, tpSection: tpSection };
    pathsList.push(pathObj);
    lfUpdatePathBadge(pathsContainer);
    return pathObj;
}

// Default route paths
document.getElementById('lf-default-add-path').addEventListener('click', function() {
    lfCreatePathCard(document.getElementById('lf-default-paths'), lfDefaultPaths);
});

// ========== Host-Based Routes ==========

document.getElementById('lf-add-host-route').addEventListener('click', function() {
    lfCreateHostRouteCard();
});

function lfCreateHostRouteCard() {
    lfIdCounter++;
    var routeId = 'lf-route-' + lfIdCounter;
    var container = document.getElementById('lf-host-routes');

    var card = document.createElement('div');
    card.className = 'lf-route-card';
    card.id = routeId;

    var routeNum = lfHostRoutes.length + 1;
    card.innerHTML =
        '<div class="lf-route-card-header">' +
        '<h5>Route #' + routeNum + '</h5>' +
        '<button type="button" class="lf-remove-btn">✕ Remove Route</button>' +
        '</div>' +
        '<div class="lf-grid-2">' +
        '<label style="font-size:0.85rem;margin-bottom:0.25rem;">Host' +
        '<input type="text" class="lf-route-host" placeholder="e.g. jellyfin.home.com" style="margin-bottom:0.5rem;"></label>' +
        '<label style="font-size:0.85rem;margin-bottom:0.25rem;">Backend URL' +
        '<input type="text" class="lf-route-backend" placeholder="e.g. http://192.168.1.100:8096" style="margin-bottom:0.5rem;"></label>' +
        '</div>' +
        '<label style="display:inline-flex;align-items:center;gap:0.3rem;font-size:0.78rem;cursor:pointer;margin:0 0 0.5rem;">' +
        '<input type="checkbox" class="lf-route-replace-host"> Replace Host header with backend host</label>' +
        '<details class="lf-mw-section" open>' +
        '<summary>Middleware Chain <span class="lf-mw-badge lf-route-mw-badge"></span></summary>' +
        '<fieldset style="margin:0.5rem 0 0.5rem;padding:0;border:none;">' +
        '<label style="display:inline-flex;align-items:center;gap:0.4rem;font-size:0.8rem;cursor:pointer;">' +
        '<input type="checkbox" class="lf-route-disable-defaults" role="switch"> Disable default middlewares' +
        '</label>' +
        '<p style="font-size:0.72rem;color:var(--pico-muted-color);margin:0.15rem 0 0;">When unchecked, RequestId, RequestLog and Metrics are automatically prepended.</p>' +
        '</fieldset>' +
        '<div class="lf-route-mw-container"></div>' +
        '</details>' +
        '<details class="lf-mw-section" style="margin-top:0.75rem;">' +
        '<summary>Path Rules <span class="lf-mw-badge lf-route-paths-badge"></span></summary>' +
        '<div class="lf-route-paths-container" style="margin-top:0.5rem;"></div>' +
        '<button type="button" class="lf-add-btn lf-route-add-path">+ Add Path Rule</button>' +
        '</details>';

    var mwContainer = card.querySelector('.lf-route-mw-container');
    var mwChain = new MwChainBuilder(mwContainer, lfSchemas);

    // Trusted proxies section for this route
    var tpContainer = document.createElement('div');
    tpContainer.className = 'lf-route-tp-container';
    // Insert before the middleware details
    var mwDetails = card.querySelector('.lf-mw-section');
    mwDetails.parentNode.insertBefore(tpContainer, mwDetails);
    var tpSection = lfCreateTrustedProxiesSection(tpContainer);

    var routeObj = { id: routeId, el: card, mwChain: mwChain, tpSection: tpSection, paths: [] };

    card.querySelector('.lf-remove-btn').addEventListener('click', function() {
        for (var i = 0; i < lfHostRoutes.length; i++) {
            if (lfHostRoutes[i].id === routeId) { lfHostRoutes.splice(i, 1); break; }
        }
        card.remove();
        lfRenumberRoutes();
    });

    var pathsContainer = card.querySelector('.lf-route-paths-container');
    card.querySelector('.lf-route-add-path').addEventListener('click', function() {
        lfCreatePathCard(pathsContainer, routeObj.paths);
    });

    container.appendChild(card);
    lfHostRoutes.push(routeObj);
    return routeObj;
}

function lfRenumberRoutes() {
    lfHostRoutes.forEach(function(r, i) {
        var h5 = r.el.querySelector('.lf-route-card-header h5');
        if (h5) h5.textContent = 'Route #' + (i + 1);
    });
}

// ========== Trusted Proxies UI ==========

function lfCreateTrustedProxiesSection(parentEl) {
    var details = document.createElement('details');
    details.className = 'lf-mw-section';
    details.style.marginTop = '0.5rem';
    details.innerHTML =
        '<summary>Trusted Proxies <span class="lf-mw-badge lf-tp-badge"></span></summary>' +
        '<div class="lf-tp-fields" style="margin-top:0.5rem;">' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Preset' +
        '<select class="lf-tp-preset" style="margin-bottom:0.4rem;font-size:0.8rem;padding:0.3rem 0.5rem;">' +
        '<option value="">None (manual ranges only)</option>' +
        '<option value="cloudflare">Cloudflare</option>' +
        '</select></label>' +
        '<p class="mw-help" style="margin-top:-0.2rem;">Built-in provider whose IP ranges are fetched live and refreshed automatically.</p>' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Header <small>(optional)</small>' +
        '<input type="text" class="lf-tp-header" placeholder="e.g. CF-Connecting-IP, X-Real-IP" ' +
        'style="margin-bottom:0.4rem;font-size:0.8rem;padding:0.3rem 0.5rem;"></label>' +
        '<p class="mw-help" style="margin-top:-0.2rem;">HTTP header to read client IP from. Leave empty for default (X-Forwarded-For, or preset default).</p>' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Refresh Interval <small>(optional)</small>' +
        '<input type="text" class="lf-tp-refresh" placeholder="e.g. 24h, 12h" ' +
        'style="margin-bottom:0.4rem;font-size:0.8rem;padding:0.3rem 0.5rem;"></label>' +
        '<p class="mw-help" style="margin-top:-0.2rem;">How often to re-fetch preset CIDRs. Ignored for static ranges.</p>' +
        '<label style="font-size:0.8rem;margin-bottom:0.25rem;">Additional Ranges <small>(IP/CIDR, optional)</small></label>' +
        '<div class="lf-tp-ranges"></div>' +
        '<p class="mw-help">Static IP addresses or CIDR blocks to trust, merged with preset if set.</p>' +
        '</div>';

    var rangesEl = details.querySelector('.lf-tp-ranges');
    rangesEl.setAttribute('data-option-key', '__tp_ranges');
    rangesEl.setAttribute('data-option-type', 'stringlist');
    lfInitTagInput(rangesEl, []);

    // Update badge when preset changes
    var presetSel = details.querySelector('.lf-tp-preset');
    var badge = details.querySelector('.lf-tp-badge');
    presetSel.addEventListener('change', function() {
        lfUpdateTpBadge(details);
    });

    parentEl.appendChild(details);
    return details;
}

function lfUpdateTpBadge(tpSection) {
    var badge = tpSection.querySelector('.lf-tp-badge');
    if (!badge) return;
    var preset = tpSection.querySelector('.lf-tp-preset').value;
    var ranges = lfCollectStringListValues(tpSection.querySelector('.lf-tp-ranges'));
    if (preset || ranges.length > 0) {
        badge.textContent = preset ? preset : ranges.length + ' range(s)';
    } else {
        badge.textContent = '';
    }
}

function lfCollectTrustedProxies(tpSection) {
    if (!tpSection) return undefined;
    var preset = tpSection.querySelector('.lf-tp-preset').value;
    var header = tpSection.querySelector('.lf-tp-header').value.trim();
    var refresh = tpSection.querySelector('.lf-tp-refresh').value.trim();
    var ranges = lfCollectStringListValues(tpSection.querySelector('.lf-tp-ranges'));
    if (!preset && ranges.length === 0) return undefined;
    var tp = {};
    if (preset) tp.preset = preset;
    if (ranges.length > 0) tp.ranges = ranges;
    if (header) tp.header = header;
    if (refresh) tp.refresh_interval = refresh;
    return tp;
}

function lfPopulateTrustedProxies(tpSection, data) {
    if (!tpSection || !data) return;
    if (data.preset) tpSection.querySelector('.lf-tp-preset').value = data.preset;
    if (data.header) tpSection.querySelector('.lf-tp-header').value = data.header;
    if (data.refresh_interval) tpSection.querySelector('.lf-tp-refresh').value = data.refresh_interval;
    if (data.ranges && data.ranges.length > 0) {
        data.ranges.forEach(function(r) { lfAddTag(tpSection.querySelector('.lf-tp-ranges'), r); });
    }
    lfUpdateTpBadge(tpSection);
}

// ========== Collect paths from a path list ==========
function lfCollectPaths(pathsList) {
    var result = [];
    pathsList.forEach(function(p) {
        var pattern = p.el.querySelector('.lf-path-pattern').value.trim();
        if (!pattern) return;
        var entry = { pattern: pattern };
        var backend = p.el.querySelector('.lf-path-backend').value.trim();
        if (backend) {
            entry.backend = { address: backend };
            if (p.el.querySelector('.lf-path-replace-host').checked) entry.backend.replace_host_header = true;
        }
        if (p.el.querySelector('.lf-path-strip-prefix').checked) entry.strip_prefix = true;
        if (p.el.querySelector('.lf-path-drop-query').checked) entry.drop_query = true;
        if (p.el.querySelector('.lf-path-disable-defaults') && p.el.querySelector('.lf-path-disable-defaults').checked) {
            entry.disable_default_middlewares = true;
        }
        var mws = p.mwChain.collect();
        if (mws.length > 0) entry.middlewares = mws;
        var tp = lfCollectTrustedProxies(p.tpSection);
        if (tp) entry.trusted_proxies = tp;
        result.push(entry);
    });
    return result;
}

// ========== Build full request payload ==========
function lfCollectPayload() {
    var payload = {
        port: parseInt(document.getElementById('lf-port').value, 10),
        interface: document.getElementById('lf-interface').value,
        bind: parseInt(document.getElementById('lf-bind').value, 10)
    };

    // TLS
    if (document.getElementById('lf-tls-enable').checked) {
        payload.tls = {
            use_acme: document.getElementById('lf-tls-acme').checked,
            cert: document.getElementById('lf-tls-cert').value.trim(),
            key: document.getElementById('lf-tls-key').value.trim()
        };
    }

    // Timeouts
    var rt = document.getElementById('lf-timeout-read').value.trim();
    var rht = document.getElementById('lf-timeout-read-header').value.trim();
    var wt = document.getElementById('lf-timeout-write').value.trim();
    var it = document.getElementById('lf-timeout-idle').value.trim();
    if (rt) payload.read_timeout = rt;
    if (rht) payload.read_header_timeout = rht;
    if (wt) payload.write_timeout = wt;
    if (it) payload.idle_timeout = it;

    // Disable HTTP/2
    if (document.getElementById('lf-disable-http2').checked) {
        payload.disable_http2 = true;
    }

    // Default route
    if (document.getElementById('lf-default-enable').checked) {
        var defTarget = {
            backend: {
                address: document.getElementById('lf-default-backend').value.trim(),
                replace_host_header: document.getElementById('lf-default-replace-host').checked
            }
        };
        if (document.getElementById('lf-default-disable-defaults').checked) {
            defTarget.disable_default_middlewares = true;
        }
        var defMws = lfDefaultMwChain ? lfDefaultMwChain.collect() : [];
        if (defMws.length > 0) defTarget.middlewares = defMws;
        var defTp = lfCollectTrustedProxies(lfDefaultTpSection);
        if (defTp) defTarget.trusted_proxies = defTp;
        var defPaths = lfCollectPaths(lfDefaultPaths);
        if (defPaths.length > 0) defTarget.paths = defPaths;
        payload.default = defTarget;
    }

    // Host routes
    if (lfHostRoutes.length > 0) {
        payload.routes = [];
        lfHostRoutes.forEach(function(r) {
            var target = {
                backend: {
                    address: r.el.querySelector('.lf-route-backend').value.trim(),
                    replace_host_header: r.el.querySelector('.lf-route-replace-host').checked
                }
            };
            if (r.el.querySelector('.lf-route-disable-defaults') && r.el.querySelector('.lf-route-disable-defaults').checked) {
                target.disable_default_middlewares = true;
            }
            var mws = r.mwChain.collect();
            if (mws.length > 0) target.middlewares = mws;
            var routeTp = lfCollectTrustedProxies(r.tpSection);
            if (routeTp) target.trusted_proxies = routeTp;
            var paths = lfCollectPaths(r.paths);
            if (paths.length > 0) target.paths = paths;
            payload.routes.push({
                host: r.el.querySelector('.lf-route-host').value.trim(),
                target: target
            });
        });
    }

    return payload;
}

// ========== Review step ==========

var lfReviewMode = 'yaml';    // 'yaml' | 'edit' | 'json'
var lfReviewEdited = false;   // true if user touched the textarea
var lfReviewLastEditor = null;  // 'yaml' | 'json' — tracks which editor was last used

// --- JSON-key ↔ YAML-key mapping (matches config struct yaml tags) ---
var lfYamlKeyMap = {
    'use_acme': 'use-acme',
    'read_timeout': 'read-timeout',
    'read_header_timeout': 'read-header-timeout',
    'write_timeout': 'write-timeout',
    'idle_timeout': 'idle-timeout',
    'drop_query': 'drop-query',
    'strip_prefix': 'strip-prefix',
    'disable_http2': 'disable-http2',
    'disable_default_middlewares': 'disable-default-middlewares',
    'replace_host_header': 'replace-host-header',
    'trusted_proxies': 'trusted-proxies',
    'refresh_interval': 'refresh-interval'
};
var lfJsonKeyMap = {};
Object.keys(lfYamlKeyMap).forEach(function(k) { lfJsonKeyMap[lfYamlKeyMap[k]] = k; });

function lfRenameKeys(obj, map) {
    if (Array.isArray(obj)) return obj.map(function(v) { return lfRenameKeys(v, map); });
    if (obj && typeof obj === 'object') {
        var out = {};
        Object.keys(obj).forEach(function(k) {
            out[map[k] || k] = lfRenameKeys(obj[k], map);
        });
        return out;
    }
    return obj;
}

function lfPayloadToYaml(payload) {
    var yamlObj = lfRenameKeys(payload, lfYamlKeyMap);
    return jsyaml.dump(yamlObj, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false });
}

function lfYamlToPayload(yamlText) {
    var parsed = jsyaml.load(yamlText);
    return lfRenameKeys(parsed, lfJsonKeyMap);
}

// --- Review tab switching ---
document.querySelectorAll('.lf-review-tab').forEach(function(btn) {
    btn.addEventListener('click', function() {
        var tab = this.getAttribute('data-review-tab');
        lfReviewMode = tab;
        document.querySelectorAll('.lf-review-tab').forEach(function(b) { b.classList.remove('active'); });
        this.classList.add('active');
        lfRenderReview();
    });
});

function lfBuildReview() {
    lfReviewEdited = false;
    lfReviewLastEditor = null;
    lfRenderReview();
}

function lfRenderReview() {
    var formPayload = lfCollectPayload();
    var preEl = document.getElementById('lf-review-json');
    var yamlEditorEl = document.getElementById('lf-review-editor');
    var jsonEditorEl = document.getElementById('lf-review-editor-json');
    var hintEl = document.getElementById('lf-review-editor-hint');
    var errEl = document.getElementById('lf-review-editor-error');
    errEl.style.display = 'none';

    // Determine the "current" payload: if user edited, parse from the last-edited textarea
    var currentPayload = formPayload;
    if (lfReviewEdited && lfReviewLastEditor) {
        try {
            if (lfReviewLastEditor === 'json') {
                currentPayload = JSON.parse(jsonEditorEl.value);
            } else {
                currentPayload = lfYamlToPayload(yamlEditorEl.value);
            }
        } catch (e) { /* keep formPayload on parse error */ }
    }

    // Hide all, then show the right one
    preEl.style.display = 'none';
    yamlEditorEl.style.display = 'none';
    jsonEditorEl.style.display = 'none';
    hintEl.style.display = 'none';

    if (lfReviewMode === 'yaml') {
        preEl.style.display = '';
        preEl.textContent = lfPayloadToYaml(currentPayload);
    } else if (lfReviewMode === 'json') {
        preEl.style.display = '';
        preEl.textContent = JSON.stringify(currentPayload, null, 2);
    } else if (lfReviewMode === 'edit-yaml') {
        yamlEditorEl.style.display = '';
        hintEl.style.display = '';
        if (!lfReviewEdited || lfReviewLastEditor !== 'yaml') {
            yamlEditorEl.value = lfPayloadToYaml(currentPayload);
            lfReviewLastEditor = 'yaml';
        }
    } else if (lfReviewMode === 'edit-json') {
        jsonEditorEl.style.display = '';
        hintEl.style.display = '';
        if (!lfReviewEdited || lfReviewLastEditor !== 'json') {
            jsonEditorEl.value = JSON.stringify(currentPayload, null, 2);
            lfReviewLastEditor = 'json';
        }
    }

    // Show edit impact banner
    var impactEl = document.getElementById('lf-edit-impact');
    if (!lfEditMode || !lfEditOriginalConfig) {
        impactEl.style.display = 'none';
        return;
    }
    lfShowImpactBanner(impactEl, lfServerLevelChanged(lfEditOriginalConfig, currentPayload));
}

function lfShowImpactBanner(el, willRebuild) {
    el.style.display = 'block';
    el.style.color = '#fff';
    if (willRebuild) {
        el.style.background = 'rgba(231,76,60,0.12)';
        el.style.borderLeft = '3px solid #e74c3c';
        el.innerHTML = '<strong>\u26A0 Full rebuild required</strong><br>' +
            'Server-level settings changed (TLS, timeouts, bind, or interface). ' +
            'The proxy will be stopped, rebuilt, and restarted. ' +
            'Active connections will be dropped and middleware state will be reset.';
    } else {
        el.style.background = 'rgba(46,204,113,0.1)';
        el.style.borderLeft = '3px solid #2ecc71';
        el.innerHTML = '<strong>\u2713 Hot-swap (zero downtime)</strong><br>' +
            'Only routes and middleware changed. The handler will be swapped live \u2014 ' +
            'active connections are preserved.';
    }
}

// Mirror of serverLevelChanged() from the Go backend
function lfServerLevelChanged(old, neu) {
    if ((old.bind || 3) !== (neu.bind || 3)) return true;
    if ((old.interface || '') !== (neu.interface || '')) return true;
    if (!!old.disable_http2 !== !!neu.disable_http2) return true;
    if ((old.read_timeout || '') !== (neu.read_timeout || '')) return true;
    if ((old.read_header_timeout || '') !== (neu.read_header_timeout || '')) return true;
    if ((old.write_timeout || '') !== (neu.write_timeout || '')) return true;
    if ((old.idle_timeout || '') !== (neu.idle_timeout || '')) return true;
    var oldTLS = old.tls || null;
    var newTLS = neu.tls || null;
    if (!!oldTLS !== !!newTLS) return true;
    if (oldTLS && newTLS) {
        if (oldTLS.use_acme !== newTLS.use_acme) return true;
        if ((oldTLS.cert || '') !== (newTLS.cert || '')) return true;
        if ((oldTLS.key || '') !== (newTLS.key || '')) return true;
    }
    return false;
}

// Live update on YAML editor input
document.getElementById('lf-review-editor').addEventListener('input', function() {
    lfReviewEdited = true;
    lfReviewLastEditor = 'yaml';
    if (lfEditMode && lfEditOriginalConfig) {
        try {
            var p = lfYamlToPayload(this.value);
            lfShowImpactBanner(document.getElementById('lf-edit-impact'), lfServerLevelChanged(lfEditOriginalConfig, p));
        } catch (e) { /* not valid yet */ }
    }
});

// Live update on JSON editor input
document.getElementById('lf-review-editor-json').addEventListener('input', function() {
    lfReviewEdited = true;
    lfReviewLastEditor = 'json';
    if (lfEditMode && lfEditOriginalConfig) {
        try {
            var p = JSON.parse(this.value);
            lfShowImpactBanner(document.getElementById('lf-edit-impact'), lfServerLevelChanged(lfEditOriginalConfig, p));
        } catch (e) { /* not valid yet */ }
    }
});

// ========== Form submission ==========
document.getElementById('create-listener-form').addEventListener('submit', function(e) {
    e.preventDefault();
    var resultEl = document.getElementById('lf-result');
    resultEl.innerHTML = '';

    var payload;
    var errEl = document.getElementById('lf-review-editor-error');
    errEl.style.display = 'none';

    // If the user edited a textarea, parse it and use that as the payload
    if (lfReviewEdited) {
        try {
            if (lfReviewLastEditor === 'json') {
                payload = JSON.parse(document.getElementById('lf-review-editor-json').value);
            } else {
                payload = lfYamlToPayload(document.getElementById('lf-review-editor').value);
            }
            if (!payload || typeof payload !== 'object') throw new Error('Expected a mapping');
            // Ensure port and bind are numbers
            if (payload.port) payload.port = parseInt(payload.port, 10);
            if (payload.bind) payload.bind = parseInt(payload.bind, 10);
        } catch (ex) {
            errEl.textContent = 'Parse error: ' + ex.message;
            errEl.style.display = '';
            return;
        }
    } else {
        if (!lfValidateStep1() || !lfValidateStep2()) return;
        payload = lfCollectPayload();
    }
    var submitBtn = document.getElementById('lf-submit');
    submitBtn.disabled = true;

    var isEdit = lfEditMode && lfEditPort;
    var url = isEdit ? '/api/v1/proxy/routes/' + lfEditPort : '/api/v1/proxy/routes/http';
    var method = isEdit ? 'PUT' : 'POST';
    submitBtn.textContent = isEdit ? 'Saving…' : 'Creating…';

    fetch(url, {
        method: method,
        credentials: 'same-origin',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
    })
    .then(function(resp) {
        if (resp.status === 401) { window.location.href = '/ui/login'; return null; }
        if (!resp.ok) return resp.text().then(function(t) {
            resultEl.innerHTML = '<div class="error-alert">' + (t || (isEdit ? 'Failed to update proxy.' : 'Failed to create listener.')) + '</div>';
            return null;
        });
        return resp.json();
    })
    .then(function(result) {
        if (result === null) return;
        showToast(isEdit
            ? 'Proxy on port ' + payload.port + ' updated successfully.'
            : 'HTTP proxy server on port ' + payload.port + ' created successfully.', 'success');
        lfResetForm();
        lastRouteFingerprint = '';
        loadProxyRoutes();
        document.getElementById('create-proxy-panel').style.display = 'none';
        document.getElementById('toggle-create-proxy').textContent = '+ Create Proxy';
    })
    .catch(function() {
        resultEl.innerHTML = '<div class="error-alert">An error occurred. Please try again.</div>';
    })
    .finally(function() {
        submitBtn.disabled = false;
        submitBtn.textContent = lfEditMode ? 'Save Changes' : 'Create HTTP Proxy';
    });
});

// ========== Reset form ==========
function lfResetForm() {
    document.getElementById('lf-port').value = '';
    document.getElementById('lf-interface').value = '__unselected__';
    document.getElementById('lf-bind').value = '3';
    document.getElementById('lf-tls-enable').checked = false;
    document.getElementById('lf-tls-fields').style.display = 'none';
    document.getElementById('lf-tls-acme').checked = false;
    document.getElementById('lf-tls-cert').value = '';
    document.getElementById('lf-tls-key').value = '';
    document.getElementById('lf-tls-cert-fields').style.display = '';
    document.getElementById('lf-disable-http2').checked = false;
    document.getElementById('lf-timeout-read').value = '';
    document.getElementById('lf-timeout-read-header').value = '';
    document.getElementById('lf-timeout-write').value = '';
    document.getElementById('lf-timeout-idle').value = '';
    document.getElementById('lf-default-enable').checked = false;
    document.getElementById('lf-default-route-fields').style.display = 'none';
    document.getElementById('lf-default-backend').value = '';
    document.getElementById('lf-default-replace-host').checked = false;
    document.getElementById('lf-default-disable-defaults').checked = false;
    if (lfDefaultMwChain) lfDefaultMwChain.clear();
    if (lfDefaultTpSection) {
        lfDefaultTpSection.querySelector('.lf-tp-preset').value = '';
        lfDefaultTpSection.querySelector('.lf-tp-header').value = '';
        lfDefaultTpSection.querySelector('.lf-tp-refresh').value = '';
        lfDefaultTpSection.querySelector('.lf-tp-ranges .mw-tag-list').innerHTML = '';
        lfUpdateTpBadge(lfDefaultTpSection);
    }
    lfDefaultPaths.forEach(function(p) { p.el.remove(); });
    lfDefaultPaths = [];
    lfHostRoutes.forEach(function(r) { r.el.remove(); });
    lfHostRoutes = [];
    document.getElementById('lf-result').innerHTML = '';
    lfShowStep(1);

    // Reset edit mode
    lfEditMode = false;
    lfEditPort = null;
    lfEditOriginalConfig = null;
    document.getElementById('lf-port').readOnly = false;
    document.getElementById('lf-port').style.opacity = '';
    document.querySelector('#create-proxy-panel > div:first-child h4').textContent = 'Create HTTP Proxy Server';
    document.getElementById('lf-submit').textContent = 'Create HTTP Proxy';
    document.getElementById('lf-review-subtitle').textContent = 'Review the full configuration before creating the HTTP proxy server. It will be created in a stopped state.';

    // Reset review tabs
    lfReviewMode = 'yaml';
    lfReviewEdited = false;
    document.getElementById('lf-review-editor').value = '';
    document.getElementById('lf-review-editor-json').value = '';
    document.getElementById('lf-review-editor-error').style.display = 'none';
    document.querySelectorAll('.lf-review-tab').forEach(function(b) {
        b.classList.toggle('active', b.getAttribute('data-review-tab') === 'yaml');
    });
}

// ---------- Create Proxy Panel Toggle ----------
document.getElementById('toggle-create-proxy').addEventListener('click', function () {
    var panel = document.getElementById('create-proxy-panel');
    var isVisible = panel.style.display !== 'none';
    if (isVisible) {
        panel.style.display = 'none';
        this.textContent = '+ Create Proxy';
        lfResetForm();
    } else {
        lfResetForm();
        initListenerWizard();
        panel.style.display = '';
        this.textContent = '− Cancel';
        panel.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
});
document.getElementById('close-create-proxy').addEventListener('click', function () {
    document.getElementById('create-proxy-panel').style.display = 'none';
    document.getElementById('toggle-create-proxy').textContent = '+ Create Proxy';
    lfResetForm();
});
