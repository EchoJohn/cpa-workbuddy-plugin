// panel_chart_removal_repro.mjs —— 静态断言 Token 用量面板的四个图表与导出图片已彻底下线，
// 同时确认保留下来的统计卡片与两张表格结构没有被误删。
// 用法：node test/token-usage/panel_chart_removal_repro.mjs [dashboard.go 路径]
// 动态执行（面板 JS 真实跑起来、且不再请求 /stats/trends）由
// panel_perf_repro.mjs 与 panel_provider_column_repro.mjs 覆盖，本脚本只补结构面。
import fs from "node:fs";
import path from "node:path";

const source = process.argv[2] ?? "token-usage-tracker/usage_stats/dashboard.go";
const text = fs.readFileSync(source, "utf8");

// extractTemplate 取出内嵌 HTML 模板正文，避免把 Go 侧替换器字符串误当成面板代码。
function extractTemplate(body) {
  const start = body.indexOf("const dashboardHTMLTemplate = `");
  if (start < 0) throw new Error("dashboardHTMLTemplate 常量缺失");
  const open = body.indexOf("`", start + "const dashboardHTMLTemplate =".length);
  const close = body.indexOf("`", open + 1);
  return body.slice(open + 1, close);
}

const template = extractTemplate(text);
let failures = 0;

function assertAbsent(needle, label) {
  const hit = template.includes(needle);
  console.log(`${hit ? "   FAIL" : "   PASS"}  ${label}（标记 ${JSON.stringify(needle)}${hit ? " 仍存在" : " 已移除"}）`);
  if (hit) failures += 1;
}

function assertPresent(needle, label) {
  const hit = template.includes(needle);
  console.log(`${hit ? "   PASS" : "   FAIL"}  ${label}（标记 ${JSON.stringify(needle)}${hit ? " 保留" : " 缺失"}）`);
  if (!hit) failures += 1;
}

console.log(`[输入] ${path.resolve(source)}`);
console.log(`[指纹] bytes=${Buffer.byteLength(text, "utf8")} 模板 bytes=${Buffer.byteLength(template, "utf8")}`);
console.log("[回归] 图表与导出图片应彻底下线，卡片与两张表应保留");

// 1. 四张图表的容器与控件
[
  ['<section class="visual-grid"', "Token 趋势柱状图与模型占比环形图容器"],
  ['<section class="advanced-grid"', "成本趋势与效率散点图容器"],
  ['id="barWrap"', "趋势图挂载点"],
  ['id="chart"', "趋势图 SVG"],
  ['id="donut"', "环形图 SVG"],
  ['id="legend"', "环形图图例"],
  ['id="costWrap"', "成本图挂载点"],
  ['id="costChart"', "成本图 SVG"],
  ['id="efficiencyChart"', "效率图 SVG"],
  ['id="modelShareMetric"', "占比口径切换组"],
  ['id="resetModelButton"', "图表下钻重置按钮"],
  ['id="granularity"', "图表聚合粒度下拉"],
  ['id="tooltip" class="chart-tooltip"', "图表浮层容器"],
  ['id="exportPNG"', "导出图表图片按钮"],
].forEach(([needle, label]) => assertAbsent(needle, label));

// 2. 图表专用 JS 与后端接口
[
  ["function renderBar(", "趋势柱状图渲染函数"],
  ["function renderDonut(", "环形图渲染函数"],
  ["function renderCostTrend(", "成本趋势渲染函数"],
  ["function renderEfficiency(", "效率散点渲染函数"],
  ["function exportPNG(", "导出图片实现"],
  ["function aggregateTrend(", "趋势聚合消费端"],
  ["function selectModel(", "图表下钻选择"],
  ["function toggleModel(", "图例隐藏切换"],
  ["statsTrendsURL", "趋势接口地址常量"],
  ["/stats/trends", "趋势接口路径"],
].forEach(([needle, label]) => assertAbsent(needle, label));

// 3. 保留能力必须仍在
[
  ['<section class="cards"', "顶部四张统计卡片"],
  ['id="totalTokens"', "Token 总量卡片"],
  ['id="totalCost"', "成本卡片"],
  ['id="requests"', "请求数卡片"],
  ['id="topModel"', "最常用模型卡片"],
  ['id="requestRows"', "请求明细表体"],
  ['id="groups"', "维度汇总表体"],
  ["function renderGroups(", "维度表渲染函数"],
  ["function renderRequests(", "请求表渲染函数"],
  ["function renderVisuals(", "卡片渲染函数"],
  ["function exportCSV(", "CSV 导出实现"],
  ["key:'provider'", "服务商列定义"],
  ["statsInitialURL", "首屏统计接口地址"],
  ["costsURL", "成本接口地址"],
].forEach(([needle, label]) => assertPresent(needle, label));

console.log(`\n[回归] ${failures === 0 ? "PASS" : `FAIL（${failures} 项不通过）`}`);
process.exit(failures === 0 ? 0 : 1);
