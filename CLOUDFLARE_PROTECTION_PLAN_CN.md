# sub2api × Cloudflare Pro 防护方案（CC/DDoS）

> 目标：客户端 → Cloudflare（橙云代理）→ 源站防火墙（仅放行 CF 网段）→ Caddy → sub2api。
> 本文档面向"由 Codex 通过 Cloudflare API 自动创建规则"的场景：占位符 `$CF_API_TOKEN`、`$ZONE_ID`、`$DOMAIN` 由执行时注入。
> 所有规则均基于本仓库真实路由设计，核心原则：**API 路径只 block/限速、绝不 challenge**（SDK/CLI 客户端无法过人机验证）。

## 0. 本项目的流量画像（规则设计依据）

| 路径 | 用途 | 认证方式 | 防护策略 |
|---|---|---|---|
| `/v1/*` | Anthropic/OpenAI 网关（messages、chat/completions、responses 等，SSE 流式） | `Authorization: Bearer` / `x-api-key` | 无认证头直接挡、限速、禁 challenge |
| `/v1beta/*` | Gemini 网关 | `x-goog-api-key` 或 `?key=` 查询参数 | 限速；**不做**无认证头拦截（有 query key 形态） |
| `/antigravity/v1/*`、`/antigravity/v1beta/*` | Antigravity 网关 | 同上两类 | 同上 |
| `/backend-api/codex/*` | Codex 直连 | `Authorization: Bearer` | 同 `/v1/*` |
| `/api/v1/auth/*` | 登录/注册/验证码 | 无（公开） | 严格限速（防爆破+邮件轰炸） |
| `/api/v1/admin/*` | 管理面板 XHR | JWT | 可选 IP 白名单；**不能 challenge**（XHR 会静默失败） |
| `/api/v1/payment/public/*` | 支付回调/公开支付接口 | 签名 | **必须豁免**一切拦截 |
| `/`、`/assets/*` | Vue SPA | — | 可缓存；紧急时可对 HTML 文档 challenge |
| WebSocket（OpenAI WS 转发） | 实时接口 | header | 需开启 zone WebSockets |

其他事实：
- 后端已原生支持 `CF-Connecting-IP`（`backend/internal/pkg/ip/ip.go`）与 `server.trusted_proxies` 配置（`config.go`），配好后用量日志/应用内 IP 限速拿到的才是真实客户端 IP。
- LLM prompt 正文天然包含 SQL/HTML/script 文本 → OWASP 托管规则必须对网关路径豁免，否则大规模误报 403。
- CF 代理有约 100s 空闲超时（Pro 不可调）：引导用户走 `stream:true`；SSE 持续出字不受影响。

## 1. Zone 基础设置（Zone Settings API）

```bash
CF_API="https://api.cloudflare.com/client/v4"
H=(-H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json")

for kv in \
  'ssl:strict' \
  'always_use_https:on' \
  'automatic_https_rewrites:on' \
  'min_tls_version:1.2' \
  'tls_1_3:on' \
  'http3:on' \
  'websockets:on' \
  'security_level:medium' \
  'browser_check:off' \
  'rocket_loader:off' \
  'challenge_ttl:1800'
do
  key="${kv%%:*}"; val="${kv#*:}"
  curl -s "${H[@]}" -X PATCH "$CF_API/zones/$ZONE_ID/settings/$key" \
    --data "{\"value\":\"$val\"}"
done
```

要点：
- `ssl=strict`（Full Strict）：源站须有可信证书（见 §7 Caddy 两种方案）。
- `websockets=on`：sub2api 有 OpenAI WS 转发，必须开。
- `browser_check=off`、`rocket_loader=off`：BIC 可能拦精简 header 的 SDK；Rocket Loader 可能破坏 Vue SPA。
- **不要开启 Super Bot Fight Mode 的 block/challenge**（Pro 不能按路径豁免，会拦掉全部 SDK/CLI 流量）。"Definitely automated" 一项保持 *Allow*。
- Under Attack Mode 不要全站开（API 全灭），紧急时用 §4 的开关规则。

## 2. 自定义 WAF 规则（phase: http_request_firewall_custom）

一次性 PUT 整个 entrypoint（**顺序即优先级**，skip 必须在最前）：

