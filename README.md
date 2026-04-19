# one-codingplan (ocp)

ocp aggregates multiple AI coding plan credentials (Minimax, Kimi, Qwen, Mimo and others) behind a single OpenAI-compatible and Anthropic-compatible endpoint. Point your tools at one URL with one key — ocp handles routing, failover, and credit tracking transparently.

---

## Quick Start / 快速开始

### 1. Configure / 配置

Copy and edit the config file:

```bash
cp config.yaml.example config.yaml   # or edit config.yaml directly
```

Key fields:

```yaml
server:
  port: 9189
  admin_key: "changeme123"   # portal & admin API password

database:
  path: "./ocp.db"

upstreams:
  - name: minimax
    base_url: https://api.minimaxi.com/anthropic
    api_key: "sk-..."
    enabled: true
```

### 2. Build & Run / 构建与运行

```bash
make build
OCP_ENCRYPTION_KEY=<16-char-secret> ./ocp --config config.yaml
```

`OCP_ENCRYPTION_KEY` is used to encrypt upstream API keys in the database. Must be exactly 16, 24, or 32 characters.

### 3. Open Portal / 打开管理面板

Visit **http://localhost:9189** and sign in with your `admin_key`.

---

## Admin API

All admin endpoints require `Authorization: Bearer <admin_key>`.

### Upstreams

```bash
# List all upstreams with health status
curl http://localhost:9189/api/upstreams \
  -H "Authorization: Bearer changeme123"

# Toggle an upstream enabled/disabled (use id from list)
curl -X POST http://localhost:9189/api/upstreams/1/toggle \
  -H "Authorization: Bearer changeme123"

# Rotate to next available upstream
curl -X POST http://localhost:9189/api/upstreams/rotate \
  -H "Authorization: Bearer changeme123"
```

### Access Keys

```bash
# List all keys
curl http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123"

# Create a key
curl -X POST http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key", "token_budget": 1000000, "rate_limit_per_min": 60}'

# Block a key
curl -X POST http://localhost:9189/api/keys/<id>/block \
  -H "Authorization: Bearer changeme123"

# Unblock a key
curl -X POST http://localhost:9189/api/keys/<id>/unblock \
  -H "Authorization: Bearer changeme123"
```

### Proxy API (using an access key)

```bash
# OpenAI-compatible
curl http://localhost:9189/v1/chat/completions \
  -H "Authorization: Bearer ocp-<your-key-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "say hi"}]
  }'

# Anthropic-compatible
curl http://localhost:9189/v1/messages \
  -H "Authorization: Bearer ocp-<your-key-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "say hi"}]
  }'
```

---

## Database Initialization

ocp uses SQLite and creates the schema automatically via GORM `AutoMigrate` on first startup. No manual setup is required for a fresh deployment.

If you prefer to initialize the database manually (e.g. for a clean environment or CI), use the provided `init.sql`:

```bash
sqlite3 ocp.db < init.sql
```

`init.sql` includes:
- Full table and index definitions (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)
- Seed rows for all supported upstream providers with blank API keys

After initializing, set real API keys via the portal (**Upstream Status → Edit**) or the admin API:

```bash
curl -X PATCH http://localhost:9189/api/upstreams/<id> \
  -H "Authorization: Bearer changeme123" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "your-real-key"}'
```

> **Note:** Re-running `init.sql` on an existing database is safe — all inserts use `INSERT OR IGNORE`.

---

## Access Key Error Codes

| Situation | HTTP Status | Error |
|-----------|-------------|-------|
| Missing or unknown token | 401 Unauthorized | `unauthorized` |
| Key disabled / blocked | 403 Forbidden | `key disabled` |
| Key expired | 403 Forbidden | `key expired` |
| Token budget exceeded | 429 Too Many Requests | `token budget exceeded` |
| Per-minute rate limit exceeded | 429 Too Many Requests | `per-minute rate limit exceeded` |
| Per-day rate limit exceeded | 429 Too Many Requests | `per-day rate limit exceeded` |

---

## Database Schema / 数据库结构

SQLite file at the path configured in `database.path` (default: `./ocp.db`).

