const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const state = {
  status: null,
  models: [],
  providers: [],
  presets: [],
  editingSlug: null,
  isNew: false,
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
    presets: ["view-presets", "Пресеты", "Именованные конфиги model + sampling с готовым URL"],
    gateway: ["view-gateway", "Настройки", "Базовый URL и ключ доступа UI"],
    overview: ["view-overview", "Модели и статистика", "Остатки запросов и история использования"],
    chat: ["view-chat", "Чат", "Проверка gateway через выбранный пресет"],
    keys: ["view-keys", "Ключи", "API-ключи провайдеров → runtime-хранилище"],
    setup: ["view-setup", "Первый запуск", "Добавьте ключи, чтобы gateway заработал"],
  };
  const [id, title, sub] = map[name] || map.presets;
  $(`#${id}`).classList.remove("hidden");
  $("#view-title").textContent = title;
  $("#view-sub").textContent = sub;
  $$("#nav button").forEach((b) => b.classList.toggle("active", b.dataset.view === name));
  $("#btn-apply").classList.add("hidden");
  if (name === "overview") refreshOverview();
  if (name === "presets") loadPresets().catch((e) => toast(e.message, true));
  if (name === "chat") fillChatPresetSelect();
  if (name === "gateway") updateApiSnippets();
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
  if (!complete) showView("setup");
  return state.status;
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

function validateSampling(d) {
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
    const sel = $("#ps-model-id");
    const cur = sel.value;
    sel.innerHTML = state.models.map((m) =>
      `<option value="${escapeAttr(m.id)}">${escapeHtml(m.id)} · №${m.rank} · ${m.provider}</option>`
    ).join("");
    if (cur) sel.value = cur;
  } catch {
    state.models = [];
  }
}

async function loadPresets() {
  const data = await api("/admin/presets");
  state.presets = data.data || [];
  renderPresetList();
  fillChatPresetSelect();
  if (state.isNew) return;
  const want = state.editingSlug || "default";
  const found = state.presets.find((p) => p.slug === want) || state.presets[0];
  if (found) selectPreset(found.slug);
}

function renderPresetList() {
  $("#preset-list").innerHTML = state.presets.map((p) => {
    const active = !state.isNew && p.slug === state.editingSlug ? "active" : "";
    const badge = p.is_system ? `<span class="pill ok">system</span>` : "";
    return `<li>
      <button type="button" class="preset-item ${active}" data-slug="${escapeAttr(p.slug)}">
        <span>
          <strong>${escapeHtml(p.title || p.slug)}</strong>
          <small><code>${escapeHtml(p.slug)}</code> · ${escapeHtml(p.model || "auto")}</small>
        </span>
        ${badge}
      </button>
    </li>`;
  }).join("") || `<li class="muted">Нет пресетов</li>`;
  $$("#preset-list .preset-item").forEach((btn) => {
    btn.addEventListener("click", () => selectPreset(btn.dataset.slug));
  });
}

function fillChatPresetSelect() {
  const sel = $("#chat-preset");
  const cur = sel.value || localStorage.getItem("chat_preset") || "default";
  sel.innerHTML = state.presets.map((p) =>
    `<option value="${escapeAttr(p.slug)}">${escapeHtml(p.title || p.slug)} (${escapeHtml(p.slug)})</option>`
  ).join("");
  if ([...sel.options].some((o) => o.value === cur)) sel.value = cur;
}

function selectPreset(slug) {
  state.isNew = false;
  state.editingSlug = slug;
  const p = state.presets.find((x) => x.slug === slug);
  if (!p) return;
  $("#preset-form-title").textContent = p.is_system ? "Системный пресет" : "Редактирование";
  $("#ps-slug").value = p.slug;
  $("#ps-slug").disabled = true;
  $("#ps-title").value = p.title || "";
  $("#ps-description").value = p.description || "";
  const isAuto = !p.model || p.model === "auto";
  $("#ps-model-mode").value = isAuto ? "auto" : "fixed";
  $("#ps-model-id").disabled = isAuto;
  if (!isAuto) $("#ps-model-id").value = p.model;
  setNum("ps-temperature", p.temperature);
  setNum("ps-top_p", p.top_p);
  setNum("ps-max_tokens", p.max_tokens);
  setNum("ps-top_k", p.top_k);
  setNum("ps-presence_penalty", p.presence_penalty);
  setNum("ps-frequency_penalty", p.frequency_penalty);
  $("#btn-preset-save").textContent = "Сохранить пресет";
  $("#btn-preset-delete").classList.toggle("hidden", !!p.is_system);
  renderPresetList();
  updatePresetEndpoint(p.slug);
}

function startNewPreset() {
  state.isNew = true;
  state.editingSlug = null;
  $("#preset-form-title").textContent = "Новый пресет";
  $("#ps-slug").value = "";
  $("#ps-slug").disabled = false;
  $("#ps-title").value = "";
  $("#ps-description").value = "";
  $("#ps-model-mode").value = "auto";
  $("#ps-model-id").disabled = true;
  ["ps-temperature", "ps-top_p", "ps-max_tokens", "ps-top_k", "ps-presence_penalty", "ps-frequency_penalty"]
    .forEach((id) => setNum(id, null));
  $("#btn-preset-save").textContent = "Создать пресет";
  $("#btn-preset-delete").classList.add("hidden");
  $("#preset-endpoint").classList.add("hidden");
  renderPresetList();
}

