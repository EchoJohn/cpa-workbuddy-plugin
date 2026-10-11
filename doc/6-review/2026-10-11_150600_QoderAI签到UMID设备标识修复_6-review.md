# 6-review 风格回归：Qoder AI 国际版签到 UMID 设备标识补齐与多账号隔离修复（0.1.6）

- 日期：2026-10-11 15:06
- 范围：`qoder-ai/umid.go`、`qoder-ai/umid_codec.go`、`qoder-ai/umid_crypto.go`、`qoder-ai/billing.go`、`qoder-ai/checkin.go`、`qoder-ai/qoder_ai_endpoints_test.go`、`qoder-ai/CHANGELOG.md`、`qoder-ai/VERSION`、`qoder-ai/main.go`
- 结论：STYLE: PASS

## 检查步骤（由 `static_owner_router.py` 推导）

- STYLE-01 静态格式与编码一致性：全部新增与修改文件均为 UTF-8 编码；经 `python scripts/cgo-shim-build.py qoder-ai` 的 `go build` / `go vet` / `go test` 全绿校验；未在函数内部使用 `var (...)` 分组声明，未引入 `iota`。
- STYLE-02 命名与符号引用一致：新增 `umidIdentity`、`resolveAccountUMID`、`rotateAccountUMID`、`buildUMIDRequestBody`、`decryptUMIDResponseDT`、`deriveUMIDCodes`、`xxh64Sum` 均沿用包内驼峰命名习惯。
- STYLE-03 注释分层与定义位置：所有新增与修改函数均具备完整中文说明、`[参数]`、`[返回]` 与单行 `最近修改时间`，方法体按流程分步标注 `1.`、`2.`、`3.`，无多行论证式过程注释。
- STYLE-04 函数签名与参数结构：新增文件拆分为 `umid.go`（176 行）、`umid_codec.go`（185 行）、`umid_crypto.go`（150 行），全部小于 200 行；所有新增/修改函数参数与返回值均不超过 2 个（多参数收敛为 `umidDeriveInput`、`umidSectionSpec`、`umidRequestPacket`、`accountCacheEntry`），单函数均小于 80 行。
- STYLE-08 语言与框架特异写法：使用 Go 标准库 `crypto/aes`、`crypto/cipher`、`compress/zlib`、`math/bits` 实现 UMID 报文编解码与 XXH64，并发缓存使用 `sync.Map` 副本返回防止并发数据竞争。
- STYLE-09 测试资产与编码前契约：回归测试落在 `qoder-ai/qoder_ai_endpoints_test.go`，覆盖 Windows/Linux 实测黄金样本 `TestDeriveUMIDCodes_GoldenParity`、响应解密 `TestDecryptUMIDResponseDT_GoldenSample` 及头部注入契约 `TestCampaignHeaders_InjectsMachineIdentity`，并通过必失败哨兵反证。

## 证据

- `python scripts/cgo-shim-build.py qoder-ai`：build / vet / test 全绿（7.158s）。
- 必失败哨兵反证：将 `TestDeriveUMIDCodes_GoldenParity` 断言临时改为 `SENTINEL_FAIL` 后执行必败（`mTypeWin="51f6feff14f099a261"; want SENTINEL_FAIL`），恢复后全绿。

## 负向边界

本记录只判断写法、位置、格式与既有习惯，不判断业务正确性、需求覆盖或发布放行。