```bash
curl -s "${H[@]}" -X PUT \
  "$CF_API/zones/$ZONE_ID/rulesets/phases/http_request_firewall_custom/entrypoint" \
  --data @custom_rules.json
```

`custom_rules.json`：

```json
{
  "rules": [
    {
      "description": "00 SKIP: payment callbacks must never be blocked",
      "expression": "(http.request.uri.path wildcard \"/api/v1/payment/public/*\")",
      "action": "skip",
      "action_parameters": { "ruleset": "current" },
      "logging": { "enabled": true }
    },
    {
      "description": "01 BLOCK: bearer-auth API paths without any auth header (kills naive floods before origin)",
      "expression": "(http.request.uri.path wildcard \"/v1/*\" or http.request.uri.path wildcard \"/antigravity/v1/*\" or http.request.uri.path wildcard \"/backend-api/codex/*\") and http.request.method ne \"OPTIONS\" and not any(http.request.headers.names[*] in {\"authorization\" \"x-api-key\" \"x-goog-api-key\"})",
      "action": "block"
    },
    {
      "description": "02 BLOCK: scanner junk (site is Go+Vue, any php/wp path is malicious)",
      "expression": "http.request.uri.path wildcard \"*.php\" or http.request.uri.path wildcard \"/wp-*\" or http.request.uri.path wildcard \"/.env*\" or http.request.uri.path wildcard \"/.git*\" or http.request.uri.path wildcard \"/phpmyadmin*\" or http.request.uri.path wildcard \"/cgi-bin/*\" or http.request.uri.path wildcard \"/xmlrpc*\"",
      "action": "block"
    },
    {
      "description": "03 BLOCK: ancient HTTP versions (common in CC botnets)",
      "expression": "http.request.version in {\"HTTP/1.0\" \"HTTP/0.9\"}",
      "action": "block"
    },
    {
      "description": "04 (OPTIONAL, default disabled) admin API allowlist - fill your fixed IPs then enable",
      "expression": "http.request.uri.path wildcard \"/api/v1/admin/*\" and not ip.src in {203.0.113.10 203.0.113.11}",
      "action": "block",
      "enabled": false
    },
    {
      "description": "05 EMERGENCY (default disabled): managed challenge for browser HTML documents only - flip on during attack, APIs unaffected",
      "expression": "http.request.method eq \"GET\" and not http.request.uri.path wildcard \"/v1/*\" and not http.request.uri.path wildcard \"/v1beta/*\" and not http.request.uri.path wildcard \"/antigravity/*\" and not http.request.uri.path wildcard \"/backend-api/*\" and not http.request.uri.path wildcard \"/api/*\" and not http.request.uri.path wildcard \"/assets/*\" and any(http.request.headers[\"accept\"][*] contains \"text/html\")",
      "action": "managed_challenge",
      "enabled": false
    }
  ]
}
```

说明：
- 规则 01 故意**不含** `/v1beta/*`（Gemini 允许 `?key=` 查询参数认证，拦无 header 会误伤）。
- 规则 04 默认关闭：只有你有固定出口 IP/自建 VPN 时再填 IP 启用；没有就靠强密码+Passkey，**不要**改成 challenge。
- 规则 05 是"手动应急开关"：被打时 `PATCH` 该规则 `enabled:true`，等价于只对浏览器页面开 Under-Attack，API 不受影响。

## 3. 速率限制（phase: http_ratelimit，Pro 配额有限、只建 2 条）

```bash
curl -s "${H[@]}" -X PUT \
  "$CF_API/zones/$ZONE_ID/rulesets/phases/http_ratelimit/entrypoint" \
  --data @ratelimit_rules.json
```

`ratelimit_rules.json`：

```json
{
  "rules": [
    {
      "description": "RL1: per-IP flood guard on all LLM gateway paths",
      "expression": "(http.request.uri.path wildcard \"/v1/*\" or http.request.uri.path wildcard \"/v1beta/*\" or http.request.uri.path wildcard \"/antigravity/*\" or http.request.uri.path wildcard \"/backend-api/codex/*\")",
      "action": "block",
      "ratelimit": {
        "characteristics": ["ip.src", "cf.colo.id"],
        "period": 60,
        "requests_per_period": 300,
        "mitigation_timeout": 600
      }
    },
    {
      "description": "RL2: auth endpoints - brute force & email-code bombing",
      "expression": "(http.request.uri.path wildcard \"/api/v1/auth/*\")",
      "action": "block",
      "ratelimit": {
        "characteristics": ["ip.src", "cf.colo.id"],
        "period": 60,
        "requests_per_period": 20,
        "mitigation_timeout": 600
      }
    }
  ]
}
```

