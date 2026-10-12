# 大模型厂商权威官方规格对照矩阵（Model Specs Matrix）

> 权威参考源：CPA 宿主 `internal/registry/models/models.json`、`model_definitions.go` 及各大模型厂商官方开发文档（2026-10 实测对齐）。

| 模型家族 | 典型模型匹配键（小写前缀） | 官方最大上下文（Context Window） | 官方常规最大输出（Max Output Tokens） | 思考/推理模式最大输出（Reasoning Max） | 典型厂商限制与注意事项 |
| --- | --- | --- | --- | --- | --- |
| **DeepSeek** | `deepseek-v4.1-flash`<br>`deepseek-v4-flash`<br>`deepseek-v4-pro` | 1,048,576 | 16,384 | **64,000** | 腾讯 CodeBuddy / 云厂商放宽的推理输出上限，长思考必须留足空间。 |
| **DeepSeek** | `deepseek-chat` | 131,072 | 8,192 | 8,192 | 官方原生对话模型，无深度思维链。 |
| **DeepSeek** | `deepseek-reasoner` | 131,072 | 8,192 | **64,000** | 官方深度思考模型，思考链占用输出配额。 |
| **GLM 智谱** | `glm-5.3`<br>`glm-5.3-flash` | 1,048,576 | 16,384 | **128,000** | 旗舰超长思维链模型，官方推理上限支持 128k。 |
| **GLM 智谱** | `glm-5.2` | 200,000 | 16,384 | **64,000** | 上代旗舰模型，推理上限 64k。 |
| **GLM 智谱** | `glm-4-plus` | 131,072 | 8,192 | 16,384 | 经典通用模型。 |
| **Kimi** | `kimi-k2.6`<br>`kimi-k2.8`<br>`kimi-k3` | 262,144 | 16,384 | **65,536** | Moonshot 新一代推理模型，Thinking 模式最高 64k。 |
| **Kimi** | `kimi-k2`<br>`kimi-k2-thinking` | 131,072 | 8,192 | 32,768 | 经典长文本模型。 |
| **Qwen 通义** | `qwen3.8-max`<br>`qwen3.8-flash` | 1,000,000 | 16,384 | **32,768** | 阿里千问 3.8 增强版，支持扩展输出。 |
| **Qwen 通义** | `qwen3.7-max`<br>`qwen3.7-plus` | 1,000,000 | 16,384 | 32,768 | 千问 3.7 旗舰。 |
| **Qwen 通义** | `qwen-2.5-72b` | 131,072 | 8,192 | 16,384 | 官方 DashScope 开源旗舰。 |
| **MiniMax** | `minimax-m3` | 1,000,000 | 16,384 | **32,768** | 稀疏大模型，推理输出上限 32k。 |
| **Claude** | `claude-opus-4-6` | 1,000,000 | 8,192 | **128,000** | Anthropic 旗舰，长推理支持 128k。 |
| **Claude** | `claude-sonnet-4-6`<br>`claude-3-7-sonnet` | 200,000 | 8,192 | **64,000** | 开启 Thinking Beta 模式时最大支持 64k；常规模式 8192。 |
| **Claude** | `claude-3-5-sonnet` | 200,000 | 8,192 | 8,192 | 常规版本，无官方长思维链扩展，固定 8192。 |
| **OpenAI / Codex** | `gpt-6-luna`<br>`gpt-6-sol`<br>`gpt-5.6-luna`<br>`gpt-5.6-terra`<br>`gpt-5.5` | 272,000 ~ 1,000,000 | 16,384 | **128,000** | Codex 专用模型，全面支持 128k 深度补全。 |
| **OpenAI / Codex** | `gpt-6-astra` | 1,000,000 | 16,384 | 64,000 | 全尺寸多模态模型。 |
| **OpenAI** | `o1`<br>`o3-mini` | 200,000 | 65,536 | **100,000** | 官方专属推理模型，思维链最大 100k。 |
| **OpenAI** | `gpt-4o`<br>`gpt-4o-mini` | 128,000 | 16,384 | 16,384 | 官方标准多模态，固定 16,384。 |
| **Gemini** | `gemini-3.8-flash-high`<br>`gemini-3.7-flash-high` | 1,048,576 | 8,192 | **65,536** | Google Thinking 模式放宽至 64k。 |
| **Gemini** | `gemini-2.5-pro`<br>`gemini-2.5-flash` | 1,048,576 | 8,192 | 65,536 | 2.5 世代多模态模型。 |
| **腾讯混元** | `hy3` | 1,000,000 | 16,384 | **32,768** | 腾讯混元大模型，思维链模式 32k。 |
| **通用未知模型** | `*`（兜底） | 131,072 | 8,192 | 32,768 | 未收录模型兜底区间，既安全不超纲，又具备充裕思考空间。 |
