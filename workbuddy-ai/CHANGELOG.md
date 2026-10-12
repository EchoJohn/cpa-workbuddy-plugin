# WorkBuddy AI Plugin Changelog

## 0.1.11

### Feat - 补齐 checkin_auto 配置声明与全套路由与基础设施回归单测

- 变更要点:
  1. 补齐 `main.go` 中 `ConfigFields` 的 `checkin_auto` 配置字段声明，打通宿主管理界面对国际版自动签到调度器的可视化配置。
  2. 全面补充 11 套核心单测资产（涵盖会话粘性路由、低积分优先调度、全灭 defer、计数持久化、Token 保活、测试标签与看门狗）。
- 涉及文件: workbuddy-ai/main.go、workbuddy-ai/VERSION、workbuddy-ai/*_test.go