function currentPresetPayload() {
  const mode = $("#ps-model-mode").value;
  const model = mode === "auto" ? "auto" : ($("#ps-model-id").value || "auto");
  return {
    slug: $("#ps-slug").value.trim().toLowerCase(),
    title: $("#ps-title").value.trim(),
    description: $("#ps-description").value.trim(),
    model,
    temperature: getNum("ps-temperature"),
    top_p: getNum("ps-top_p"),
    max_tokens: getNum("ps-max_tokens") != null ? Math.round(getNum("ps-max_tokens")) : null,
    top_k: getNum("ps-top_k") != null ? Math.round(getNum("ps-top_k")) : null,
    presence_penalty: getNum("ps-presence_penalty"),
    frequency_penalty: getNum("ps-frequency_penalty"),
  };
}

function updatePresetEndpoint(slug) {
  const base = location.origin;
  const path = `/v1/p/${slug}/chat/completions`;
  const url = `${base}${path}`;
  const curl = `curl -s ${url} \\\n  -H "Content-Type: application/json" \\\n  -d '{"messages":[{"role":"user","content":"ping"}]}'`;
  $("#preset-url").textContent = url;
  $("#preset-curl").textContent = curl;
  $("#preset-endpoint").classList.remove("hidden");
}

function updateApiSnippets() {
  const base = location.origin;
  $("#api-base").textContent = base;
  const slug = state.editingSlug || state.presets[0]?.slug || "default";
  $("#api-example").textContent =
`curl -s ${base}/v1/p/${slug}/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{"messages":[{"role":"user","content":"ping"}]}'`;
}

async function savePreset() {
  const body = currentPresetPayload();
  const verr = validateSampling(body);
  if (verr) {
    toast(verr, true);
    return;
  }
  if (state.isNew) {
    if (!/^[a-z0-9][a-z0-9_-]{0,63}$/.test(body.slug)) {
      toast("slug: латиница/цифры/_/- , начинается с буквы или цифры", true);
      return;
    }
    const res = await api("/admin/presets", { method: "POST", body: JSON.stringify(body) });
    toast("Пресет создан");
    state.isNew = false;
    state.editingSlug = res.preset?.slug || body.slug;
  } else {
    const slug = state.editingSlug;
    const res = await api(`/admin/presets/${encodeURIComponent(slug)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
    toast("Пресет сохранён");
    state.editingSlug = res.preset?.slug || slug;
  }
  await loadPresets();
  updateApiSnippets();
}

async function deletePreset() {
  const slug = state.editingSlug;
  if (!slug || !confirm(`Удалить пресет «${slug}»?`)) return;
  await api(`/admin/presets/${encodeURIComponent(slug)}`, { method: "DELETE" });
  toast("Пресет удалён");
  state.editingSlug = "default";
  await loadPresets();
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
  if (fromSetup) showView("presets");
}

async function sendChat(e) {
  e.preventDefault();
  const text = $("#chat-input").value.trim();
  if (!text) return;
  const slug = $("#chat-preset").value || "default";
  localStorage.setItem("chat_preset", slug);
  const log = $("#chat-log");
  const meta = $("#chat-meta");
  log.classList.remove("hidden");
  log.insertAdjacentHTML("beforeend", `<div class="msg user">${escapeHtml(text)}</div>`);
  $("#chat-input").value = "";
  const body = { messages: [{ role: "user", content: text }] };
  try {
    const res = await fetch(`/v1/p/${encodeURIComponent(slug)}/chat/completions`, {
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
    const preset = res.headers.get("X-LLM-Proxy-Preset") || slug;
    meta.classList.remove("hidden");
    meta.textContent =
      `статус ${res.status} · пресет ${preset} · модель ${model} · провайдер ${provider} · попыток ${attempts}`;
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
  $("#btn-setup-save").addEventListener("click", () => saveKeys(true).catch((e) => toast(e.message, true)));
  $("#btn-keys-save").addEventListener("click", () => saveKeys(false).catch((e) => toast(e.message, true)));
  $("#btn-refresh-overview").addEventListener("click", () => refreshOverview());
  $("#chat-form").addEventListener("submit", sendChat);
  $("#btn-preset-new").addEventListener("click", startNewPreset);
  $("#btn-preset-save").addEventListener("click", () => savePreset().catch((e) => toast(e.message, true)));
  $("#btn-preset-delete").addEventListener("click", () => deletePreset().catch((e) => toast(e.message, true)));
  $("#btn-preset-copy").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText($("#preset-curl").textContent);
      toast("curl скопирован");
    } catch {
      toast("Не удалось скопировать", true);
    }
  });
  $("#ps-model-mode").addEventListener("change", () => {
    $("#ps-model-id").disabled = $("#ps-model-mode").value === "auto";
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
    await loadPresets();
    updateApiSnippets();
    if (state.status.setup_complete) showView("presets");
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