阈值建议：RL1 从 300/min/IP 起步（Claude Code 并行会话也远达不到），观察一周 Security Events 后再收紧；公司/校园 NAT 大出口用户多时勿低于 120。Pro 只按 IP 维度计数（按 API-key 维度计数需更高套餐），更细粒度限流靠 sub2api 自身的 key/用户配额。

## 4. 托管规则集 + LLM 路径豁免（phase: http_request_firewall_managed）

```bash
curl -s "${H[@]}" -X PUT \
  "$CF_API/zones/$ZONE_ID/rulesets/phases/http_request_firewall_managed/entrypoint" \
  --data @managed_rules.json
```

`managed_rules.json`：

```json
{
  "rules": [
    {
      "description": "SKIP OWASP for LLM bodies (prompts legitimately contain SQL/XSS-looking text)",
      "expression": "(http.request.uri.path wildcard \"/v1/*\" or http.request.uri.path wildcard \"/v1beta/*\" or http.request.uri.path wildcard \"/antigravity/*\" or http.request.uri.path wildcard \"/backend-api/codex/*\")",
      "action": "skip",
      "action_parameters": { "rulesets": ["4814384a9e5d4991b9815dcfc25d2f1f"] },
      "logging": { "enabled": true }
    },
    {
      "description": "Cloudflare Managed Ruleset",
      "expression": "true",
      "action": "execute",
      "action_parameters": { "id": "efb7b8c949ac4650a09736fc376e9aee" }
    },
    {
      "description": "OWASP Core Ruleset (only for non-LLM paths due to skip above)",
      "expression": "true",
      "action": "execute",
      "action_parameters": { "id": "4814384a9e5d4991b9815dcfc25d2f1f" }
    }
  ]
}
```

## 5. 缓存规则（phase: http_request_cache_settings）

```json
{
  "rules": [
    {
      "description": "bypass cache for all dynamic/API paths",
      "expression": "(http.request.uri.path wildcard \"/v1/*\" or http.request.uri.path wildcard \"/v1beta/*\" or http.request.uri.path wildcard \"/antigravity/*\" or http.request.uri.path wildcard \"/backend-api/*\" or http.request.uri.path wildcard \"/api/*\")",
      "action": "set_cache_settings",
      "action_parameters": { "cache": false }
    }
  ]
}
```

（PUT 到 `.../rulesets/phases/http_request_cache_settings/entrypoint`。SPA 静态资源走 CF 默认静态扩展名缓存即可。）

## 6. 源站锁死（没有这一步，前面全部白做）

1. **换源站 IP**：你已被直打过，旧 IP 视为已暴露。迁移/换 IP 后再接入 CF，DNS 只留橙云记录，不留任何指向源站的灰云/历史记录（含邮件 MX 泄漏）。
2. **防火墙只放行 CF 网段** 到 80/443（其余端口一律不对公网开放）：

```bash
# ufw 示例（v4+v6，建议放 cron 每周刷新一次 CF 网段）
for ip in $(curl -s https://www.cloudflare.com/ips-v4) $(curl -s https://www.cloudflare.com/ips-v6); do
  ufw allow proto tcp from "$ip" to any port 80,443 comment cloudflare
done
ufw deny in to any port 80,443
```

3. **进阶（强烈推荐）**：开启 Authenticated Origin Pulls（zone 设置 `tls_client_auth=on` + Caddy 校验 CF origin-pull 客户端证书），即使 IP 泄漏也无法绕过 CF 直连。

## 7. Caddy 配置（真实 IP + SSE + 证书）

