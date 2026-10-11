# Source Notes

## 2026-10-11 创建：project-cpa-workbuddy-plugin-host-config-safety-rules

- **创建原因**：用户要求「这个问题不仅仅要吸收到知识库，还要吸收到当前项目的 skill 中，作为经验」。
- **来源**：2026-10-11 生产事故「cpa 打不开」的完整诊断与恢复——
  - 现象：`cpa.luode.vip` 管理面板报 `404 未找到 v8 Management API`，重启容器后外网 `502 Bad Gateway`。
  - 根因：`config_store.content` 与 spool 文件被写成**字面 `\n` 转义的单行格式**（70136 字节 / 真实换行 = 1），CPA 原样写 spool 后 YAML 解析为空配置 → `port` 归 0（监听随机端口 44363）、`secret-key` 丢失（`/v0`+`/v8` 管理路由全 404）→ nginx 仍代理 8317 → 502。
  - 写入源定位：`docker events`（16:50–16:56 外部会话在容器内 grep config、直连 mysql 查 `config_store`）+ `find -newermt` 时间窗 + md5 指纹比对 → 命中宿主 `/tmp/new_config.yaml`，与 spool 逐字节相同（md5 `236fa7a9…`）。
  - 恢复：hex 反转义写回（`CONVERT(UNHEX(...) USING utf8mb4)`）+ 回读校验（68930 字节 / 1207 换行 / md5 `21dfea98…` 逐字节一致）+ `docker restart cli-proxy-api`；验收本地与外网 `/management.html`=200、`/v0|/v8/management/config`=401，502 解除。
  - 回滚点保留：`/root/config.yaml.broken.20261011_1715`、表 `cli_proxy_api.config_store_bak_20261011_1715`。
- **关键契约实证**：DB `md5(content)` == spool 文件 `md5sum`，spool mtime 恰为容器启动时刻 → 证明 CPA **原样**把 content 写 spool、不做 `\n` 解码 → `content` 必须是多行 YAML 原文。
- **覆盖**：配置存储契约、故障特征速判（404 / 端口漂移 / 502）、写入源定位手法、带回滚恢复流程、配置写入红线（禁止二次序列化写回、hex 写入、端口显式固定、禁止多会话并行改生产配置）、凭据纪律。
- **分层沉淀**：主体（生产拓扑、路径、容器名、端口、本项目专属事实）留在本**项目级** skill；可迁移部分（「读出→二次序列化→写回」损坏模式、故障时段写入源定位手法、hex 写入 + md5 回读）抽象到用户级 `config-persistence-safety-rules`，本 skill 引用之。
- **关联 skill**：`project-cpa-workbuddy-plugin-release-rules`（发布主链路，本 skill 不重复）；`cgo-plugin-isolated-test`（本地编译验证，不涉及）；`config-persistence-safety-rules`（用户级通用层）。
- **同域冗余扫描**：范围 = 本 skill + release-rules + release-asset-prune-rules + trae-local-verify-rules。发现 0 处逐字重复、0 处门控层叠；既有三个 skill 分别覆盖「发布链路」「发布后资产清理」「Trae 本地验证」，与本 skill「生产宿主配置层」职责互补不重叠。PASS。
- **环境依赖登记**：依赖生产服务器 `cpa-server`（`45.207.222.65:18998`，密钥 `C:\Users\luode\.ssh\id_ed25519_cpa-server`）、MySQL 容器 `mysql`（root 密码经 `$MYSQL_ROOT_PASSWORD` 环境变量读取，不落盘）。
- **凭据纪律**：本 skill 不含任何凭据明文；`docker events` 等命令输出中的密码明文严禁回显。
- **字典刷新**：本项目 `skills/` 无 data.js / 字典.md（该机制属 luode-skills 仓库，跨项目只读），不适用。

## 2026-10-11 更新（二次复发后补强）

- **触发**：同日 18:06 同一模式**二次复发**（外部会话读写生产配置），用户要求继续把经验沉淀到项目 skill。
- **新增内容**：
  1. **故障形态 B**：容器未重启、端口不漂移（仍 `:8317`）、`/management.html` 仍 200，**只有管理面 404** —— 避免被误判为「面板 bug」；
  2. **会话层写入源排查**：`who` / `w` / `last -n 12`；并注明非交互式 `ssh host "cmd"` **不留 `last` 记录**，只能靠 `docker events` 抓；
  3. **热重载修复法**（改 DB + 写 spool 双写，靠 file watcher 重载，**业务零中断**）升为 Step 4 路径 A；重启降级为路径 B；
  4. 修复必须「**双写**」，且 DB `md5(content)` 与 spool `md5sum` 必须一致。
- **证据**：二次事故写入源确认到会话层（外部 SSH 会话 `183.239.175.194`，18:07 登录且仍在线，正在 `docker logs -fn111`）；修复后日志 `management routes enabled after secret key update`，本地与外网 `/management.html`=200、`/v0|/v8/management/config`=401，容器全程未重启（`Up` 时长连续）。
- **关联文档**：`doc/4-bugs/2026-10-11_1812_生产CPA配置转义损坏二次复发与热重载修复.md`
- **复用判定**：二次复发证明「写入源不停止则故障反复」，已把「改配置前先 `w` / `docker events` 确认无并发会话」写入红线。

## 2026-10-11 更新：配置写入红线新增「非必要不重启生产 cpa 容器」

- **更新原因**：用户要求把「非必要不要重启生产的 cpa 容器，插件的更新安装一般不需要重启」吸收到项目规则与 skill。
- **变更**：配置写入红线新增第 7 条——重启 `cli-proxy-api` 属高成本、高影响操作（中断在线流量 + 60–80 秒冷启动），插件的新增 / 更新 / 安装走 `plugin-store install` 热重载即可、一般不需要重启；本 skill「配置损坏恢复」是重启的必要例外（空配置必须重启才能重新加载 spool）且需用户确认。恢复流程 Step 4 同步注明该重启属必要例外。
- **同步落点**：AGENTS.md / CLAUDE.md「仓库与发布」新增「生产容器重启红线（非必要不重启，强制）」；`project-cpa-workbuddy-plugin-release-rules` Step 13 与权责边界同步。
- **字典刷新**：不适用。
- **验证**：SKILL.md 全文回读无乱码，UTF-8 无漂移。
