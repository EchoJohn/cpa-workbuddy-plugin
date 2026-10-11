#!/usr/bin/env node
/**
 * Token 用量面板「服务商」列回归脚本（Node vm + 真实 DOM 树桩，执行 dashboard.go 内联 JS）。
 *
 * 回归对象：请求明细表与维度汇总表新增 / 校准「服务商」列。
 * 背景：用户在统一用量看板中需要知道每条请求来自哪个服务商
 * （如 qoder-ai-provider、cursor-provider、qoderwork-provider、traework-provider、
 *   workbuddy-ai-provider、workbuddy-provider、zcode-provider、Antigravity 等）。
 * 历史缺陷：请求明细表 requestColumns 只有 time/model/source/session_key/...，
 * 没有任何列暴露 Dimensions.Provider；用户无法从请求明细看到服务商。
 * 期望行为（原始 provider id 口径，两张表统一）：
 *   1. 请求明细表 requestColumns 含 provider 列，表头渲染出 provider th；
 *   2. 请求行 provider 单元格显示原始 provider id（不做友好名替换）；
 *   3. 维度汇总表 provider 单元格显示原始 provider id；
 *   4. provider 列可排序（requestSortValue 有 provider 分支）；
 *   5. CSV 导出表头含服务商列，且行数据含 provider 值。
 *
 * 用法：
 *   node test/token-usage/panel_provider_column_repro.mjs [dashboard-go-path] [--legacy]
 *   --legacy：对旧版 dashboard.go 做反证，断言旧版 requestColumns 缺 provider 列。
 *
 * 退出码：0 = 全部断言通过；1 = 存在断言失败。
 */
import fs from "fs";
import path from "path";
import vm from "vm";
import crypto from "crypto";
import { fileURLToPath } from "url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const legacyMode = process.argv.includes("--legacy");
const argPath = process.argv.slice(2).find((arg) => !arg.startsWith("--"));
const dashboardPath = argPath ? path.resolve(argPath) : path.join(repoRoot, "token-usage-tracker", "usage_stats", "dashboard.go");

// 1) 读取 dashboard.go 并提取内联模板（含指纹打印，防止读错文件）。
const source = fs.readFileSync(dashboardPath, "utf8");
const md5 = crypto.createHash("md5").update(Buffer.from(source, "utf8")).digest("hex");
console.log(`[输入] ${dashboardPath}`);
console.log(`[指纹] md5=${md5} bytes=${Buffer.byteLength(source, "utf8")}`);

const templateStart = source.indexOf("const dashboardHTMLTemplate = `");
if (templateStart < 0) throw new Error("未找到 dashboardHTMLTemplate 模板");
const bodyStart = templateStart + "const dashboardHTMLTemplate = `".length;
const bodyEnd = source.indexOf("</html>", bodyStart);
if (bodyEnd < 0) throw new Error("模板缺少 </html> 结束标记");
let html = source.slice(bodyStart, bodyEnd + "</html>".length);

// 2) 复刻 Go 侧占位符替换中影响执行的两项（其余 FULL_MODE_APIKEY_* 保持注释形态即可）。
const localeDir = path.join(repoRoot, "token-usage-tracker", "usage_stats", "locales");
const publicLocales = {};
const rawLocales = {};
for (const code of ["en", "zh-CN", "zh-TW", "ru"]) {
  const raw = JSON.parse(fs.readFileSync(path.join(localeDir, `${code}.json`), "utf8"));
  rawLocales[code] = raw;
  const filtered = {};
  for (const [key, value] of Object.entries(raw)) {
    if (key.startsWith("apiKey.") || key === "table.apiKey") continue;
    filtered[key] = value;
  }
  publicLocales[code] = filtered;
}
html = html.replace("/*LOCALE_PLACEHOLDER*/", JSON.stringify(publicLocales));
html = html.replace("/*FULL_MODE_PAGE*/", "false");