```caddyfile
{
  servers {
    # 只信任来自 CF 的转发头，并用 CF-Connecting-IP 还原真实客户端 IP
    trusted_proxies static 173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22 141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20 197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13 104.24.0.0/14 172.64.0.0/13 131.0.72.0/22 2400:cb00::/32 2606:4700::/32 2803:f800::/32 2405:b500::/32 2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32
    client_ip_headers CF-Connecting-IP
  }
}

$DOMAIN {
  # 证书二选一：
  # A（推荐）：Cloudflare DNS-01（需 caddy-dns/cloudflare 插件 + 仅含 DNS:Edit 的独立 token）
  tls {
    dns cloudflare {env.CF_DNS_API_TOKEN}
  }
  # B：Cloudflare Origin CA 证书（面板签 15 年，配 ssl=strict）
  # tls /etc/caddy/origin-cert.pem /etc/caddy/origin-key.pem

  encode zstd gzip
  reverse_proxy 127.0.0.1:8080 {
    flush_interval -1   # SSE 逐字下发，禁止缓冲
  }
}
```

**sub2api 配置**：`server.trusted_proxies` 设为 Caddy 地址（如 `["127.0.0.1"]`）。代码已支持 CF-Connecting-IP 链路，配好后用量日志 `ip_address`、应用内按 IP 的风控才是真实客户端 IP，而不是 CF/Caddy 的 IP。

## 8. 给 Codex 的执行须知（token 安全）

- 用 **zone 级最小权限 API Token**，不要用 Global API Key。权限：`Zone:Read` + `Zone Settings:Edit` + `Zone WAF:Edit` + `Cache Rules:Edit` + `Config Rules:Edit`，Zone Resources 只选这一个域名，设置过期时间，可再加 Client IP 过滤限定 Codex 出口 IP。
- 执行顺序：§1 设置 → §5 缓存 → §4 托管 → §2 自定义 → §3 限速 → §6/§7 源站侧 → §9 验收。
- PUT entrypoint 是**整体替换**该 phase 的规则：如 zone 里已有手工建的规则，先 GET 备份再合并。

## 9. 验收清单（Codex 执行完逐条跑）

```bash
# 1. 无认证头打 /v1 → 应被 WAF 挡（非 200，且源站无日志）
curl -s -o /dev/null -w "%{http_code}\n" -X POST https://$DOMAIN/v1/messages
# 2. 带 key 正常请求 → 200
curl -s -o /dev/null -w "%{http_code}\n" https://$DOMAIN/v1/models -H "Authorization: Bearer $SK"
# 3. SSE 流式逐字出（不被缓冲/不断流）
curl -N https://$DOMAIN/v1/messages -H "Authorization: Bearer $SK" -H "content-type: application/json" \
  -d '{"model":"claude-sonnet-4","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"hi"}]}'
# 4. 登录限速：25 次后应出现 429
for i in $(seq 1 25); do curl -s -o /dev/null -w "%{http_code} " -X POST https://$DOMAIN/api/v1/auth/login -d '{}'; done; echo
# 5. 扫描路径被挡
curl -s -o /dev/null -w "%{http_code}\n" https://$DOMAIN/wp-login.php
# 6. 直连源站 IP 应超时/拒绝（防火墙生效）
curl -m 5 -s -o /dev/null -w "%{http_code}\n" https://<源站IP>/ || echo "blocked ✓"
# 7. prompt 含 SQL 文本不被 OWASP 误杀 → 200
curl -s -o /dev/null -w "%{http_code}\n" https://$DOMAIN/v1/messages -H "Authorization: Bearer $SK" \
  -H "content-type: application/json" \
  -d '{"model":"claude-sonnet-4","max_tokens":16,"messages":[{"role":"user","content":"explain SELECT * FROM users WHERE 1=1"}]}'
# 8. 用量日志里的 ip_address 是你的真实公网 IP（真实 IP 链路生效）
```

## 10. 已知限制与运营建议

- CF 边缘约 100s 空闲超时（Pro 不可调）：非流式超长推理可能 524，引导用户 `stream:true`。
- challenge 类动作只出现在规则 05（浏览器 HTML 应急开关）；任何 API 路径出现 challenge 都是配置错误。
- 配额（自定义规则/限速规则条数）以你面板实际显示为准，本方案已按 Pro 的小配额设计（限速仅 2 条）。
- 每周看一次 Security → Events：误杀（合法 UA 被 block）就加 skip；漏杀（源站仍有攻击日志）就收紧 RL1 阈值或启用规则 05。
