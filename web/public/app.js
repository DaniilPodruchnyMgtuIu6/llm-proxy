const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const state = {
  status: null,
  models: [],
  providers: [],
  pollTimer: null,
};

function apiKeyHeaders() {
  const key = localStorage.getItem("proxy_api_key") || "";
  const h = { "Content-Type": "application/json" };
  if (key) {
    h.Authorization = `Bearer ${key}`;
    h["X-API-Key"] = key;
  }
  return h;
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    ...opts,
    headers: { ...apiKeyHeaders(), ...(opts.headers || {}) },
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) {
    const msg = data?.error?.message || data?.message || res.statusText;
    throw new Error(msg);
  }
  return data;
}

function toast(msg, isErr = false) {
  const el = $("#toast");
  el.textContent = msg;
  el.classList.remove("hidden");
  el.style.background = isErr ? "#9b2c2c" : "#14201c";
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add("hidden"), 3200);
}

function showView(name) {
  $$(".panel").forEach((p) => p.classList.add("hidden"));
  const map = {
    gateway: ["view-gateway", "Настройки", "Модель по умолчанию и параметры — применяются без перезапуска Docker"],
    overview: ["view-overview", "Модели и статистика", "Остатки запросов и история использования"],
    chat: ["view-chat", "Чат", "Проверка gateway с текущими настройками"],
    keys: ["view-keys", "Ключи", "API-ключи провайдеров → runtime-хранилище"],
    setup: ["view-setup", "Первый запуск", "Добавьте ключи, чтобы gateway заработал"],
  };
  const [id, title, sub] = map[name] || map.gateway;
  $(`#${id}`).classList.remove("hidden");
  $("#view-title").textContent = title;
  $("#view-sub").textContent = sub;
  $$("#nav button").forEach((b) => b.classList.toggle("active", b.dataset.view === name));
  $("#btn-apply").classList.toggle("hidden", name !== "gateway");
  if (name === "overview") refreshOverview();
}

function keyFields(targetId, values = {}) {
  const labels = { gemini: "Gemini", groq: "Groq", openrouter: "OpenRouter" };
  const root = $(targetId);
  root.innerHTML = ["gemini", "groq", "openrouter"].map((p) => `
    <article class="card">
      <h3>${labels[p]}</h3>
      <p class="muted">Сейчас: <code>${values[p]?.masked_key || "—"}</code></p>
      <label class="field">
        <span>API key (оставьте пустым, чтобы не менять)</span>
        <input data-provider="${p}" type="password" autocomplete="off" placeholder="${values[p]?.configured ? "•••• сохранён" : "вставьте ключ"}" />
      </label>
    </article>
  `).join("");
}

function readKeyInputs(containerSel) {
  const out = {};
  $$(`${containerSel} input[data-provider]`).forEach((inp) => {
    const v = inp.value.trim();
    if (v) out[`${inp.dataset.provider}_api_key`] = v;
  });
  return {
    gemini_api_key: out.gemini_api_key,
    groq_api_key: out.groq_api_key,
    openrouter_api_key: out.openrouter_api_key,
  };
}

async function loadStatus() {
  state.status = await api("/admin/status");
  const complete = !!state.status.setup_complete;
  $("#setup-pill").textContent = complete ? "готово" : "настройка";
  $("#setup-pill").className = `pill ${complete ? "ok" : "warn"}`;
  keyFields("#setup-keys", state.status.providers);
  keyFields("#keys-form", state.status.providers);
  fillDefaults(state.status.defaults || {});
  if (!complete) showView("setup");
  return state.status;
}

function fillDefaults(d) {
  const model = d.model || "auto";
  const isAuto = !model || model === "auto";
  $("#model-mode").value = isAuto ? "auto" : "fixed";
  $("#model-id").disabled = isAuto;
  setNum("p-temperature", d.temperature);
  setNum("p-top_p", d.top_p);
  setNum("p-max_tokens", d.max_tokens);
  setNum("p-top_k", d.top_k);
  setNum("p-presence_penalty", d.presence_penalty);
  setNum("p-frequency_penalty", d.frequency_penalty);
  if (!isAuto) $("#model-id").value = model;
}