// 3) 提取两个 <script> 块并做语法编译校验。
const scripts = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
if (scripts.length < 2) throw new Error(`预期至少 2 个 script 块，实际 ${scripts.length}`);
scripts.forEach((body, i) => {
  try {
    new vm.Script(body, { filename: `token-usage-dashboard-script-${i}.js` });
  } catch (e) {
    throw new Error(`第 ${i + 1} 个 script 块语法校验失败：${e && e.message ? e.message : e}`);
  }
});
console.log(`[语法] ${scripts.length} 个 script 块编译通过`);

// 4) 构建真实树状 DOM / 网络 / 定时器桩（appendChild / replaceChildren 真实挂树）。
const eventSources = [];
const intervalCalls = [];
const fetches = [];
const listeners = new Map();
const rafQueue = [];

// canvas 2d 上下文桩：真实记录 fillText 调用文本，其余方法 no-op，属性可自由读写。
function mkCanvasCtx() {
  return new Proxy(
    {
      fillText() {},
      measureText: (t) => ({ width: String(t).length * 6 }),
      createLinearGradient: () => ({ addColorStop() {} }),
      canvas: { width: 1600, height: 980 },
    },
    {
      get(target, prop) {
        if (prop in target) return target[prop];
        return () => {};
      },
      set(target, prop, value) {
        target[prop] = value;
        return true;
      },
    },
  );
}

function mkEl(id) {
  const stubParent = {
    insertBefore() {},
    appendChild() {},
    replaceChildren() {},
  };
  const el = {
    id,
    textContent: "",
    innerHTML: "",
    value: "",
    checked: false,
    disabled: false,
    hidden: true,
    open: false,
    returnValue: "",
    files: [],
    options: [],
    selectedIndex: -1,
    children: [],
    childNodes: [],
    dataset: {},
    attrs: {},
    className: "",
    parentNode: stubParent,
    style: {
      setProperty() {},
      getPropertyValue() { return ""; },
      removeProperty() {},
    },
    isConnected: true,
    offsetWidth: 900,
    offsetHeight: 330,
    clientWidth: 900,
    clientHeight: 330,
    scrollHeight: 100,
    scrollWidth: 100,
    firstChild: null,
    classList: {
      add() {}, remove() {}, toggle() {}, contains() { return false; },
    },
    addEventListener(event, handler) {
      const key = `${id}|${event}`;
      if (!listeners.has(key)) listeners.set(key, []);
      listeners.get(key).push(handler);
    },
    removeEventListener() {},
    setAttribute(k, v) { el.attrs[k] = String(v); },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(el.attrs, k) ? el.attrs[k] : null; },
    removeAttribute(k) { delete el.attrs[k]; },
    appendChild(child) {
      // document fragment 语义：挂载其子节点，而非它本身。
      if (child && child.__fragment) {
        child.children.forEach((c) => { c.parentNode = el; el.children.push(c); });
        child.children = [];
      } else {
        if (child) child.parentNode = el;
        el.children.push(child);
      }
      return child;
    },
    insertBefore(child) { return el.appendChild(child); },
    replaceChildren(...nodes) {
      el.children = [];
      nodes.forEach((n) => el.appendChild(n));
    },
    replaceWith() {}, append(...nodes) { nodes.forEach((n) => el.appendChild(n)); }, remove() {},
    insertAdjacentElement() {}, insertAdjacentHTML() {},
    focus() {}, blur() {}, click() {}, scrollIntoView() {}, dispatchEvent() {},
    showModal() {}, close() {},
    getContext() { return mkCanvasCtx(); },
    toDataURL() { return "data:image/png;base64,"; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    closest() { return null; },
    contains() { return false; },
    getBoundingClientRect() { return { width: 900, height: 330, top: 0, left: 0, right: 900, bottom: 330 }; },
    animate() { return { cancel() {} }; },
  };
  return el;
}

const els = new Map();
const getEl = (id) => {
  if (!els.has(id)) els.set(id, mkEl(id));
  return els.get(id);
};

