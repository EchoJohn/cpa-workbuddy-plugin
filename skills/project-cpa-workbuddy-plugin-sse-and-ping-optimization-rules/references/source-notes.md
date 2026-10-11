# Source Notes

## 2026-10-11 创建：project-cpa-workbuddy-plugin-sse-and-ping-optimization-rules

- **创建原因**：用户要求「将本次的经验吸收到项目 skill 中。看看其他的插件根据当前的优化经验有没有需要优化的地方」。
- **来源**：2026-10-11 会话对 `qoderwork-provider`（0.9.28/0.9.29）、`qoder-ai-provider`（0.1.7/0.1.8）、`traework-provider`（0.2.6/0.2.8）三插件的完整排障、修复、发布与生产实测闭环：
  1. `qoderwork` / `qoder-ai` 0.9.28 & 0.1.7：定位并修复 `collectUpstreamStreamQoder` 遇到 `[DONE]` 误用 `continue` 死等 Keep-Alive、多种 `[DONE]` 形态漏检、换号 Context 丢失与 UTF-8 截断；
  2. `qoderwork` / `qoder-ai` 0.9.29 & 0.1.8：定位并移除 `handlePollLogin` 中基于 `hostAuthList()` `CreatedAt > startedAt` 判定成功的 `Panel-import` 假成功漏洞（`writeAuthFileDirect` 原子重命名刷新重置 `CreatedAt` 导致未点链接自动认证成功）；
  3. `traework` 0.2.6：定位并修复 `scanSSE` 收到 `event: done` 未跳出循环死等 Keep-Alive 的问题（引入 `errTraeStreamDone`）；
  4. `traework` 0.2.8：定位并修复面板「测试」按钮（`sendActivePingTrae`）使用非流式 `hostHTTPDo` 被迫等待思考模型（如 `glm-5.3-flash`）生成全部 264 个 `reasoning_content` token（耗时 5.6~8.1s）及未拦截 HTTP 200 `event: error` 的问题；改为流式首包即停（`verifyActivePingTraeStream`）+ `prewarmStreamHTTPClient` 后台预热 TLS Keep-Alive 连接 + 过滤内部非对话子代理占位项，生产同账号实测 `glm-5.3-flash` 从 `5.615s` 降至 `1.543s`、`deepseek-v4.1-flash` 降至 `943ms`、`qwen3.8-max` 降至 `1.388s`。
- **同域冗余扫描**：与 `project-cpa-workbuddy-plugin-trae-local-verify-rules`（只管本地解密 storage.json 直连验证脚本）、`project-cpa-workbuddy-plugin-release-rules`（只管 CI 发布与生产部署链路）职责正交，无逐字重复与门控冲突。
