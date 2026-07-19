/* myFiPay dashboard SPA — vanilla JS, cookie-auth against /api */
(function () {
  'use strict';

  var me = null;

  // ── helpers ────────────────────────────────────────────────────────────────

  function api(path, opts) {
    opts = opts || {};
    var init = { method: opts.method || 'GET', credentials: 'same-origin', headers: {} };
    if (opts.body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(opts.body);
    }
    return fetch(path, init).then(function (res) {
      if (res.status === 401) { location.href = '/login'; throw new Error('unauthenticated'); }
      return res.json().then(function (json) {
        if (!res.ok) {
          var msg = (json && json.error && json.error.message) || 'Request failed';
          var err = new Error(msg);
          err.code = json && json.error && json.error.code;
          throw err;
        }
        return json.data;
      });
    });
  }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function ugx(n) {
    if (n == null || isNaN(n)) return '—';
    return Math.round(Number(n)).toLocaleString() + ' UGX';
  }

  function bytesFmt(b) {
    b = Number(b) || 0;
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
    return (b / 1073741824).toFixed(2) + ' GB';
  }

  function dt(iso) {
    if (!iso) return '—';
    var d = new Date(iso);
    if (isNaN(d)) return '—';
    return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) + ' ' +
           d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function dateOnly(iso) {
    if (!iso) return '—';
    var d = new Date(iso);
    if (isNaN(d)) return '—';
    return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
  }

  function durFmt(mins) {
    mins = Number(mins) || 0;
    if (mins < 60) return mins + ' min';
    if (mins < 1440) return (mins / 60) + ' hr';
    return (mins / 1440) + ' day' + (mins >= 2880 ? 's' : '');
  }

  function pill(status) {
    var map = {
      active: 'pill-ok', confirmed: 'pill-ok', paid: 'pill-ok', approved: 'pill-ok', used: 'pill-ok',
      pending: 'pill-warn', pending_kyc: 'pill-warn', unused: 'pill-mute',
      expired: 'pill-mute', inactive: 'pill-mute', terminated: 'pill-mute',
      rejected: 'pill-bad', failed: 'pill-bad', suspended: 'pill-bad'
    };
    return '<span class="pill ' + (map[status] || 'pill-mute') + '">' + esc(status) + '</span>';
  }

  function el(id) { return document.getElementById(id); }

  // Inline feather-style icons (stroke, 24 viewBox) — no external assets.
  var ICONS = {
    grid: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/>',
    activity: '<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
    tag: '<path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"/><line x1="7" y1="7" x2="7.01" y2="7"/>',
    pin: '<path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/>',
    router: '<line x1="22" y1="12" x2="2" y2="12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><line x1="6" y1="16" x2="6.01" y2="16"/><line x1="10" y1="16" x2="10.01" y2="16"/>',
    card: '<rect x="1" y="4" width="22" height="16" rx="2"/><line x1="1" y1="10" x2="23" y2="10"/>',
    gift: '<polyline points="20 12 20 22 4 22 4 12"/><rect x="2" y="7" width="20" height="5"/><line x1="12" y1="22" x2="12" y2="7"/><path d="M12 7H7.5a2.5 2.5 0 0 1 0-5C11 2 12 7 12 7z"/><path d="M12 7h4.5a2.5 2.5 0 0 0 0-5C13 2 12 7 12 7z"/>',
    dollar: '<line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>',
    usercheck: '<path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><polyline points="17 11 19 13 23 9"/>',
    users: '<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    barchart: '<line x1="12" y1="20" x2="12" y2="10"/><line x1="18" y1="20" x2="18" y2="4"/><line x1="6" y1="20" x2="6" y2="16"/>',
    briefcase: '<rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"/>',
    sliders: '<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>',
    wifi: '<path d="M5 12.55a11 11 0 0 1 14.08 0"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>',
    globe: '<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'
  };
  function ic(name, size) {
    size = size || 20;
    return '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="none" ' +
      'stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
      (ICONS[name] || ICONS.grid) + '</svg>';
  }

  // Pastel stat tile — variants assigned per view in fixed order (blue, cyan,
  // teal, amber, violet, coral), never cycled at runtime.
  function tile(variant, icon, label, valueHTML, sub) {
    return '<div class="tile t-' + variant + '"><span class="tile-ic">' + ic(icon, 22) + '</span>' +
      '<div class="label">' + esc(label) + '</div><div class="value">' + valueHTML + '</div>' +
      (sub ? '<div class="sub">' + esc(sub) + '</div>' : '') + '</div>';
  }

  function content() { return el('content'); }

  function setTitle(t, actionsHTML) {
    el('page-title').textContent = t;
    el('page-actions').innerHTML = actionsHTML || '';
  }

  function loading() { content().innerHTML = '<p class="empty-note">Loading…</p>'; }

  function errorView(e) {
    content().innerHTML = '<div class="panel"><p class="empty-note">Could not load this page: ' + esc(e.message) + '</p></div>';
  }

  function table(headers, rowsHTML, emptyMsg) {
    if (!rowsHTML.length) return '<p class="empty-note">' + esc(emptyMsg || 'Nothing here yet.') + '</p>';
    return '<div class="table-wrap"><table class="data"><thead><tr>' +
      headers.map(function (h) { return '<th>' + h + '</th>'; }).join('') +
      '</tr></thead><tbody>' + rowsHTML.join('') + '</tbody></table></div>';
  }

  // ── modal ──────────────────────────────────────────────────────────────────

  function openModal(title, bodyHTML) {
    el('modal-title').textContent = title;
    el('modal-body').innerHTML = bodyHTML;
    el('modal-overlay').style.display = 'flex';
  }
  function closeModal() { el('modal-overlay').style.display = 'none'; }
  el('modal-close').addEventListener('click', closeModal);
  el('modal-overlay').addEventListener('click', function (e) {
    if (e.target === el('modal-overlay')) closeModal();
  });

  // Wire a form inside the modal: gather named fields, POST, refresh view.
  function modalForm(formId, submit) {
    var form = el(formId);
    var errBox = form.querySelector('.alert-error');
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      if (errBox) errBox.classList.remove('show');
      var btn = form.querySelector('button[type=submit]');
      btn.disabled = true;
      Promise.resolve().then(function () { return submit(form); }).then(function () {
        closeModal();
        render();
      }).catch(function (err) {
        if (errBox) { errBox.textContent = err.message; errBox.classList.add('show'); }
        btn.disabled = false;
      });
    });
  }

  function field(label, name, attrs, hint) {
    return '<div class="field"><label for="f-' + name + '">' + esc(label) + '</label>' +
      '<input id="f-' + name + '" name="' + name + '" ' + (attrs || '') + '>' +
      (hint ? '<p class="hint">' + esc(hint) + '</p>' : '') + '</div>';
  }

  function selectField(label, name, options, hint) {
    return '<div class="field"><label for="f-' + name + '">' + esc(label) + '</label>' +
      '<select id="f-' + name + '" name="' + name + '">' + options + '</select>' +
      (hint ? '<p class="hint">' + esc(hint) + '</p>' : '') + '</div>';
  }

  function fval(form, name) {
    var f = form.querySelector('[name="' + name + '"]');
    return f ? f.value.trim() : '';
  }

  function planOptions(plans) {
    return plans.filter(function (p) { return p.status === 'active'; }).map(function (p) {
      return '<option value="' + esc(p.id) + '">' + esc(p.name) + ' — ' + ugx(p.price_ugx) +
        ' / ' + durFmt(p.duration_minutes) + ' (' + esc(p.location_name) + ')</option>';
    }).join('');
  }

  function locationOptions(locs) {
    return locs.map(function (l) {
      return '<option value="' + esc(l.id) + '">' + esc(l.name) + '</option>';
    }).join('');
  }

  // ── chart (inline SVG bar chart) ───────────────────────────────────────────

  function revenueChart(points) {
    if (!points || !points.length) return '<p class="chart-empty">No revenue in the last 30 days yet.</p>';
    var W = 720, H = 200, padL = 8, padB = 22, padT = 12;
    var max = Math.max.apply(null, points.map(function (p) { return p.revenue; })) || 1;
    var bw = (W - padL * 2) / points.length;
    var bars = points.map(function (p, i) {
      var h = Math.max(2, (p.revenue / max) * (H - padB - padT));
      var x = padL + i * bw;
      var label = new Date(p.day).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
      return '<rect x="' + (x + 1).toFixed(1) + '" y="' + (H - padB - h).toFixed(1) +
        '" width="' + Math.max(1, bw - 2).toFixed(1) + '" height="' + h.toFixed(1) +
        '" rx="3" fill="#5d87ff"><title>' + esc(label) + ': ' + ugx(p.revenue) + '</title></rect>';
    }).join('');
    var first = new Date(points[0].day).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
    var last = new Date(points[points.length - 1].day).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
    return '<div class="chart-box"><svg viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none">' + bars +
      '<text x="' + padL + '" y="' + (H - 6) + '" font-size="11" fill="#5a6a85">' + esc(first) + '</text>' +
      '<text x="' + (W - padL) + '" y="' + (H - 6) + '" font-size="11" fill="#5a6a85" text-anchor="end">' + esc(last) + '</text>' +
      '</svg></div>';
  }

  // ── views: operator ────────────────────────────────────────────────────────

  function viewOverview() {
    setTitle('Overview');
    loading();
    Promise.all([api('/api/dashboard/stats'), api('/api/dashboard/revenue-chart')]).then(function (r) {
      var s = r[0], chart = r[1];
      var firstName = ((me && me.name) || '').split(' ')[0];
      content().innerHTML =
        '<div class="welcome-banner"><div>' +
        '<h2>Welcome back' + (firstName ? ', ' + esc(firstName) : '') + '!</h2>' +
        '<p>Here is how your WiFi business is doing today.</p></div>' +
        '<div class="wb-figure">📶</div></div>' +
        '<div class="tile-grid">' +
        tile('blue', 'wifi', 'Online now', s.active_sessions) +
        tile('cyan', 'activity', 'Sessions today', s.today_sessions) +
        tile('teal', 'dollar', 'Revenue today', ugx(s.today_revenue)) +
        tile('amber', 'download', 'Data today', bytesFmt(s.today_bandwidth_bytes)) +
        tile('violet', 'pin', 'Locations', s.total_locations) +
        '</div>' +
        '<div class="panel"><h2>Revenue — last 30 days</h2>' + revenueChart(chart) + '</div>';
    }).catch(errorView);
  }

  function viewSessions() {
    var status = viewSessions.filter || 'all';
    setTitle('Sessions', '<button class="btn btn-primary btn-sm" id="grant-btn">Grant session</button>');
    loading();
    api('/api/sessions?status=' + status).then(function (sessions) {
      var rows = sessions.map(function (s) {
        var actions = '';
        if (s.status === 'active') {
          actions = '<div class="row-actions">' +
            '<button class="btn btn-ghost btn-sm act-extend" data-id="' + esc(s.id) + '">Extend</button>' +
            '<button class="btn btn-danger btn-sm act-term" data-id="' + esc(s.id) + '">End</button></div>';
        }
        return '<tr><td>' + esc(s.customer_phone || s.username) + '</td><td>' + esc(s.plan_name) +
          '</td><td>' + esc(s.location_name) + '</td><td>' + dt(s.started_at) + '</td><td>' + dt(s.expires_at) +
          '</td><td>' + bytesFmt(s.bytes_used) + '</td><td>' + pill(s.status) + '</td><td>' + actions + '</td></tr>';
      });
      content().innerHTML =
        '<div class="panel"><div class="panel-head"><h2>Sessions</h2>' +
        '<select id="sess-filter"><option value="all">All</option><option value="active">Active</option><option value="expired">Expired</option></select></div>' +
        table(['Customer', 'Plan', 'Location', 'Started', 'Expires', 'Data', 'Status', ''], rows, 'No sessions yet — they appear when customers pay or you grant access.') +
        '</div>';
      var filterSel = el('sess-filter');
      filterSel.value = status;
      filterSel.addEventListener('change', function () { viewSessions.filter = filterSel.value; viewSessions(); });

      content().querySelectorAll('.act-term').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('End this session now? The customer will be disconnected.')) return;
          api('/api/sessions/' + b.dataset.id, { method: 'DELETE' }).then(viewSessions).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-extend').forEach(function (b) {
        b.addEventListener('click', function () { extendModal(b.dataset.id); });
      });
      el('grant-btn').addEventListener('click', grantModal);
    }).catch(errorView);
  }

  function grantModal() {
    api('/api/plans').then(function (plans) {
      var opts = planOptions(plans);
      if (!opts) { alert('Create a plan first — sessions are granted against a plan.'); return; }
      openModal('Grant a session (cash sale)',
        '<div class="alert alert-error"></div><form id="grant-form">' +
        field('Customer phone', 'phone', 'required placeholder="0772 123 456"') +
        selectField('Plan', 'plan_id', opts) +
        field('Device MAC (optional)', 'mac', 'placeholder="AA:BB:CC:DD:EE:FF"') +
        field('Note (optional)', 'note', 'placeholder="e.g. paid cash at counter"') +
        '<button class="btn btn-primary btn-block" type="submit">Grant access</button></form>');
      modalForm('grant-form', function (form) {
        return api('/api/sessions/grant', { method: 'POST', body: {
          phone: fval(form, 'phone'), plan_id: fval(form, 'plan_id'),
          mac: fval(form, 'mac'), note: fval(form, 'note')
        } });
      });
    }).catch(function (e) { alert(e.message); });
  }

  function extendModal(sessionID) {
    api('/api/plans').then(function (plans) {
      openModal('Extend session',
        '<div class="alert alert-error"></div><form id="extend-form">' +
        selectField('Add time from plan', 'plan_id', planOptions(plans), 'The plan\'s duration is added to the current expiry.') +
        '<button class="btn btn-primary btn-block" type="submit">Extend</button></form>');
      modalForm('extend-form', function (form) {
        return api('/api/sessions/' + sessionID + '/extend', { method: 'POST', body: { plan_id: fval(form, 'plan_id') } });
      });
    }).catch(function (e) { alert(e.message); });
  }

  function viewPlans() {
    setTitle('Plans', '<button class="btn btn-primary btn-sm" id="new-plan-btn">New plan</button>');
    loading();
    Promise.all([api('/api/plans'), api('/api/locations')]).then(function (r) {
      var plans = r[0], locs = r[1];
      var rows = plans.map(function (p) {
        return '<tr><td>' + esc(p.name) + '</td><td>' + esc(p.location_name) + '</td><td>' + ugx(p.price_ugx) +
          '</td><td>' + durFmt(p.duration_minutes) + '</td><td>' + p.speed_down_kbps + '/' + p.speed_up_kbps + ' kbps</td><td>' +
          pill(p.status) + '</td><td><div class="row-actions">' +
          '<button class="btn btn-ghost btn-sm act-edit" data-id="' + esc(p.id) + '">Edit</button>' +
          (p.status === 'active' ? '<button class="btn btn-danger btn-sm act-del" data-id="' + esc(p.id) + '">Deactivate</button>' : '') +
          '</div></td></tr>';
      });
      content().innerHTML = '<div class="panel">' +
        table(['Plan', 'Location', 'Price', 'Duration', 'Speed ↓/↑', 'Status', ''], rows, 'No plans yet. Create your first plan — e.g. "1 Hour" at 1,000 UGX.') + '</div>';

      el('new-plan-btn').addEventListener('click', function () {
        if (!locs.length) { alert('Create a location first — plans belong to a location.'); return; }
        planModal(null, locs);
      });
      content().querySelectorAll('.act-edit').forEach(function (b) {
        b.addEventListener('click', function () {
          var p = plans.find(function (x) { return x.id === b.dataset.id; });
          planModal(p, locs);
        });
      });
      content().querySelectorAll('.act-del').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Deactivate this plan? It disappears from your portal; existing sessions keep running.')) return;
          api('/api/plans/' + b.dataset.id, { method: 'DELETE' }).then(viewPlans).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function planModal(p, locs) {
    var isEdit = !!p;
    openModal(isEdit ? 'Edit plan' : 'New plan',
      '<div class="alert alert-error"></div><form id="plan-form">' +
      (isEdit ? '' : selectField('Location', 'location_id', locationOptions(locs))) +
      field('Plan name', 'name', 'required value="' + esc(p ? p.name : '') + '" placeholder="e.g. 1 Hour"') +
      field('Price (UGX)', 'price_ugx', 'required type="number" min="100" value="' + (p ? p.price_ugx : '') + '"') +
      field('Duration (minutes)', 'duration_minutes', 'required type="number" min="5" value="' + (p ? p.duration_minutes : '') + '"', '60 = 1 hour, 1440 = 1 day') +
      field('Download speed (kbps)', 'speed_down_kbps', 'type="number" value="' + (p ? p.speed_down_kbps : 2048) + '"', '2048 = 2 Mbps') +
      field('Upload speed (kbps)', 'speed_up_kbps', 'type="number" value="' + (p ? p.speed_up_kbps : 512) + '"') +
      (isEdit ? selectField('Status', 'status',
        '<option value="active"' + (p.status === 'active' ? ' selected' : '') + '>Active</option>' +
        '<option value="inactive"' + (p.status !== 'active' ? ' selected' : '') + '>Inactive</option>') : '') +
      '<button class="btn btn-primary btn-block" type="submit">' + (isEdit ? 'Save changes' : 'Create plan') + '</button></form>');
    modalForm('plan-form', function (form) {
      var body = {
        name: fval(form, 'name'),
        price_ugx: Number(fval(form, 'price_ugx')),
        duration_minutes: Number(fval(form, 'duration_minutes')),
        speed_down_kbps: Number(fval(form, 'speed_down_kbps')) || 2048,
        speed_up_kbps: Number(fval(form, 'speed_up_kbps')) || 512
      };
      if (isEdit) {
        body.status = fval(form, 'status');
        return api('/api/plans/' + p.id, { method: 'PUT', body: body });
      }
      body.location_id = fval(form, 'location_id');
      return api('/api/plans', { method: 'POST', body: body });
    });
  }

  function viewLocations() {
    setTitle('Locations', '<button class="btn btn-primary btn-sm" id="new-loc-btn">New location</button>');
    loading();
    api('/api/locations').then(function (locs) {
      var rows = locs.map(function (l) {
        var portal = 'https://myfipay.com/portal/' + encodeURIComponent(l.portal_slug) + '/';
        return '<tr><td>' + esc(l.name) + '</td><td class="wrap">' + esc(l.address || '—') +
          '</td><td><a href="' + portal + '" target="_blank" rel="noopener">' + esc(l.portal_slug) + '</a></td><td>' +
          pill(l.status) + '</td><td><button class="btn btn-ghost btn-sm act-brand" data-id="' + esc(l.id) + '">Branding</button></td></tr>';
      });
      content().innerHTML = '<div class="panel">' +
        table(['Location', 'Address', 'Portal', 'Status', ''], rows, 'No locations yet. A location is one hotspot site with its own portal page.') + '</div>';

      el('new-loc-btn').addEventListener('click', function () {
        openModal('New location',
          '<div class="alert alert-error"></div><form id="loc-form">' +
          field('Location name', 'name', 'required placeholder="e.g. Main Street Shop"') +
          field('Address (optional)', 'address', 'placeholder="e.g. Soroti Main Street"') +
          field('Portal address', 'portal_slug', 'required placeholder="e.g. mainstreet" pattern="[a-z0-9-]+"',
            'Lowercase letters, numbers and dashes. Customers land on myfipay.com/portal/<address>/') +
          '<button class="btn btn-primary btn-block" type="submit">Create location</button></form>');
        modalForm('loc-form', function (form) {
          return api('/api/locations', { method: 'POST', body: {
            name: fval(form, 'name'), address: fval(form, 'address'), portal_slug: fval(form, 'portal_slug')
          } });
        });
      });

      content().querySelectorAll('.act-brand').forEach(function (b) {
        b.addEventListener('click', function () {
          api('/api/locations/' + b.dataset.id + '/branding').then(function (br) {
            openModal('Portal branding',
              '<div class="alert alert-error"></div><form id="brand-form">' +
              field('Portal name', 'portal_name', 'value="' + esc(br.portal_name) + '" placeholder="Shown as the portal title"') +
              field('Tagline', 'tagline', 'value="' + esc(br.tagline) + '" placeholder="e.g. Fast WiFi, fair prices"') +
              field('Brand colour', 'primary_color', 'type="color" value="' + esc(br.primary_color || '#0b7a4b') + '"') +
              field('Logo URL (optional)', 'logo_url', 'value="' + esc(br.logo_url) + '" placeholder="https://…"') +
              '<button class="btn btn-primary btn-block" type="submit">Save branding</button></form>');
            modalForm('brand-form', function (form) {
              return api('/api/locations/' + b.dataset.id + '/branding', { method: 'PUT', body: {
                portal_name: fval(form, 'portal_name'), tagline: fval(form, 'tagline'),
                primary_color: fval(form, 'primary_color'), logo_url: fval(form, 'logo_url')
              } });
            });
          }).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function viewRouters() {
    setTitle('Routers', '<button class="btn btn-primary btn-sm" id="add-router-btn">Add router</button>');
    loading();
    Promise.all([api('/api/devices'), api('/api/locations')]).then(function (r) {
      var devices = r[0], locs = r[1];
      var rows = devices.map(function (d) {
        var seen = d.last_seen ? 'Last seen ' + dt(d.last_seen) : 'Never connected';
        return '<tr><td>' + esc(d.name) + '</td><td>' + esc(d.location_name) + '</td><td>' + esc(d.nas_ip) +
          '</td><td>' + (d.online ? pill('active') : '<span class="pill pill-mute">offline</span>') +
          '<div class="hint">' + esc(seen) + '</div></td><td><div class="row-actions">' +
          '<button class="btn btn-ghost btn-sm act-setup" data-id="' + esc(d.id) + '">Setup</button>' +
          '<button class="btn btn-ghost btn-sm act-test" data-id="' + esc(d.id) + '">Test</button>' +
          '<button class="btn btn-ghost btn-sm act-edit" data-id="' + esc(d.id) + '">Edit</button>' +
          '<button class="btn btn-danger btn-sm act-del" data-id="' + esc(d.id) + '">Remove</button>' +
          '</div></td></tr>';
      });
      content().innerHTML =
        '<div class="panel">' +
        table(['Router', 'Location', 'Public IP', 'Status', ''], rows,
          'No routers connected yet. Add your MikroTik router to link it to your payment portal — no technical support needed.') +
        '</div>' +
        '<div class="panel"><h2>How it works</h2><p class="hint">' +
        '1. Add your router with its public IP address &nbsp;→&nbsp; 2. Paste the generated setup script into the MikroTik terminal ' +
        'and upload the login page file &nbsp;→&nbsp; 3. Run the connection test. ' +
        'Within a minute of adding a router, our RADIUS server starts accepting it. Your router needs a public IP ' +
        '(on MikroTik, check IP → Cloud, or search "what is my IP" from a device on the router\'s internet connection).</p></div>';

      el('add-router-btn').addEventListener('click', function () {
        if (!locs.length) { alert('Create a location first — a router belongs to a location.'); return; }
        openModal('Add your router',
          '<div class="alert alert-error"></div><form id="router-form">' +
          field('Router name', 'name', 'required placeholder="e.g. Main Street MikroTik"') +
          selectField('Location', 'location_id', locationOptions(locs)) +
          field('Router public IP', 'nas_ip', 'required placeholder="e.g. 41.210.12.34"',
            'The internet-facing IP of your router. On MikroTik: IP → Cloud shows it, or search "what is my IP".') +
          '<button class="btn btn-primary btn-block" type="submit">Register router</button></form>');
        var form = el('router-form');
        var errBox = form.querySelector('.alert-error');
        form.addEventListener('submit', function (e) {
          e.preventDefault();
          errBox.classList.remove('show');
          api('/api/devices', { method: 'POST', body: {
            name: fval(form, 'name'), location_id: fval(form, 'location_id'), nas_ip: fval(form, 'nas_ip')
          } }).then(function (d) {
            setupModal(d);
          }).catch(function (err) {
            errBox.textContent = err.message; errBox.classList.add('show');
          });
        });
      });

      content().querySelectorAll('.act-setup').forEach(function (b) {
        b.addEventListener('click', function () {
          setupModal(devices.find(function (x) { return x.id === b.dataset.id; }));
        });
      });
      content().querySelectorAll('.act-test').forEach(function (b) {
        b.addEventListener('click', function () { testModal(b.dataset.id); });
      });
      content().querySelectorAll('.act-edit').forEach(function (b) {
        b.addEventListener('click', function () {
          var d = devices.find(function (x) { return x.id === b.dataset.id; });
          openModal('Edit router',
            '<div class="alert alert-error"></div><form id="redit-form">' +
            field('Router name', 'name', 'required value="' + esc(d.name) + '"') +
            field('Router public IP', 'nas_ip', 'required value="' + esc(d.nas_ip) + '"',
              'If your router\'s public IP changed, update it here — RADIUS follows within a minute.') +
            '<button class="btn btn-primary btn-block" type="submit">Save</button></form>');
          modalForm('redit-form', function (form) {
            return api('/api/devices/' + d.id, { method: 'PUT', body: {
              name: fval(form, 'name'), nas_ip: fval(form, 'nas_ip')
            } });
          });
        });
      });
      content().querySelectorAll('.act-del').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Remove this router? It will no longer be able to authenticate customers.')) return;
          api('/api/devices/' + b.dataset.id, { method: 'DELETE' }).then(viewRouters).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function routerLoginHTML(slug) {
    var u = 'https://myfipay.com/portal/' + slug + '/?mac=$(mac)&ip=$(ip)&login=$(link-login-only)';
    return '<html><head><meta http-equiv="refresh" content="0; url=' + u + '"></head>' +
      '<body><a href="' + u + '">Continue to payment portal</a></body></html>\n';
  }

  function setupModal(d) {
    fetch('/api/devices/' + d.id + '/script', { credentials: 'same-origin' }).then(function (res) {
      if (res.status === 401) { location.href = '/login'; throw new Error('unauthenticated'); }
      if (!res.ok) throw new Error('could not load setup script');
      return res.text();
    }).then(function (script) {
      openModal('Set up — ' + d.name,
        '<p class="hint" style="margin-bottom:10px"><strong>Step 1.</strong> On the router, run MikroTik\'s built-in Hotspot Setup once (IP → Hotspot → Hotspot Setup) if you haven\'t.</p>' +
        '<p class="hint" style="margin-bottom:6px"><strong>Step 2.</strong> Copy this script and paste it into the MikroTik Terminal:</p>' +
        '<textarea readonly style="width:100%;height:180px;font-family:monospace;font-size:12px;border:1px solid var(--line);border-radius:8px;padding:10px">' + esc(script) + '</textarea>' +
        '<div class="copy-row" style="margin:8px 0 14px"><button class="btn btn-ghost btn-sm" id="copy-script">Copy script</button>' +
        '<button class="btn btn-ghost btn-sm" id="dl-login">Download login.html</button></div>' +
        '<p class="hint" style="margin-bottom:10px"><strong>Step 3.</strong> Upload the downloaded <code>login.html</code> into the router\'s <code>hotspot</code> folder (Winbox → Files), replacing the existing one.</p>' +
        '<p class="hint" style="margin-bottom:14px"><strong>Step 4.</strong> Wait one minute, then run the connection test.</p>' +
        '<button class="btn btn-primary btn-block" id="goto-test">Test connection</button>');
      el('copy-script').addEventListener('click', function () {
        navigator.clipboard.writeText(script).then(function () {
          el('copy-script').textContent = 'Copied!';
          setTimeout(function () { el('copy-script').textContent = 'Copy script'; }, 1500);
        });
      });
      el('dl-login').addEventListener('click', function () {
        var blob = new Blob([routerLoginHTML(d.portal_slug)], { type: 'text/html' });
        var a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = 'login.html';
        a.click();
        URL.revokeObjectURL(a.href);
      });
      el('goto-test').addEventListener('click', function () { testModal(d.id); });
    }).catch(function (e) { alert(e.message); });
  }

  function testModal(deviceID) {
    openModal('Connection test', '<p class="empty-note" id="test-out">Checking…</p>' +
      '<button class="btn btn-primary btn-block" id="retest">Check again</button>' +
      '<p class="hint" style="margin-top:10px">Tip: connect a phone to the WiFi and try to open any website — ' +
      'the portal should appear. That attempt registers here within seconds.</p>');
    function run() {
      el('test-out').textContent = 'Checking…';
      api('/api/devices/' + deviceID + '/status').then(function (s) {
        var msg;
        if (s.online) {
          msg = '✅ Router is talking to myFiPay! Last activity: ' + dt(s.last_seen) + '.';
        } else if (s.last_seen) {
          msg = '⚠️ Router has connected before (last: ' + dt(s.last_seen) + ') but not in the past 10 minutes. ' +
            'Trigger a login attempt from a phone on the WiFi and check again.';
        } else {
          msg = '❌ No contact from this router yet. Make sure the script ran without errors and the router\'s ' +
            'public IP is correct, wait a minute, then have a phone on the WiFi try to open a website.';
        }
        el('test-out').textContent = msg;
      }).catch(function (e) { el('test-out').textContent = 'Check failed: ' + e.message; });
    }
    el('retest').addEventListener('click', run);
    run();
  }

  function viewPayments() {
    var method = viewPayments.filter || '';
    setTitle('Payments');
    loading();
    api('/api/payments' + (method ? '?method=' + method : '')).then(function (payments) {
      var rows = payments.map(function (p) {
        return '<tr><td>' + esc(p.customer_phone) + '</td><td>' + ugx(p.amount_ugx) + '</td><td>' +
          esc(p.method === 'mobile_money' ? 'Mobile money' : 'Cash') + '</td><td>' + esc(p.plan_name) + '</td><td>' +
          esc(p.location_name) + '</td><td>' + dt(p.paid_at) + '</td></tr>';
      });
      content().innerHTML =
        '<div class="panel"><div class="panel-head"><h2>Confirmed payments</h2>' +
        '<select id="pay-filter"><option value="">All methods</option><option value="mobile_money">Mobile money</option><option value="cash">Cash</option></select></div>' +
        table(['Customer', 'Amount', 'Method', 'Plan', 'Location', 'Paid at'], rows, 'No payments yet.') + '</div>';
      var sel = el('pay-filter');
      sel.value = method;
      sel.addEventListener('change', function () { viewPayments.filter = sel.value; viewPayments(); });
    }).catch(errorView);
  }

  function viewVouchers() {
    setTitle('Vouchers', '<button class="btn btn-primary btn-sm" id="new-batch-btn">New batch</button>');
    loading();
    Promise.all([api('/api/vouchers/batches'), api('/api/locations'), api('/api/plans')]).then(function (r) {
      var batches = r[0], locs = r[1], plans = r[2];
      var rows = batches.map(function (b) {
        return '<tr><td>' + dateOnly(b.created_at) + '</td><td>' + esc(b.location_name) + '</td><td>' + esc(b.plan_name) +
          '</td><td>' + ugx(b.price_ugx) + '</td><td>' + b.used_count + ' / ' + b.quantity + '</td><td class="wrap">' + esc(b.note || '—') +
          '</td><td><button class="btn btn-ghost btn-sm act-view" data-id="' + esc(b.id) + '">View codes</button></td></tr>';
      });
      content().innerHTML = '<div class="panel">' +
        table(['Created', 'Location', 'Plan', 'Value', 'Used', 'Note', ''], rows, 'No voucher batches yet. Vouchers let you sell WiFi offline — print codes, customers redeem them on the portal.') + '</div>';

      el('new-batch-btn').addEventListener('click', function () {
        var opts = planOptions(plans);
        if (!locs.length || !opts) { alert('Create a location and a plan first.'); return; }
        openModal('New voucher batch',
          '<div class="alert alert-error"></div><form id="batch-form">' +
          selectField('Location', 'location_id', locationOptions(locs)) +
          selectField('Plan', 'plan_id', opts) +
          field('Quantity', 'quantity', 'required type="number" min="1" max="500" value="20"') +
          field('Note (optional)', 'note', 'placeholder="e.g. for Musa\'s kiosk"') +
          '<button class="btn btn-primary btn-block" type="submit">Generate vouchers</button></form>');
        modalForm('batch-form', function (form) {
          return api('/api/vouchers/batches', { method: 'POST', body: {
            location_id: fval(form, 'location_id'), plan_id: fval(form, 'plan_id'),
            quantity: Number(fval(form, 'quantity')), note: fval(form, 'note')
          } });
        });
      });

      content().querySelectorAll('.act-view').forEach(function (b) {
        b.addEventListener('click', function () {
          var batch = batches.find(function (x) { return x.id === b.dataset.id; });
          api('/api/vouchers/batches/' + b.dataset.id).then(function (vouchers) {
            var cells = vouchers.map(function (v) {
              return '<div class="code-cell' + (v.status === 'used' ? ' used' : '') + '">' + esc(v.code) + '</div>';
            }).join('');
            openModal('Voucher codes — ' + (batch ? batch.plan_name : ''),
              '<p class="hint" style="margin-bottom:12px">Struck-through codes are already used.</p>' +
              '<div class="code-grid" style="margin-bottom:16px">' + cells + '</div>' +
              '<button class="btn btn-primary btn-block" id="print-codes">Print voucher sheet</button>');
            el('print-codes').addEventListener('click', function () {
              printVoucherSheet(batch, vouchers.filter(function (v) { return v.status !== 'used'; }));
            });
          }).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function printVoucherSheet(batch, vouchers) {
    var head = batch ?
      '<div class="print-head"><strong>' + esc(batch.plan_name) + '</strong> — ' + ugx(batch.price_ugx) +
      ' · ' + esc(batch.location_name) + ' · Redeem at the WiFi portal</div>' : '';
    el('print-area').innerHTML = head + '<div class="code-grid">' +
      vouchers.map(function (v) { return '<div class="code-cell">' + esc(v.code) + '</div>'; }).join('') + '</div>';
    document.body.classList.add('printing');
    window.print();
    document.body.classList.remove('printing');
  }

  function viewPayouts() {
    setTitle('Payouts', '<button class="btn btn-primary btn-sm" id="req-payout-btn">Request payout</button>');
    loading();
    Promise.all([api('/api/payouts/balance'), api('/api/payouts')]).then(function (r) {
      var bal = r[0], payouts = r[1];
      var rows = payouts.map(function (p) {
        return '<tr><td>' + dt(p.requested_at) + '</td><td>' + ugx(p.amount_ugx) + '</td><td>' + esc(p.momo_phone) +
          '</td><td>' + pill(p.status) + '</td><td class="wrap">' +
          esc(p.status === 'rejected' && p.rejection_reason ? p.rejection_reason : (p.reference || p.note || '—')) +
          '</td><td>' + (p.paid_at ? dt(p.paid_at) : '—') + '</td></tr>';
      });
      content().innerHTML =
        '<div class="tile-grid">' +
        tile('blue', 'dollar', 'Available to withdraw', ugx(bal.available_ugx), 'min ' + ugx(bal.min_payout_ugx)) +
        tile('cyan', 'card', 'Mobile-money collected', ugx(bal.gross_mobile_money_ugx)) +
        tile('teal', 'barchart', 'Platform fee (' + Math.round(bal.commission_rate * 100) + '%)', ugx(bal.commission_ugx)) +
        tile('amber', 'activity', 'Already requested', ugx(bal.already_requested_ugx)) +
        '</div>' +
        '<div class="panel"><h2>Payout history</h2>' +
        table(['Requested', 'Amount', 'To', 'Status', 'Reference / reason', 'Paid'], rows, 'No payout requests yet.') + '</div>';

      el('req-payout-btn').addEventListener('click', function () {
        openModal('Request a payout',
          '<div class="alert alert-error"></div><form id="payout-form">' +
          field('Amount (UGX)', 'amount_ugx', 'required type="number" min="' + bal.min_payout_ugx + '" max="' + bal.available_ugx + '" value="' + bal.available_ugx + '"',
            'Available: ' + ugx(bal.available_ugx)) +
          field('Mobile money number', 'momo_phone', 'required placeholder="0772 123 456"') +
          field('Registered name', 'momo_name', 'required placeholder="Name on the mobile money account"') +
          field('Note (optional)', 'note', '') +
          '<button class="btn btn-primary btn-block" type="submit">Request payout</button></form>');
        modalForm('payout-form', function (form) {
          return api('/api/payouts', { method: 'POST', body: {
            amount_ugx: Number(fval(form, 'amount_ugx')), momo_phone: fval(form, 'momo_phone'),
            momo_name: fval(form, 'momo_name'), note: fval(form, 'note')
          } });
        });
      });
    }).catch(errorView);
  }

  function viewSettings() {
    setTitle('Settings');
    loading();
    api('/api/profile').then(function (p) {
      content().innerHTML =
        '<div class="panel"><h2>Profile</h2><div class="alert alert-error" id="prof-err"></div><div class="alert alert-ok" id="prof-ok"></div>' +
        '<form id="prof-form" style="max-width:420px">' +
        field('Name', 'name', 'required value="' + esc(p.name) + '"') +
        field('Phone', 'phone', 'value="' + esc(p.phone || '') + '"') +
        '<div class="field"><label>Email</label><input value="' + esc(p.email) + '" disabled></div>' +
        '<button class="btn btn-primary" type="submit">Save profile</button></form></div>' +
        '<div class="panel"><h2>Change password</h2><div class="alert alert-error" id="pw-err"></div><div class="alert alert-ok" id="pw-ok"></div>' +
        '<form id="pw-form" style="max-width:420px">' +
        field('Current password', 'current_password', 'required type="password" autocomplete="current-password"') +
        field('New password', 'new_password', 'required type="password" autocomplete="new-password"', 'At least 8 characters.') +
        '<button class="btn btn-primary" type="submit">Change password</button></form></div>';

      el('prof-form').addEventListener('submit', function (e) {
        e.preventDefault();
        el('prof-err').classList.remove('show'); el('prof-ok').classList.remove('show');
        api('/api/profile', { method: 'PUT', body: { name: fval(el('prof-form'), 'name'), phone: fval(el('prof-form'), 'phone') } })
          .then(function () { el('prof-ok').textContent = 'Profile saved.'; el('prof-ok').classList.add('show'); })
          .catch(function (err) { el('prof-err').textContent = err.message; el('prof-err').classList.add('show'); });
      });
      el('pw-form').addEventListener('submit', function (e) {
        e.preventDefault();
        el('pw-err').classList.remove('show'); el('pw-ok').classList.remove('show');
        api('/api/auth/password', { method: 'PUT', body: {
          current_password: el('pw-form').querySelector('[name=current_password]').value,
          new_password: el('pw-form').querySelector('[name=new_password]').value
        } }).then(function () {
          el('pw-ok').textContent = 'Password changed.'; el('pw-ok').classList.add('show');
          el('pw-form').reset();
        }).catch(function (err) { el('pw-err').textContent = err.message; el('pw-err').classList.add('show'); });
      });
    }).catch(errorView);
  }

  // ── views: agent ───────────────────────────────────────────────────────────

  function viewAgent() {
    setTitle('Agent overview');
    loading();
    Promise.all([api('/api/agent/dashboard'), api('/api/agent/invite'), api('/api/agent/operators'), api('/api/agent/commissions')])
      .then(function (r) {
        var d = r[0], invite = r[1], ops = r[2], comms = r[3];
        var inviteURL = location.origin + '/signup?agent=' + encodeURIComponent(invite.invite_code);
        var opRows = ops.map(function (o) {
          return '<tr><td>' + esc(o.name) + '</td><td>' + pill(o.status) + '</td><td>' + dateOnly(o.joined_at) +
            '</td><td>' + ugx(o.total_commission_ugx) + '</td></tr>';
        });
        var commRows = comms.slice(0, 50).map(function (c) {
          return '<tr><td>' + dt(c.created_at) + '</td><td>' + esc(c.operator_name) + '</td><td>' + ugx(c.amount_ugx) +
            '</td><td>' + c.rate_pct + '%</td><td>' + pill(c.status) + '</td></tr>';
        });
        content().innerHTML =
          '<div class="tile-grid">' +
          tile('blue', 'users', 'Operators referred', d.operator_count) +
          tile('cyan', 'barchart', 'Total earned', ugx(d.total_earned_ugx)) +
          tile('teal', 'dollar', 'Available balance', ugx(d.available_balance)) +
          tile('amber', 'activity', 'Pending payouts', d.pending_payouts) +
          '</div>' +
          '<div class="panel"><h2>Your invite link</h2>' +
          '<p class="hint" style="margin-bottom:10px">Operators who sign up through this link are yours — you earn 3% of every mobile-money payment they collect.</p>' +
          '<div class="copy-row"><input id="invite-url" readonly value="' + esc(inviteURL) + '">' +
          '<button class="btn btn-ghost" id="copy-invite">Copy</button></div></div>' +
          '<div class="panel"><h2>Your operators</h2>' + table(['Operator', 'Status', 'Joined', 'Commission earned'], opRows, 'No operators yet — share your invite link.') + '</div>' +
          '<div class="panel"><h2>Recent commissions</h2>' + table(['Date', 'Operator', 'Amount', 'Rate', 'Status'], commRows, 'Commissions appear when your operators collect mobile-money payments.') + '</div>';
        el('copy-invite').addEventListener('click', function () {
          el('invite-url').select();
          navigator.clipboard.writeText(inviteURL).then(function () {
            el('copy-invite').textContent = 'Copied!';
            setTimeout(function () { el('copy-invite').textContent = 'Copy'; }, 1500);
          });
        });
      }).catch(errorView);
  }

  function viewAgentPayouts() {
    setTitle('My payouts', '<button class="btn btn-primary btn-sm" id="req-payout-btn">Request payout</button>');
    loading();
    Promise.all([api('/api/agent/dashboard'), api('/api/agent/payouts')]).then(function (r) {
      var d = r[0], payouts = r[1];
      var rows = payouts.map(function (p) {
        return '<tr><td>' + dt(p.requested_at) + '</td><td>' + ugx(p.amount_ugx) + '</td><td>' + esc(p.phone) +
          '</td><td>' + pill(p.status) + '</td><td>' + (p.processed_at ? dt(p.processed_at) : '—') + '</td></tr>';
      });
      content().innerHTML =
        '<div class="tile-grid">' + tile('blue', 'dollar', 'Available balance', ugx(d.available_balance)) + '</div>' +
        '<div class="panel"><h2>Payout history</h2>' + table(['Requested', 'Amount', 'To', 'Status', 'Processed'], rows, 'No payout requests yet.') + '</div>';
      el('req-payout-btn').addEventListener('click', function () {
        openModal('Request a payout',
          '<div class="alert alert-error"></div><form id="apayout-form">' +
          field('Amount (UGX)', 'amount_ugx', 'required type="number" min="5000" value="' + Math.floor(d.available_balance) + '"',
            'Available: ' + ugx(d.available_balance) + ' · minimum 5,000 UGX') +
          field('Mobile money number', 'phone', 'required placeholder="0772 123 456"') +
          field('Note (optional)', 'notes', '') +
          '<button class="btn btn-primary btn-block" type="submit">Request payout</button></form>');
        modalForm('apayout-form', function (form) {
          return api('/api/agent/payouts', { method: 'POST', body: {
            amount_ugx: Number(fval(form, 'amount_ugx')), method: 'mobile_money',
            phone: fval(form, 'phone'), notes: fval(form, 'notes')
          } });
        });
      });
    }).catch(errorView);
  }

  // ── views: admin ───────────────────────────────────────────────────────────

  function viewAdminKYC() {
    var status = viewAdminKYC.filter || 'pending_kyc';
    setTitle('KYC queue');
    loading();
    api('/api/admin/kyc?status=' + status).then(function (ops) {
      var rows = ops.map(function (o) {
        var actions = '';
        if (o.status === 'pending_kyc' || o.status === 'rejected') {
          actions += '<button class="btn btn-primary btn-sm act-approve" data-id="' + esc(o.tenant_id) + '">Approve</button>';
        }
        if (o.status === 'pending_kyc') {
          actions += '<button class="btn btn-danger btn-sm act-reject" data-id="' + esc(o.tenant_id) + '">Reject</button>';
        }
        return '<tr><td>' + esc(o.business_name) + '</td><td>' + esc(o.name) + '</td><td>' + esc(o.email) +
          '</td><td>' + esc(o.phone || '—') + '</td><td>' + esc(o.district || '—') + '</td><td>' + dateOnly(o.applied_at) +
          '</td><td>' + pill(o.status) + (o.rejection_reason ? '<div class="hint">' + esc(o.rejection_reason) + '</div>' : '') +
          '</td><td><div class="row-actions">' + actions + '</div></td></tr>';
      });
      content().innerHTML =
        '<div class="panel"><div class="panel-head"><h2>Operator applications</h2>' +
        '<select id="kyc-filter"><option value="pending_kyc">Pending</option><option value="active">Approved</option><option value="rejected">Rejected</option></select></div>' +
        table(['Business', 'Applicant', 'Email', 'Phone', 'District', 'Applied', 'Status', ''], rows, 'Queue is empty.') + '</div>';
      var sel = el('kyc-filter');
      sel.value = status;
      sel.addEventListener('change', function () { viewAdminKYC.filter = sel.value; viewAdminKYC(); });

      content().querySelectorAll('.act-approve').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Approve this operator? They can sign in immediately.')) return;
          api('/api/admin/kyc/' + b.dataset.id + '/approve', { method: 'POST', body: {} }).then(viewAdminKYC).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-reject').forEach(function (b) {
        b.addEventListener('click', function () {
          var reason = prompt('Reason for rejection (shown to the applicant):');
          if (reason === null) return;
          api('/api/admin/kyc/' + b.dataset.id + '/reject', { method: 'POST', body: { reason: reason } }).then(viewAdminKYC).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function viewAdminTenants() {
    setTitle('Operators');
    loading();
    api('/api/admin/tenants').then(function (tenants) {
      var rows = tenants.map(function (t) {
        return '<tr><td>' + esc(t.name) + '</td><td>' + esc(t.owner_email || '—') + '</td><td>' + pill(t.status) +
          '</td><td>' + t.location_count + '</td><td>' + t.session_count + '</td><td>' + ugx(t.total_revenue) +
          '</td><td>' + dateOnly(t.created_at) + '</td></tr>';
      });
      content().innerHTML = '<div class="panel">' +
        table(['Operator', 'Owner', 'Status', 'Locations', 'Sessions', 'Revenue', 'Joined'], rows, 'No operators yet.') + '</div>';
    }).catch(errorView);
  }

  function viewAdminRevenue() {
    setTitle('Platform revenue');
    loading();
    api('/api/admin/revenue').then(function (r) {
      var rows = (r.by_tenant || []).map(function (t) {
        return '<tr><td>' + esc(t.name) + '</td><td>' + pill(t.status) + '</td><td>' + t.sessions + '</td><td>' + ugx(t.revenue) + '</td></tr>';
      });
      content().innerHTML =
        '<div class="tile-grid">' +
        tile('blue', 'barchart', 'Total revenue', ugx(r.total_revenue)) +
        tile('cyan', 'dollar', 'Revenue today', ugx(r.today_revenue)) +
        tile('teal', 'activity', 'Total sessions', r.total_sessions) +
        tile('amber', 'wifi', 'Sessions today', r.today_sessions) +
        '</div>' +
        '<div class="panel"><h2>Revenue — last 30 days</h2>' + revenueChart(r.chart) + '</div>' +
        '<div class="panel"><h2>By operator</h2>' + table(['Operator', 'Status', 'Sessions', 'Revenue'], rows, 'No revenue yet.') + '</div>';
    }).catch(errorView);
  }

  function viewAdminPayouts() {
    var status = viewAdminPayouts.filter || 'pending';
    setTitle('Operator payouts');
    loading();
    api('/api/admin/payouts?status=' + (status === 'all' ? '' : status)).then(function (payouts) {
      var rows = payouts.map(function (p) {
        var actions = '';
        if (p.status === 'pending') {
          actions = '<button class="btn btn-primary btn-sm act-approve" data-id="' + esc(p.id) + '">Approve</button>' +
            '<button class="btn btn-danger btn-sm act-reject" data-id="' + esc(p.id) + '">Reject</button>';
        } else if (p.status === 'approved') {
          actions = '<button class="btn btn-primary btn-sm act-paid" data-id="' + esc(p.id) + '">Mark paid</button>';
        }
        return '<tr><td>' + esc(p.tenant_name) + '</td><td>' + ugx(p.amount_ugx) + '</td><td>' + esc(p.momo_phone) +
          '<div class="hint">' + esc(p.momo_name || '') + '</div></td><td>' + pill(p.status) + '</td><td>' + dt(p.requested_at) +
          '</td><td class="wrap">' + esc(p.rejection_reason || p.note || '—') + '</td><td><div class="row-actions">' + actions + '</div></td></tr>';
      });
      content().innerHTML =
        '<div class="panel"><div class="panel-head"><h2>Payout queue</h2>' +
        '<select id="po-filter"><option value="pending">Pending</option><option value="approved">Approved</option><option value="paid">Paid</option><option value="rejected">Rejected</option><option value="all">All</option></select></div>' +
        table(['Operator', 'Amount', 'To', 'Status', 'Requested', 'Note', ''], rows, 'Queue is empty.') + '</div>';
      var sel = el('po-filter');
      sel.value = status;
      sel.addEventListener('change', function () { viewAdminPayouts.filter = sel.value; viewAdminPayouts(); });

      content().querySelectorAll('.act-approve').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Approve this payout?')) return;
          api('/api/admin/payouts/' + b.dataset.id + '/approve', { method: 'POST', body: {} }).then(viewAdminPayouts).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-reject').forEach(function (b) {
        b.addEventListener('click', function () {
          var reason = prompt('Reason for rejection:');
          if (reason === null) return;
          api('/api/admin/payouts/' + b.dataset.id + '/reject', { method: 'POST', body: { reason: reason } }).then(viewAdminPayouts).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-paid').forEach(function (b) {
        b.addEventListener('click', function () {
          var ref = prompt('Payment reference (e.g. MoMo transaction ID):');
          if (ref === null) return;
          api('/api/admin/payouts/' + b.dataset.id + '/mark-paid', { method: 'POST', body: { reference: ref } }).then(viewAdminPayouts).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  function viewAdminAgents() {
    var status = viewAdminAgents.filter || 'pending';
    setTitle('Agents');
    loading();
    Promise.all([api('/api/admin/agents'), api('/api/admin/agent-payouts?status=' + (status === 'all' ? '' : status))]).then(function (r) {
      var agents = r[0], payouts = r[1];
      var agentRows = agents.map(function (a) {
        return '<tr><td>' + esc(a.name) + '</td><td>' + esc(a.email) + '</td><td>' + esc(a.slug) + '</td><td>' + pill(a.status) +
          '</td><td>' + a.operator_count + '</td><td>' + ugx(a.total_commission_ugx) + '</td><td>' + dateOnly(a.created_at) + '</td></tr>';
      });
      var poRows = payouts.map(function (p) {
        var actions = '';
        if (p.status === 'pending') {
          actions = '<button class="btn btn-primary btn-sm act-approve" data-id="' + esc(p.id) + '">Approve</button>' +
            '<button class="btn btn-danger btn-sm act-reject" data-id="' + esc(p.id) + '">Reject</button>';
        } else if (p.status === 'approved') {
          actions = '<button class="btn btn-primary btn-sm act-paid" data-id="' + esc(p.id) + '">Mark paid</button>';
        }
        return '<tr><td>' + esc(p.agent_name) + '<div class="hint">' + esc(p.agent_email) + '</div></td><td>' + ugx(p.amount_ugx) +
          '</td><td>' + esc(p.phone) + '</td><td>' + pill(p.status) + '</td><td>' + dt(p.requested_at) +
          '</td><td><div class="row-actions">' + actions + '</div></td></tr>';
      });
      content().innerHTML =
        '<div class="panel"><h2>Agents</h2>' + table(['Agent', 'Email', 'Invite code', 'Status', 'Operators', 'Commission', 'Joined'], agentRows, 'No agents yet.') + '</div>' +
        '<div class="panel"><div class="panel-head"><h2>Agent payout requests</h2>' +
        '<select id="ap-filter"><option value="pending">Pending</option><option value="approved">Approved</option><option value="paid">Paid</option><option value="rejected">Rejected</option><option value="all">All</option></select></div>' +
        table(['Agent', 'Amount', 'To', 'Status', 'Requested', ''], poRows, 'No payout requests.') + '</div>';
      var sel = el('ap-filter');
      sel.value = status;
      sel.addEventListener('change', function () { viewAdminAgents.filter = sel.value; viewAdminAgents(); });

      content().querySelectorAll('.act-approve').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Approve this agent payout?')) return;
          api('/api/admin/agent-payouts/' + b.dataset.id + '/approve', { method: 'POST', body: {} }).then(viewAdminAgents).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-reject').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Reject this agent payout?')) return;
          api('/api/admin/agent-payouts/' + b.dataset.id + '/reject', { method: 'POST', body: {} }).then(viewAdminAgents).catch(function (e) { alert(e.message); });
        });
      });
      content().querySelectorAll('.act-paid').forEach(function (b) {
        b.addEventListener('click', function () {
          if (!confirm('Mark this agent payout as paid?')) return;
          api('/api/admin/agent-payouts/' + b.dataset.id + '/paid', { method: 'POST', body: {} }).then(viewAdminAgents).catch(function (e) { alert(e.message); });
        });
      });
    }).catch(errorView);
  }

  // ── routing ────────────────────────────────────────────────────────────────

  var routes = {
    overview: { title: 'Overview', icon: 'grid', sep: 'Dashboard', view: viewOverview, roles: ['operator'] },
    sessions: { title: 'Sessions', icon: 'activity', sep: 'Manage', view: viewSessions, roles: ['operator'] },
    plans: { title: 'Plans', icon: 'tag', view: viewPlans, roles: ['operator'] },
    locations: { title: 'Locations', icon: 'pin', view: viewLocations, roles: ['operator'] },
    routers: { title: 'Routers', icon: 'router', view: viewRouters, roles: ['operator'] },
    payments: { title: 'Payments', icon: 'card', sep: 'Money', view: viewPayments, roles: ['operator'] },
    vouchers: { title: 'Vouchers', icon: 'gift', view: viewVouchers, roles: ['operator'] },
    payouts: { title: 'Payouts', icon: 'dollar', view: viewPayouts, roles: ['operator'] },
    agent: { title: 'Overview', icon: 'grid', sep: 'Dashboard', view: viewAgent, roles: ['agent'] },
    'agent-payouts': { title: 'My payouts', icon: 'dollar', sep: 'Money', view: viewAgentPayouts, roles: ['agent'] },
    'admin-kyc': { title: 'KYC queue', icon: 'usercheck', sep: 'Platform', view: viewAdminKYC, roles: ['admin', 'super_admin'] },
    'admin-tenants': { title: 'Operators', icon: 'briefcase', view: viewAdminTenants, roles: ['admin', 'super_admin'] },
    'admin-agents': { title: 'Agents', icon: 'users', view: viewAdminAgents, roles: ['admin', 'super_admin'] },
    'admin-revenue': { title: 'Revenue', icon: 'barchart', sep: 'Money', view: viewAdminRevenue, roles: ['admin', 'super_admin'] },
    'admin-payouts': { title: 'Operator payouts', icon: 'dollar', view: viewAdminPayouts, roles: ['admin', 'super_admin'] },
    settings: { title: 'Settings', icon: 'sliders', sep: 'Account', view: viewSettings, roles: ['operator', 'agent', 'admin', 'super_admin'] }
  };

  function defaultRoute() {
    if (me.role === 'agent') return 'agent';
    if (me.role === 'admin' || me.role === 'super_admin') return 'admin-kyc';
    return 'overview';
  }

  function currentRoute() {
    var name = (location.hash || '').replace(/^#\/?/, '');
    var r = routes[name];
    if (!r || r.roles.indexOf(me.role) === -1) return defaultRoute();
    return name;
  }

  function buildNav() {
    var items = Object.keys(routes).filter(function (name) {
      return routes[name].roles.indexOf(me.role) !== -1;
    });
    el('dash-nav').innerHTML = items.map(function (name) {
      var r = routes[name];
      return (r.sep ? '<div class="nav-sep">' + esc(r.sep) + '</div>' : '') +
        '<a href="#/' + name + '" data-route="' + name + '">' + ic(r.icon, 18) +
        '<span>' + esc(r.title) + '</span></a>';
    }).join('');
    el('dash-nav').addEventListener('click', function () {
      el('sidebar').classList.remove('open');
    });
  }

  function render() {
    closeModal();
    var name = currentRoute();
    el('dash-nav').querySelectorAll('a').forEach(function (a) {
      a.classList.toggle('active', a.dataset.route === name);
    });
    routes[name].view();
  }

  // ── boot ───────────────────────────────────────────────────────────────────

  api('/api/auth/me').then(function (u) {
    me = u;
    var display = u.name || u.email;
    el('dash-user').innerHTML =
      '<span class="dash-user-avatar">' + esc(display.charAt(0).toUpperCase()) + '</span>' +
      '<span class="dash-user-meta"><span class="u-name" title="' + esc(display) + '">' + esc(display) + '</span>' +
      '<span class="u-role">' + esc((u.role || '').replace(/_/g, ' ')) + '</span></span>';
    buildNav();
    el('dash-layout').style.display = '';
    if (!location.hash) location.hash = '#/' + defaultRoute();
    render();
  }).catch(function () { /* 401 already redirected */ });

  window.addEventListener('hashchange', render);

  el('menu-btn').addEventListener('click', function () {
    el('sidebar').classList.toggle('open');
  });

  el('logout-link').addEventListener('click', function (e) {
    e.preventDefault();
    fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' })
      .finally(function () { location.href = '/'; });
  });
})();