const rangePresets = ["last_1_hour", "last_5_hours", "last_7_days", "last_30_days", "current_month"].map((mode) => {
  const btn = mkEl(`range-preset-${mode}`);
  btn.dataset.rangePreset = mode;
  return btn;
});

const document = {
  title: "",
  getElementById(id) { return getEl(id); },
  querySelector(selector) {
    if (selector === ".request-table") return getEl("__request-table");
    if (selector === ".dimension-table") return getEl("__dimension-table");
    return null;
  },
  querySelectorAll(selector) {
    if (selector === "[data-range-preset]") return rangePresets;
    return [];
  },
  createElement(tag) { return mkEl(tag); },
  createElementNS(ns, tag) { return mkEl(tag); },
  createDocumentFragment() { const f = mkEl("#fragment"); f.__fragment = true; return f; },
  addEventListener(event, handler) {
    const key = `document|${event}`;
    if (!listeners.has(key)) listeners.set(key, []);
    listeners.get(key).push(handler);
  },
  removeEventListener() {},
  body: mkEl("body"),
  documentElement: mkEl("html"),
};

// /requests 返回两条不同服务商的真实记录（provider 为原始 id）。
const requestItems = [
  {
    sequence: 2, time: "2026-10-11T11:00:00Z", provider: "qoder-ai-provider",
    model: "claude-3-5-sonnet", source: "acc-a", result: "成功", failed: false,
    ttft_ns: 100, generation_ns: 200, tps: 12.5, reasoning_effort: "high",
    input_tokens: 10, output_tokens: 20, reasoning_tokens: 0,
    cache_read_tokens: 0, cache_creation_tokens: 0, total_tokens: 30,
    cache_hit: false, session_key: "s-1",
  },
  {
    sequence: 1, time: "2026-10-11T10:00:00Z", provider: "workbuddy-provider",
    model: "gpt-5", source: "acc-b", result: "成功", failed: false,
    ttft_ns: 50, generation_ns: 80, tps: 30, reasoning_effort: "medium",
    input_tokens: 5, output_tokens: 8, reasoning_tokens: 0,
    cache_read_tokens: 0, cache_creation_tokens: 0, total_tokens: 13,
    cache_hit: false, session_key: "s-2",
  },
];

// /stats/groups 返回一条含原始 provider 的分组行。
const groupItems = [
  {
    model: "claude-3-5-sonnet", provider: "qoder-ai-provider", alias: "claude",
    source: "acc-a", executor_type: "qoder-ai", auth_type: "oauth",
    session_key: "s-1", reasoning_effort: "high",
    requests: 2, failed_requests: 0, input_tokens: 10, output_tokens: 20,
    reasoning_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0,
    total_tokens: 30, average_latency_ns: 300, average_ttft_ns: 100,
  },
];

const buildPayload = (url) => {
  if (url.includes("/preferences")) return {};
  if (url.includes("/stats/initial")) {
    return { models: [], series: [], generated_at: "2026-10-11T12:00:00Z", sources: [], last_used: "" };
  }
  if (url.includes("/stats/trends")) return { model_series: [], bucket_seconds: 3600 };
  if (url.includes("/stats/groups")) return { items: groupItems, total: groupItems.length };
  if (url.includes("/requests")) return { items: requestItems, total: requestItems.length };
  if (url.includes("/prices")) return { prices: {}, revision: 0, sync_settings: { provider_priority: [], ignored_suffixes: [], mappings: [] }, last_sync: null };
  if (url.includes("/costs")) return { summary: { requests: 0, priced_requests: 0, unpriced_requests: 0 }, models: [], series: [], price_book_revision: 0 };
  if (url.includes("/exchange-rate")) return { rate: 7.1, base: "USD", quote: "CNY" };
  return {};
};

