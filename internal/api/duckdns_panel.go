package api

import "strings"

const duckDNSManagementPanelHTML = `<style id="cliproxy-duckdns-style">
#cliproxy-duckdns-root{box-sizing:border-box;width:100%;margin:0 0 18px;color:var(--text-color,#2d2a26);font:14px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
#cliproxy-duckdns-root *{box-sizing:border-box}
#cliproxy-duckdns-root[hidden]{display:none!important}
#cliproxy-duckdns-card{border:1px solid var(--border-color,#e3e1db);background:var(--card-bg,#fff);border-radius:8px;padding:16px;box-shadow:0 8px 28px rgba(15,23,42,.06)}
#cliproxy-duckdns-head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px}
#cliproxy-duckdns-title{font-size:15px;font-weight:700}
#cliproxy-duckdns-fields{display:grid;grid-template-columns:minmax(150px,1fr) minmax(190px,1.25fr) minmax(150px,1fr);gap:10px}
#cliproxy-duckdns-root label{display:grid;gap:5px;min-width:0;color:var(--text-muted,#6b7280);font-size:12px;font-weight:600}
#cliproxy-duckdns-root input{width:100%;height:36px;border:1px solid var(--border-color,#d8d4cc);border-radius:7px;padding:7px 9px;background:var(--input-bg,#fff);color:var(--text-color,#2d2a26);font:inherit;outline:none}
#cliproxy-duckdns-root input:focus{border-color:#0f766e;box-shadow:0 0 0 3px rgba(15,118,110,.12)}
#cliproxy-duckdns-actions{display:flex;align-items:center;justify-content:flex-end;gap:8px;margin-top:12px}
#cliproxy-duckdns-actions button{height:34px;border:1px solid var(--border-color,#d8d4cc);border-radius:7px;padding:0 12px;background:var(--button-bg,#fff);color:var(--text-color,#2d2a26);font:inherit;font-weight:700;cursor:pointer}
#cliproxy-duckdns-actions button:last-child{border-color:#0f766e;background:#0f766e;color:#fff}
#cliproxy-duckdns-status{min-height:18px;margin-top:9px;color:var(--text-muted,#6b7280);font-size:12px;overflow-wrap:anywhere}
#cliproxy-duckdns-status[data-kind="error"]{color:#b91c1c}
#cliproxy-duckdns-status[data-kind="ok"]{color:#047857}
@media (max-width:780px){#cliproxy-duckdns-fields{grid-template-columns:1fr}#cliproxy-duckdns-actions{justify-content:stretch;flex-wrap:wrap}#cliproxy-duckdns-actions button{flex:1 1 120px}}
</style>
<section id="cliproxy-duckdns-root" aria-label="DuckDNS" hidden>
  <div id="cliproxy-duckdns-card">
    <div id="cliproxy-duckdns-head">
      <div id="cliproxy-duckdns-title">DuckDNS</div>
    </div>
    <div id="cliproxy-duckdns-fields">
      <label>Domain<input id="cliproxy-duckdns-domain" type="text" autocomplete="off" placeholder="iscdx"></label>
      <label>Token<input id="cliproxy-duckdns-token" type="password" autocomplete="off" placeholder="unchanged"></label>
      <label>Public IP<input id="cliproxy-duckdns-ip" type="text" inputmode="numeric" autocomplete="off" placeholder="auto"></label>
    </div>
    <div id="cliproxy-duckdns-actions">
      <button id="cliproxy-duckdns-auto-ip" type="button">Auto IP</button>
      <button id="cliproxy-duckdns-save" type="button">Save</button>
      <button id="cliproxy-duckdns-update" type="button">Update IP</button>
    </div>
    <div id="cliproxy-duckdns-status" role="status"></div>
  </div>
</section>
<script id="cliproxy-duckdns-script">
(function(){
  var defaultDuckDNSDomain = "iscdx";
  var root = document.getElementById("cliproxy-duckdns-root");
  if (!root || root.dataset.ready === "true") return;
  root.dataset.ready = "true";
  var domainInput = document.getElementById("cliproxy-duckdns-domain");
  var tokenInput = document.getElementById("cliproxy-duckdns-token");
  var ipInput = document.getElementById("cliproxy-duckdns-ip");
  var status = document.getElementById("cliproxy-duckdns-status");
  var autoIP = document.getElementById("cliproxy-duckdns-auto-ip");
  var save = document.getElementById("cliproxy-duckdns-save");
  var update = document.getElementById("cliproxy-duckdns-update");
  var storagePrefix = "enc::v1::";
  var storageKeyBase = "cli-proxy-api-webui::secure-storage";
  var loaded = false;

  function bytes(text) {
    return new TextEncoder().encode(text);
  }
  function text(data) {
    return new TextDecoder().decode(data);
  }
  function obfuscationKey() {
    return bytes(storageKeyBase + "|" + window.location.host + "|" + navigator.userAgent);
  }
  function xor(data, key) {
    var out = new Uint8Array(data.length);
    for (var i = 0; i < data.length; i++) out[i] = data[i] ^ key[i % key.length];
    return out;
  }
  function fromBase64(value) {
    var raw = atob(value);
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }
  function decodeStoredValue(value) {
    if (!value) return "";
    var decoded = value;
    if (value.indexOf(storagePrefix) === 0) {
      decoded = text(xor(fromBase64(value.slice(storagePrefix.length)), obfuscationKey()));
    }
    try {
      var parsed = JSON.parse(decoded);
      return typeof parsed === "string" ? parsed : "";
    } catch (_) {
      return decoded;
    }
  }
  function managementKey() {
    try {
      return decodeStoredValue(localStorage.getItem("managementKey")) || localStorage.getItem("managementKey") || "";
    } catch (_) {
      return "";
    }
  }
  function setStatus(message, kind) {
    status.textContent = message || "";
    if (kind) status.dataset.kind = kind; else status.removeAttribute("data-kind");
  }
  function headers(json) {
    var key = managementKey();
    var out = {};
    if (json) out["Content-Type"] = "application/json";
    if (key) out.Authorization = "Bearer " + key;
    return out;
  }
  async function api(path, options) {
    options = options || {};
    var request = {method: options.method || "GET", headers: headers(options.json)};
    if (options.json) request.body = JSON.stringify(options.json);
    var res = await fetch(path, request);
    var raw = await res.text();
    var data = {};
    if (raw) {
      try { data = JSON.parse(raw); } catch (_) { data = {message: raw}; }
    }
    if (!res.ok) throw new Error(data.message || data.error || ("HTTP " + res.status));
    return data;
  }
  function ensureMount() {
    if (!managementKey()) {
      root.hidden = true;
      return false;
    }
    var target = document.querySelector(".main-content") || document.querySelector(".content");
    if (!target) {
      root.hidden = true;
      return false;
    }
    if (root.parentNode !== target) target.insertBefore(root, target.firstChild);
    root.hidden = false;
    return true;
  }
  async function refreshPublicIP(quiet) {
    try {
      if (!quiet) setStatus("Checking public IP...", "");
      var data = await api("/v0/management/duckdns/public-ip");
      ipInput.value = data.ip || "";
      if (!quiet) setStatus("Detected " + (data.ip || ""), "ok");
    } catch (err) {
      if (!quiet) setStatus(err.message || "Public IP lookup failed", "error");
    }
  }
  async function load() {
    if (!ensureMount() || loaded) return;
    loaded = true;
    try {
      var data = await api("/v0/management/duckdns");
      domainInput.value = data.domain || defaultDuckDNSDomain;
      ipInput.value = data.ip || "";
      tokenInput.value = "";
      tokenInput.placeholder = data.token_set ? "configured" : "unchanged";
      setStatus(data.hostname || (defaultDuckDNSDomain + ".duckdns.org"), "ok");
      await refreshPublicIP(true);
    } catch (err) {
      loaded = false;
      setStatus(err.message || "DuckDNS load failed", "error");
    }
  }
  function payload() {
    var domain = (domainInput.value || "").trim() || defaultDuckDNSDomain;
    var body = {domain: domain, ip: ipInput.value || ""};
    if ((tokenInput.value || "").trim()) body.token = tokenInput.value;
    return body;
  }
  async function saveConfig() {
    try {
      var data = await api("/v0/management/duckdns", {method: "PATCH", json: payload()});
      domainInput.value = data.domain || defaultDuckDNSDomain;
      tokenInput.value = "";
      tokenInput.placeholder = data.token_set ? "configured" : "unchanged";
      setStatus("Saved " + (data.hostname || data.domain || defaultDuckDNSDomain), "ok");
    } catch (err) {
      setStatus(err.message || "Save failed", "error");
    }
  }
  async function updateDNS() {
    try {
      var data = await api("/v0/management/duckdns/update", {method: "POST", json: payload()});
      if (data.ipv4) ipInput.value = data.ipv4;
      setStatus((data.status || "OK") + (data.ipv4 ? " " + data.ipv4 : ""), "ok");
    } catch (err) {
      setStatus(err.message || "Update failed", "error");
    }
  }
  autoIP.addEventListener("click", function(){ refreshPublicIP(false); });
  save.addEventListener("click", saveConfig);
  update.addEventListener("click", updateDNS);

  var observer = new MutationObserver(function(){ load(); });
  observer.observe(document.documentElement, {childList:true, subtree:true});
  window.addEventListener("storage", function(e){ if (e.key === "managementKey") load(); });
  setTimeout(load, 250);
  setTimeout(load, 1000);
})();
</script>`

func injectDuckDNSManagementPanel(data []byte) []byte {
	html := string(data)
	if strings.Contains(html, "cliproxy-duckdns-root") {
		return data
	}
	lower := strings.ToLower(html)
	idx := strings.LastIndex(lower, "</body>")
	if idx < 0 {
		return []byte(html + duckDNSManagementPanelHTML)
	}
	return []byte(html[:idx] + duckDNSManagementPanelHTML + html[idx:])
}
