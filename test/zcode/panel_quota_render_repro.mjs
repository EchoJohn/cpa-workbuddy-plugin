#!/usr/bin/env node
/**
 * ZCode 面板额度渲染回归脚本（Node vm + DOM 桩，真实执行 panel.html 内联 JS）。
 *
 * 回归对象：账号面板的额度可视化链路（0.1.12 参照 workbuddy 新增）。
 * 覆盖断言：
 *   1. 两个 script 块语法可编译；
 *   2. 无额度缓存的账号卡片渲染「加载中…」占位而非报错；
 *   3. 有额度快照的账号渲染进度条、计划名徽章与合计；
 *   4. 汇总卡按「额度已获取 / 未知」口径统计，合计等于各账号 total_grant 之和；
 *   5. 额度排序（quota_asc / quota_desc）按 total_grant 生效，未知额度排最少端；
 *   6. 筛选标签计数与状态口径一致。
 *
 * 用法：node test/zcode/panel_quota_render_repro.mjs
 * 退出码：0 = 全部断言通过；1 = 存在断言失败。
 */
import fs from "fs";
import path from "path";
import vm from "vm";
import { fileURLToPath } from "url";

const plugin = "zcode";
const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const panelPath = path.join(repoRoot, plugin, "panel.html");

// 夹具：3 个账号 —— A 有额度(300万+500万)、B 有额度(100万)、C 无额度缓存。
const ACCOUNTS = [
  {
    auth_id: "zcode-aaa", auth_index: "zcode-aaa", label: "账号A", disabled: false,
    test_failed: false, cooling_down: false, created_at: "2026-10-01T00:00:00Z",
    models: ["GLM-5.3", "GLM-5.3-Flash"],
    quota: {
      plan_name: "ZCode Start Plan", plan_id: "p1", plan_status: "active",
      total_grant: 8000000, fetched_at: "2026-10-12T05:00:00Z",
      entitlements: [
        { show_name: "GLM-5.3", grant_units: 3000000, period: "daily", capability: "model:glm-5.3" },
        { show_name: "GLM-5.3-Flash", grant_units: 5000000, period: "daily", capability: "model:glm-5.3-flash" },
      ],
    },
  },
  {
    auth_id: "zcode-bbb", auth_index: "zcode-bbb", label: "账号B", disabled: true,
    test_failed: false, cooling_down: false, created_at: "2026-10-02T00:00:00Z",
    models: ["GLM-5.2"],
    quota: {
      plan_name: "ZCode Start Plan", plan_id: "p1", plan_status: "active",
      total_grant: 1000000, fetched_at: "2026-10-12T05:00:00Z",
      entitlements: [{ show_name: "GLM-5.2", grant_units: 1000000, period: "daily", capability: "model:glm-5.2" }],
    },
  },
  {
    auth_id: "zcode-ccc", auth_index: "zcode-ccc", label: "账号C", disabled: false,
    test_failed: true, cooling_down: false, created_at: "2026-10-03T00:00:00Z",
    models: ["GLM-5.3"],
    quota: null,
  },
];

function loadPanelContext() {
  const html = fs.readFileSync(panelPath, "utf8");
  const bodies = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
  const main = bodies[bodies.length - 1];
  if (!main) throw new Error(`未在 ${panelPath} 中解析到内联脚本`);
  bodies.forEach((body, i) => {
    try {
      new vm.Script(body, { filename: `${plugin}-panel-script-${i}.js` });
    } catch (e) {
      throw new Error(`第 ${i + 1} 个 script 块语法校验失败：${e && e.message ? e.message : e}`);
    }
  });
  console.log(`[语法] ${bodies.length} 个 script 块编译通过`);

  const mkEl = (id) => ({
    id, textContent: "", innerHTML: "", value: "", checked: false, disabled: false,
    dataset: {}, style: {}, firstChild: null,
    // toast() 依赖 box.children.length 与 box.firstChild.remove() 做上限裁剪。
    children: [],
    classList: { add() {}, remove() {}, toggle() {}, contains() { return false; } },
    addEventListener() {}, removeEventListener() {}, focus() {}, remove() {},
    replaceWith() {}, appendChild() {}, insertBefore() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    setAttribute() {}, getAttribute() { return null; },
  });

  const els = new Map();
  const document = {
    getElementById(id) {
      if (!els.has(id)) els.set(id, mkEl(id));
      return els.get(id);
    },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    createElement(tag) {
      const e = mkEl(tag);
      // toast() 会 el.querySelector(".t")/.d 取子节点，桩需返回可用元素。
      e.querySelector = () => mkEl("child");
      e.firstChild = { remove() {}, classList: { add() {}, remove() {} } };
      return e;
    },
    addEventListener() {}, removeEventListener() {},
    body: { appendChild() {}, removeChild() {} },
    documentElement: mkEl("html"),
  };

  let payload;
  const ctx = {
    console, Date, Math, JSON, Number, String, Object, Array, Boolean, RegExp, Error, Promise,
    isNaN, parseInt, parseFloat, encodeURIComponent, decodeURIComponent,
    TextEncoder, TextDecoder, setTimeout, clearTimeout,
    setInterval: () => 0, clearInterval: () => 0,
    requestAnimationFrame: (f) => f(),
    Blob: class {},
    URL: { createObjectURL() { return "blob:x"; }, revokeObjectURL() {} },
    URLSearchParams,
    btoa: (s) => Buffer.from(s, "binary").toString("base64"),
    atob: (s) => Buffer.from(s, "base64").toString("binary"),
    CSS: { escape: (s) => String(s) },
    document,
    navigator: { userAgent: "harness" },
    history: { replaceState() {} },
    sessionStorage: { getItem() { return "KEY"; }, setItem() {}, removeItem() {} },
    localStorage: { getItem() { return null; }, setItem() {} },
    location: { host: "cpa.luode.dev", search: "", href: "https://cpa.luode.dev/panel" },
    fetch: async (url) => {
      const path = String(url);
      // payload 由 setPayload 注入（测试在 loadPanelContext 之后调用）；boot 阶段的
      // enterPanel 会先打一次 /accounts，此时 payload 仍为空，返回空账号列表兜底，
      // 避免桩环境下未捕获异常掩盖真实断言结果。
      let body;
      if (path.includes("/accounts")) body = payload || { accounts: [], count: 0 };
      else body = { error: "n/a" };
      return {
        status: 200, ok: true, statusText: "OK",
        headers: { get: () => "application/json" },
        text: async () => JSON.stringify(body),
        json: async () => body,
      };
    },
  };
  ctx.window = ctx; ctx.self = ctx; ctx.top = ctx; ctx.globalThis = ctx;
  vm.createContext(ctx);
  vm.runInContext(main, ctx, { filename: `${plugin}-panel-main.js` });
  return { ctx, document, setPayload: (p) => { payload = p; } };
}