### `upstreams`

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER | Primary key |
| `name` | TEXT | Provider name (e.g. `minimax`, `kimi`) |
| `base_url` | TEXT | Provider API base URL |
| `api_key_enc` | BLOB | Encrypted API key |
| `enabled` | BOOLEAN | Whether this upstream is active in the pool |
| `available` | BOOLEAN | Runtime health — false during cooldown/circuit-open |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

```bash
sqlite3 ocp.db "SELECT id, name, enabled FROM upstreams;"
```

### `access_keys`

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT (UUID) | Primary key |
| `name` | TEXT | Human-readable label |
| `token` | TEXT | Bearer token sent by clients (`ocp-...`) |
| `enabled` | BOOLEAN | false = blocked, rejects all requests |
| `token_budget` | INTEGER | Max tokens allowed (0 = unlimited) |
| `tokens_used` | INTEGER | Cumulative tokens consumed |
| `rate_limit_per_min` | INTEGER | Per-minute request cap (0 = unlimited) |
| `rate_limit_per_day` | INTEGER | Per-day request cap (0 = unlimited) |
| `allowed_upstreams` | TEXT | JSON array of upstream names; empty = all |
| `expires_at` | DATETIME | Nullable expiry |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

```bash
sqlite3 ocp.db "SELECT name, token, enabled, tokens_used FROM access_keys;"
```

---

## 中文说明

### 快速测试

**启动服务：**

```bash
make build
OCP_ENCRYPTION_KEY=1234567890123456 ./ocp --config config.yaml
```

服务默认监听 **9189** 端口。

**管理面板：** 浏览器打开 `http://localhost:9189`，使用 `admin_key`（默认 `changeme123`）登录。

---

### 管理 API 示例

所有管理接口需要 `Authorization: Bearer <admin_key>` 请求头。

**查看所有上游状态：**

```bash
curl http://localhost:9189/api/upstreams \
  -H "Authorization: Bearer changeme123"
```

**切换上游启用/禁用（id 从列表获取）：**

```bash
curl -X POST http://localhost:9189/api/upstreams/1/toggle \
  -H "Authorization: Bearer changeme123"
```

**查看所有访问密钥：**

```bash
curl http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123"
```

**创建访问密钥：**

```bash
curl -X POST http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key", "token_budget": 1000000, "rate_limit_per_min": 60}'
```

**封禁 / 解封密钥：**

```bash
curl -X POST http://localhost:9189/api/keys/<id>/block \
  -H "Authorization: Bearer changeme123"

curl -X POST http://localhost:9189/api/keys/<id>/unblock \
  -H "Authorization: Bearer changeme123"
```

**使用访问密钥发送请求：**

```bash
curl http://localhost:9189/v1/chat/completions \
  -H "Authorization: Bearer ocp-<你的密钥token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

---

### 数据库说明

数据库为 SQLite，路径由 `config.yaml` 中 `database.path` 配置（默认 `./ocp.db`）。

**`upstreams` 表 — 上游提供商**

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `name` | 提供商名称（如 `minimax`、`kimi`） |
| `base_url` | API 基础地址 |
| `api_key_enc` | 加密存储的 API Key |
| `enabled` | 是否启用（面板开关控制） |
| `available` | 运行时健康状态（限速冷却期间为 false） |

```bash
sqlite3 ocp.db "SELECT id, name, enabled FROM upstreams;"
```

**`access_keys` 表 — 访问密钥**

| 字段 | 说明 |
|------|------|
| `id` | UUID 主键 |
| `name` | 密钥名称 |
| `token` | 客户端使用的 Bearer Token（`ocp-...`） |
| `enabled` | false 表示已封禁 |
| `token_budget` | 最大 token 用量（0 = 不限） |
| `tokens_used` | 已消耗 token 数 |
| `rate_limit_per_min` | 每分钟请求上限（0 = 不限） |
| `rate_limit_per_day` | 每天请求上限（0 = 不限） |
| `allowed_upstreams` | 允许使用的上游列表（空 = 全部） |
| `expires_at` | 过期时间（可为空） |

```bash
sqlite3 ocp.db "SELECT name, token, enabled, tokens_used FROM access_keys;"
```
