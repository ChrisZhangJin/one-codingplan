# one-codingplan (ocp)

ocp 将多个 AI 编程计划的凭证（Minimax、Kimi、Qwen、Mimo 等）聚合在单一兼容 OpenAI 和 Anthropic 的端点后面。将你的工具指向一个 URL、使用一个密钥——ocp 透明地处理路由、故障转移和用量追踪。

![Portal](./img/Portal.jpg)

---

## 快速开始

### 1. 配置

复制并编辑配置文件：

```bash
cp config.yaml.example config.yaml
```

关键字段：

```yaml
server:
  port: 9189
  admin_key: "changeme123"   # 管理面板与管理 API 的密码

database:
  path: "./ocp.db"

upstreams:
  - name: minimax
    base_url: https://api.minimaxi.com/anthropic
    api_key: "sk-..."
    enabled: true
```

### 2. 构建与运行

```bash
make build
OCP_ENCRYPTION_KEY=<16字符密钥> ./ocp --config config.yaml
```

`OCP_ENCRYPTION_KEY` 用于加密数据库中的上游 API Key，长度必须为 16、24 或 32 个字符。

### 3. 打开管理面板

浏览器访问 **http://localhost:9189**，使用 `admin_key` 登录。

---

## 管理 API

所有管理接口需要 `Authorization: Bearer <admin_key>` 请求头。

### 上游管理

```bash
# 查看所有上游及健康状态
curl http://localhost:9189/api/upstreams \
  -H "Authorization: Bearer changeme123"

# 切换上游启用/禁用（id 从列表获取）
curl -X POST http://localhost:9189/api/upstreams/1/toggle \
  -H "Authorization: Bearer changeme123"

# 强制轮换到下一个可用上游
curl -X POST http://localhost:9189/api/upstreams/rotate \
  -H "Authorization: Bearer changeme123"
```

### 访问密钥管理

```bash
# 查看所有密钥
curl http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123"

# 创建密钥
curl -X POST http://localhost:9189/api/keys \
  -H "Authorization: Bearer changeme123" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key", "token_budget": 1000000, "rate_limit_per_minute": 60}'

# 封禁密钥
curl -X POST http://localhost:9189/api/keys/<id>/block \
  -H "Authorization: Bearer changeme123"

# 解封密钥
curl -X POST http://localhost:9189/api/keys/<id>/unblock \
  -H "Authorization: Bearer changeme123"
```

### 使用访问密钥发送请求

```bash
# OpenAI 兼容接口
curl http://localhost:9189/v1/chat/completions \
  -H "Authorization: Bearer ocp-<你的密钥>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "你好"}]
  }'

# Anthropic 兼容接口
curl http://localhost:9189/v1/messages \
  -H "Authorization: Bearer ocp-<你的密钥>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

---

## 数据库初始化

ocp 使用 SQLite，首次启动时会通过 GORM `AutoMigrate` 自动创建表结构，无需手动操作。

如需手动初始化数据库（例如在干净环境或 CI 中），可使用提供的 `init.sql`：

```bash
sqlite3 ocp.db < init.sql
```

`init.sql` 包含：
- 完整的表和索引定义（使用 `CREATE TABLE IF NOT EXISTS`、`CREATE INDEX IF NOT EXISTS`）
- 所有支持的上游提供商种子数据（API Key 留空）

初始化后，通过管理面板（**Upstream Status → Edit**）或管理 API 设置真实的 API Key：

```bash
curl -X PATCH http://localhost:9189/api/upstreams/<id> \
  -H "Authorization: Bearer changeme123" \
  -H "Content-Type: application/json" \
  -d '{"api_key": "your-real-key"}'
```

> **注意：** 在已有数据库上重复执行 `init.sql` 是安全的——所有插入语句均使用 `INSERT OR IGNORE`。

---

## 访问密钥错误码

| 情况 | HTTP 状态码 | 错误信息 |
|------|-------------|----------|
| Token 缺失或不存在 | 401 Unauthorized | `unauthorized` |
| 密钥已被禁用/封禁 | 403 Forbidden | `key disabled` |
| 密钥已过期 | 403 Forbidden | `key expired` |
| Token 用量超出预算 | 429 Too Many Requests | `token budget exceeded` |
| 每分钟请求频率超限 | 429 Too Many Requests | `per-minute rate limit exceeded` |
| 每日请求频率超限 | 429 Too Many Requests | `per-day rate limit exceeded` |

---

## 数据库结构

数据库为 SQLite，路径由 `config.yaml` 中 `database.path` 配置（默认 `./ocp.db`）。

### `upstreams` 表 — 上游提供商

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | INTEGER | 主键 |
| `name` | TEXT | 提供商名称（如 `minimax`、`kimi`） |
| `base_url` | TEXT | API 基础地址 |
| `api_key_enc` | BLOB | 加密存储的 API Key |
| `enabled` | BOOLEAN | 是否启用（面板开关控制） |
| `available` | BOOLEAN | 运行时健康状态（限速冷却期间为 false） |
| `created_at` | DATETIME | 创建时间 |
| `updated_at` | DATETIME | 更新时间 |

```bash
sqlite3 ocp.db "SELECT id, name, enabled FROM upstreams;"
```

### `access_keys` 表 — 访问密钥

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | TEXT (UUID) | 主键 |
| `name` | TEXT | 密钥名称 |
| `token` | TEXT | 客户端使用的 Bearer Token（`ocp-...`） |
| `enabled` | BOOLEAN | false 表示已封禁 |
| `token_budget` | INTEGER | 最大 Token 用量（0 = 不限） |
| `rate_limit_per_minute` | INTEGER | 每分钟请求上限（0 = 不限） |
| `rate_limit_per_day` | INTEGER | 每天请求上限（0 = 不限） |
| `allowed_upstreams` | TEXT | 允许使用的上游列表，JSON 数组（空 = 全部） |
| `expires_at` | DATETIME | 过期时间（可为空） |
| `created_at` | DATETIME | 创建时间 |
| `updated_at` | DATETIME | 更新时间 |

```bash
sqlite3 ocp.db "SELECT name, token, enabled FROM access_keys;"
```