const ctx = {
  console, Date, Math, JSON, Number, String, Object, Array, Boolean, RegExp, Error, Promise,
  isNaN, parseInt, parseFloat, encodeURIComponent, decodeURIComponent,
  TextEncoder, TextDecoder, Intl, Set, Map, WeakMap, WeakSet, AbortController,
  setTimeout, clearTimeout,
  setInterval: (fn, ms) => { intervalCalls.push(ms); return 0; },
  clearInterval: () => {},
  requestAnimationFrame: (fn) => { rafQueue.push(fn); return rafQueue.length; },
  cancelAnimationFrame: () => {},
  Blob: class {},
  URL: { createObjectURL() { return "blob:x"; }, revokeObjectURL() {} },
  URLSearchParams,
  btoa: (s) => Buffer.from(s, "binary").toString("base64"),
  atob: (s) => Buffer.from(s, "base64").toString("binary"),
  CSS: { escape: (s) => String(s) },
  EventSource: class { constructor(url) { eventSources.push(String(url)); } close() {} },
  MutationObserver: class { constructor() {} observe() {} disconnect() {} takeRecords() { return []; } },
  ResizeObserver: class { constructor() {} observe() {} disconnect() {} },
  getComputedStyle: () => ({ getPropertyValue: () => "" }),
  matchMedia: (query) => ({ matches: false, media: query, addEventListener() {}, removeListener() {}, addListener() {} }),
  confirm: () => false,
  prompt: () => "",
  addEventListener(event, handler) {
    const key = `window|${event}`;
    if (!listeners.has(key)) listeners.set(key, []);
    listeners.get(key).push(handler);
  },
  removeEventListener() {},
  document,
  navigator: { userAgent: "harness", language: "en", languages: ["en"] },
  history: { replaceState() {} },
  sessionStorage: { getItem() { return null; }, setItem() {}, removeItem() {} },
  localStorage: { getItem() { return null; }, setItem() {}, removeItem() {} },
  location: {
    pathname: "/v0/resource/plugins/token-usage-tracker/usage",
    search: "",
    hash: "",
    href: "https://cpa.luode.dev/v0/resource/plugins/token-usage-tracker/usage",
    assign() {},
    replace() {},
  },
  fetch: async (url, options) => {
    fetches.push({ url: String(url), options: options || {} });
    const payload = buildPayload(String(url));
    return {
      ok: true,
      status: 200,
      statusText: "OK",
      headers: { get: () => "application/json" },
      json: async () => payload,
      text: async () => JSON.stringify(payload),
      blob: async () => new Blob(),
    };
  },
};
ctx.window = ctx;
ctx.self = ctx;
ctx.top = ctx;
ctx.parent = ctx;
ctx.globalThis = ctx;
vm.createContext(ctx);

// 5) 按页面顺序执行两个 script 块（主题 IIFE + 主逻辑 IIFE）。
scripts.forEach((body, i) => vm.runInContext(body, ctx, { filename: `token-usage-dashboard-script-${i}.js` }));
console.log("[执行] 面板脚本已在 vm 中真实执行");

const flush = async (rounds = 8) => {
  for (let i = 0; i < rounds; i++) {
    await new Promise((r) => setTimeout(r, 30));
    while (rafQueue.length) {
      const fn = rafQueue.shift();
      try { fn(0); } catch (e) { /* rAF 回调容错 */ }
    }
  }
};

const failures = [];
function check(label, actual, expected) {
  const ok = String(actual) === String(expected);
  if (!ok) failures.push(`${label}: 期望 ${expected}，实际 ${actual}`);
  console.log(`   ${ok ? "PASS" : "FAIL"}  ${label} = ${actual}（期望 ${expected}）`);
}
function checkTrue(label, ok, detail) {
  if (!ok) failures.push(`${label}${detail ? "：" + detail : ""}`);
  console.log(`   ${ok ? "PASS" : "FAIL"}  ${label}${detail ? "（" + detail + "）" : ""}`);
}

// 递归收集节点树中所有带 dataset.column 的单元格文本。
function collectCells(node, columnKey, out = []) {
  if (!node) return out;
  if (node.dataset && node.dataset.column === columnKey) out.push(node.textContent);
  (node.children || []).forEach((child) => collectCells(child, columnKey, out));
  return out;
}
function collectHeaders(node, out = []) {
  if (!node) return out;
  if (node.dataset && node.dataset.column) out.push(node.dataset.column);
  (node.children || []).forEach((child) => collectHeaders(child, out));
  return out;
}