const failures = [];
function check(label, actual, expected) {
  const ok = String(actual) === String(expected);
  if (!ok) failures.push(`${label}: 期望 ${expected}，实际 ${actual}`);
  console.log(`   ${ok ? "PASS" : "FAIL"}  ${label} = ${actual}（期望 ${expected}）`);
}
function checkTrue(label, cond) {
  const ok = !!cond;
  if (!ok) failures.push(`${label}: 断言为假`);
  console.log(`   ${ok ? "PASS" : "FAIL"}  ${label}`);
}

(async () => {
  console.log(`[回归] 面板 = ${plugin}/panel.html`);
  const { ctx, document, setPayload } = loadPanelContext();
  const el = (id) => document.getElementById(id);

  setPayload({ accounts: ACCOUNTS, count: ACCOUNTS.length });
  await ctx.load();
  console.log("阶段1 加载后：");

  // 卡片渲染：有额度的账号必须含进度条与计划名徽章；无额度的显示加载占位。
  const cardA = ctx.card(ACCOUNTS[0]);
  const cardC = ctx.card(ACCOUNTS[2]);
  checkTrue("有额度卡片含进度条", cardA.includes('class="pb"'));
  checkTrue("有额度卡片含额度段 GLM-5.3", cardA.includes("GLM-5.3"));
  checkTrue("有额度卡片含计划名徽章", cardA.includes('class="badge plan"'));
  checkTrue("无额度卡片显示加载占位", cardC.includes("加载中…"));
  checkTrue("无额度卡片不含计划名徽章", !cardC.includes('class="badge plan"'));

  // 汇总：额度已获取 2 / 未知 1，合计 900 万。
  console.log("阶段2 汇总卡：");
  const summary = el("summaryBox").innerHTML;
  checkTrue("汇总含「每日额度合计」", summary.includes("每日额度合计"));
  checkTrue("汇总显示合计 9.00M", summary.includes("9.00M"));
  checkTrue("汇总显示已获取 2 / 未知 1", summary.includes("2 / 未知 1"));

  // 筛选计数：可用 1（A）、禁用 1（B）、测试 1（C）。
  console.log("阶段3 筛选计数：");
  check("cntAll", el("cntAll").textContent, "3");
  check("cntAvailable", el("cntAvailable").textContent, "1");
  check("cntDisabled", el("cntDisabled").textContent, "1");
  check("cntTest", el("cntTest").textContent, "1");

  // 排序：额度升序时未知额度(-1)排最前，其次 B(100万)，最后 A(800万)。
  console.log("阶段4 额度排序：");
  vm.runInContext("currentSort='quota_asc'", ctx);
  const asc = ctx.sortedAccounts(ACCOUNTS).map((a) => a.auth_id);
  check("quota_asc 首位", asc[0], "zcode-ccc");
  check("quota_asc 末位", asc[2], "zcode-aaa");
  vm.runInContext("currentSort='quota_desc'", ctx);
  const desc = ctx.sortedAccounts(ACCOUNTS).map((a) => a.auth_id);
  check("quota_desc 首位", desc[0], "zcode-aaa");
  check("quota_desc 末位", desc[2], "zcode-ccc");

  // 额度数字格式化边界。
  console.log("阶段5 数字格式化：");
  check("fmtCompact 8000000", ctx.fmtCompact(8000000), "8.00M");
  check("fmtCompact 1500", ctx.fmtCompact(1500), "1.5K");
  check("fmtCompact 999", ctx.fmtCompact(999), "999");
  check("fmtNum 3000000", ctx.fmtNum(3000000), "3,000,000");

  console.log("");
  if (failures.length) {
    console.log(`[结果] FAIL（${failures.length} 项）`);
    failures.forEach((f) => console.log("   - " + f));
    process.exit(1);
  }
  console.log("[结果] PASS 全部断言通过");
})();