function setNum(id, v) {
  $(`#${id}`).value = v == null || v === "" ? "" : v;
}
function getNum(id) {
  const v = $(`#${id}`).value.trim();
  if (!v) return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function currentDefaultsPayload() {
  const mode = $("#model-mode").value;
  const model = mode === "auto" ? "auto" : ($("#model-id").value || "auto");
  return {
    model,
    temperature: getNum("p-temperature"),
    top_p: getNum("p-top_p"),
    max_tokens: getNum("p-max_tokens") != null ? Math.round(getNum("p-max_tokens")) : null,
    top_k: getNum("p-top_k") != null ? Math.round(getNum("p-top_k")) : null,
    presence_penalty: getNum("p-presence_penalty"),
    frequency_penalty: getNum("p-frequency_penalty"),
  };
}

function validateDefaultsPayload(d) {
  const checks = [
    ["temperature", d.temperature, 0, 2],
    ["top_p", d.top_p, 0, 1],
    ["presence_penalty", d.presence_penalty, -2, 2],
    ["frequency_penalty", d.frequency_penalty, -2, 2],
  ];
  for (const [name, v, min, max] of checks) {
    if (v == null) continue;
    if (v < min || v > max) return `${name} должен быть в диапазоне [${min}, ${max}] (сейчас ${v})`;
  }
  if (d.top_k != null && (d.top_k < 1 || d.top_k > 200 || !Number.isInteger(d.top_k))) {
    return `top_k должен быть целым числом 1–200 (сейчас ${d.top_k})`;
  }
  if (d.max_tokens != null && (d.max_tokens < 8 || d.max_tokens > 128000 || !Number.isInteger(d.max_tokens))) {
    return `max_tokens должен быть целым числом 8–128000 (сейчас ${d.max_tokens}). Значение 1 даёт пустой ответ.`;
  }
  return null;
}

async function loadModels() {
  try {
    const data = await api("/v1/models");
    state.models = data.data || [];
    const sel = $("#model-id");
    const cur = sel.value;
    sel.innerHTML = state.models.map((m) =>
      `<option value="${escapeAttr(m.id)}">${escapeHtml(m.id)} · №${m.rank} · ${m.provider}</option>`
    ).join("");
    if (cur) sel.value = cur;
    else if (state.status?.defaults?.model && state.status.defaults.model !== "auto") {
      sel.value = state.status.defaults.model;
    }
  } catch {
    state.models = [];
  }
}

async function refreshOverview() {
  const from = $("#stats-from").value;
  const to = $("#stats-to").value;
  const q = new URLSearchParams();
  if (from) q.set("from", from);
  if (to) q.set("to", to);
  try {
    const [s, prov, models] = await Promise.all([
      api(`/v1/stats/summary?${q}`),
      api("/v1/providers"),
      api("/v1/models"),
    ]);
    state.providers = prov.data || [];
    state.models = models.data || [];

    $("#stats-summary").innerHTML = [
      ["Успешные ответы", s.total_success, "kpi-ok"],
      ["Ошибки", s.total_errors, "kpi-bad"],
      ["Отказы по квоте", s.total_rate_limit, "kpi-warn"],
      ["Токены в / из", `${s.tokens_in ?? 0} / ${s.tokens_out ?? 0}`, ""],
    ].map(([k, v, cls]) =>
      `<article class="kpi ${cls}"><span>${k}</span><b>${v ?? 0}</b></article>`
    ).join("");

    $("#provider-cards").innerHTML = state.providers.map((p) => {
      const qq = p.quota || {};
      const left = qq.remaining_rpd;
      const used = qq.used_rpd ?? 0;
      const cap = qq.rpd != null ? qq.rpd : (left != null ? left + used : null);
      const pct = cap && cap > 0 && left != null ? Math.max(0, Math.min(100, Math.round((left / cap) * 100))) : null;
      const barCls = pct == null ? "" : pct <= 5 ? "is-bad" : pct <= 20 ? "is-warn" : "is-ok";
      const scopeHint = qq.scope === "shared_pool"
        ? "общий пул на все free-модели"
        : qq.scope === "per_model_aggregate"
          ? "сумма остатков по моделям"
          : "осталось на сегодня";
      const health = p.healthy
        ? `<span class="pill ok">в порядке</span>`
        : `<span class="pill bad">проблемы</span>`;
      return `<article class="provider-card">
        <header>
          <div>
            <strong>${escapeHtml(p.name || p.id)}</strong>
            <small>${escapeHtml(p.id)} · моделей: ${p.models ?? 0}</small>
          </div>
          ${health}
        </header>
        <div class="provider-metric">
          <b>${left != null ? left : "—"}</b>
          <span>осталось сегодня</span>
        </div>
        <div class="meter ${barCls}" aria-hidden="true">
          <i style="width:${pct != null ? pct : 0}%"></i>
        </div>
        <footer>
          <span>${scopeHint}</span>
          <span>использовано: ${used != null ? used : "—"}${cap != null ? ` / ${cap}` : ""}</span>
        </footer>
      </article>`;
    }).join("") || `<p class="muted">Нет провайдеров — добавьте ключи.</p>`;

    $("#quota-table tbody").innerHTML = state.models.slice(0, 60).map((m) => {
      const qq = m.quota || {};
      return `<tr class="${remainClass(qq.remaining_rpd)}">
        <td class="num col-rank">${m.rank ?? ""}</td>
        <td class="col-model"><code title="${escapeAttr(m.id)}">${escapeHtml(m.id)}</code></td>
        <td class="col-prov">${escapeHtml(m.provider)}</td>
        <td class="num remain">${fmt(qq.remaining_rpd)}</td>
        <td class="num">${fmt(qq.used_rpd)}</td>
      </tr>`;
    }).join("") || `<tr><td colspan="5" class="muted">Нет моделей</td></tr>`;

    $("#stats-by-provider tbody").innerHTML = (s.by_provider || []).map((r) =>
      `<tr>
        <td>${escapeHtml(r.provider)}</td>
        <td class="num">${r.success_count}</td>
        <td class="num">${r.error_count}</td>
        <td class="num">${r.rate_limit_count}</td>
      </tr>`
    ).join("") || `<tr><td colspan="4" class="muted">Пока нет данных</td></tr>`;

    $("#stats-by-model tbody").innerHTML = (s.by_model || []).map((r) =>
      `<tr>
        <td class="col-model"><code title="${escapeAttr(r.model)}">${escapeHtml(r.model)}</code></td>
        <td class="col-prov">${escapeHtml(r.provider)}</td>
        <td class="num">${r.success_count}</td>
        <td class="num">${r.rate_limit_count}</td>
      </tr>`
    ).join("") || `<tr><td colspan="4" class="muted">Пока нет данных</td></tr>`;
  } catch (e) {
    toast(e.message, true);
  }
}

function remainClass(rpd) {
  if (rpd == null) return "";
  if (rpd <= 0) return "remain-bad";
  if (rpd <= 5) return "remain-warn";
  return "";
}

function fmt(v) { return v == null ? "—" : v; }
function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
function escapeAttr(s) { return escapeHtml(s).replace(/`/g, ""); }

function updateApiSnippets() {
  const base = `${location.origin}`;
  $("#api-base").textContent = base;
  const d = currentDefaultsPayload();
  const body = {
    messages: [{ role: "user", content: "ping" }],
  };
  if (d.model && d.model !== "auto") body.model = d.model;
  if (d.temperature != null) body.temperature = d.temperature;
  if (d.max_tokens != null) body.max_tokens = d.max_tokens;
  $("#api-example").textContent =
`curl -s ${base}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(body)}'`;
}

async function saveKeys(fromSetup) {
  const payload = readKeyInputs(fromSetup ? "#setup-keys" : "#keys-form");
  const body = { apply: true };
  if (payload.gemini_api_key) body.gemini_api_key = payload.gemini_api_key;
  if (payload.groq_api_key) body.groq_api_key = payload.groq_api_key;
  if (payload.openrouter_api_key) body.openrouter_api_key = payload.openrouter_api_key;
  if (!body.gemini_api_key && !body.groq_api_key && !body.openrouter_api_key) {
    toast("Введите хотя бы один новый ключ", true);
    return;
  }
  await api("/admin/keys", { method: "PUT", body: JSON.stringify(body) });
  toast("Ключи сохранены и применены");
  await loadStatus();
  await loadModels();
  if (fromSetup) showView("gateway");
}

async function saveDefaults() {
  const body = currentDefaultsPayload();
  const verr = validateDefaultsPayload(body);
  if (verr) {
    toast(verr, true);
    throw new Error(verr);
  }
  await api("/admin/defaults", { method: "PUT", body: JSON.stringify(body) });
  toast("Настройки сохранены");
  updateApiSnippets();
}

async function applyAll() {
  await saveDefaults();
  await api("/admin/apply", { method: "POST", body: "{}" });
  toast("Конфигурация применена");
  await loadStatus();
  await loadModels();
  updateApiSnippets();
}

async function sendChat(e) {
  e.preventDefault();
  const text = $("#chat-input").value.trim();
  if (!text) return;
  const log = $("#chat-log");
  const meta = $("#chat-meta");
  log.classList.remove("hidden");
  log.insertAdjacentHTML("beforeend", `<div class="msg user">${escapeHtml(text)}</div>`);
  $("#chat-input").value = "";
  const d = currentDefaultsPayload();
  const verr = validateDefaultsPayload(d);
  if (verr) {
    toast(verr, true);
    return;
  }
  const body = { messages: [{ role: "user", content: text }] };
  if (d.model && d.model !== "auto") body.model = d.model;
  ["temperature", "top_p", "max_tokens", "top_k", "presence_penalty", "frequency_penalty"].forEach((k) => {
    if (d[k] != null) body[k] = d[k];
  });
  try {
    const res = await fetch("/v1/chat/completions", {
      method: "POST",
      headers: apiKeyHeaders(),
      body: JSON.stringify(body),
    });
    const raw = await res.text();
    let data; try { data = JSON.parse(raw); } catch { data = { raw }; }
    const content = data?.choices?.[0]?.message?.content || data?.error?.message || raw;
    log.insertAdjacentHTML("beforeend", `<div class="msg bot">${escapeHtml(content)}</div>`);
    const model = res.headers.get("X-LLM-Proxy-Model") || "?";
    const provider = res.headers.get("X-LLM-Proxy-Provider") || "?";
    const attempts = res.headers.get("X-LLM-Proxy-Attempts") || "?";
    meta.classList.remove("hidden");
    meta.textContent =
      `статус ${res.status} · модель ${model} · провайдер ${provider} · попыток ${attempts}`;
    log.scrollTop = log.scrollHeight;
  } catch (err) {
    toast(err.message, true);
  }
}

function initDates() {
  const to = new Date();
  const from = new Date();
  from.setDate(to.getDate() - 7);
  $("#stats-to").value = to.toISOString().slice(0, 10);
  $("#stats-from").value = from.toISOString().slice(0, 10);
}

function bind() {
  $$("#nav button").forEach((b) => b.addEventListener("click", () => showView(b.dataset.view)));
  $("#btn-apply").addEventListener("click", () => applyAll().catch((e) => toast(e.message, true)));
  $("#btn-setup-save").addEventListener("click", () => saveKeys(true).catch((e) => toast(e.message, true)));
  $("#btn-keys-save").addEventListener("click", () => saveKeys(false).catch((e) => toast(e.message, true)));
  $("#btn-refresh-overview").addEventListener("click", () => refreshOverview());
  $("#chat-form").addEventListener("submit", sendChat);
  $("#model-mode").addEventListener("change", () => {
    $("#model-id").disabled = $("#model-mode").value === "auto";
    updateApiSnippets();
  });
  ["model-id", "p-temperature", "p-top_p", "p-max_tokens", "p-top_k"].forEach((id) => {
    $(`#${id}`).addEventListener("input", updateApiSnippets);
  });
  $("#ui-proxy-key").value = localStorage.getItem("proxy_api_key") || "";
  $("#ui-proxy-key").addEventListener("change", (e) => {
    localStorage.setItem("proxy_api_key", e.target.value.trim());
    toast("Ключ UI сохранён в браузере");
  });
}

async function boot() {
  bind();
  initDates();
  updateApiSnippets();
  try {
    await loadStatus();
    await loadModels();
    fillDefaults(state.status.defaults || {});
    updateApiSnippets();
    if (state.status.setup_complete) showView("gateway");
  } catch (e) {
    toast(`Не удалось связаться с proxy: ${e.message}`, true);
    showView("setup");
  }
  state.pollTimer = setInterval(() => {
    const overviewVisible = !$("#view-overview").classList.contains("hidden");
    if (overviewVisible) refreshOverview();
  }, 5000);
}

boot();