(async () => {
  if (legacyMode) {
    console.log("[反证] legacy 模式：验证旧版 requestColumns 不含 provider 列");
    const legacyRequestCols = source.slice(source.indexOf("var requestColumns=["), source.indexOf("var requestColumns=[") + 1400);
    checkTrue("旧版请求明细表缺 provider 列", !legacyRequestCols.includes("{key:'provider'"), "requestColumns 未含 provider");
    const legacyExport = source.slice(source.indexOf("async function exportCSV"), source.indexOf("function canvasText"));
    checkTrue("旧版 CSV 导出无 record.provider", !legacyExport.includes("record.provider"), "导出脚本未写入 provider 列");
  } else {
    console.log("[回归] 新版本：请求明细表与维度汇总表都应暴露原始服务商 id");
    await flush();

    // 断言 1：请求明细表表头含 provider 列。
    const requestHeaderKeys = collectHeaders(getEl("requestHeaders"));
    checkTrue("请求明细表表头含 provider 列", requestHeaderKeys.includes("provider"), `表头列=${JSON.stringify(requestHeaderKeys)}`);

    // 断言 2：请求行 provider 单元格显示原始 provider id（不做友好名替换）。
    const requestProviderCells = collectCells(getEl("requestRows"), "provider");
    checkTrue("请求行渲染出 provider 单元格", requestProviderCells.length === requestItems.length, `单元格数=${requestProviderCells.length}`);
    checkTrue("请求行 provider 显示 qoder-ai-provider", requestProviderCells.includes("qoder-ai-provider"), JSON.stringify(requestProviderCells));
    checkTrue("请求行 provider 显示 workbuddy-provider", requestProviderCells.includes("workbuddy-provider"), JSON.stringify(requestProviderCells));

    // 断言 3：维度汇总表 provider 单元格显示原始 provider id。
    const dimensionProviderCells = collectCells(getEl("groups"), "provider");
    checkTrue("维度汇总行 provider 显示 qoder-ai-provider", dimensionProviderCells.includes("qoder-ai-provider"), JSON.stringify(dimensionProviderCells));

    // 断言 4：provider 列可排序（源码级 requestSortValue 分支）。
    const sortFn = source.slice(source.indexOf("function requestSortValue"), source.indexOf("function requestSortValue") + 300);
    checkTrue("requestSortValue 含 provider 分支", sortFn.includes("case 'provider':return item.provider"), "分支代码已存在");

    // 断言 5：CSV 导出表头含服务商列，且行数据含 provider。
    const csvHeaders = rawLocales["en"]["export.csvHeaders"].split(",");
    check("<CSV 表头> 第 3 列为服务商", csvHeaders[2], "Provider");
    const zhHeaders = rawLocales["zh-CN"]["export.csvHeaders"].split(",");
    check("<CSV 表头 zh-CN> 第 3 列为服务商", zhHeaders[2], "提供商");
    const exportScript = source.slice(source.indexOf("async function exportCSV"), source.indexOf("function canvasText"));
    checkTrue("CSV 行数据含 record.provider", exportScript.includes("record.provider"), "导出脚本已写入 provider 列");

    // 断言 6：两张表 provider 列都渲染原始 id，未引入友好名替换。
    checkTrue("未引入 displayAuthProvider 友好名替换", !source.includes("displayAuthProvider"), "dashboard.go 保持原始 id 口径");
  }

  if (failures.length) {
    console.error(`\n[回归] FAIL（${failures.length} 项）`);
    failures.forEach((f) => console.error("   - " + f));
    process.exit(1);
  }
  console.log(legacyMode ? "\n[反证] PASS 旧版缺口被正确识别" : "\n[回归] PASS 全部断言通过");
})().catch((e) => {
  console.error("[回归] 脚本异常：" + (e && e.stack ? e.stack : e));
  process.exit(1);
});
