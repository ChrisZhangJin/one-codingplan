-- one-codingplan (ocp) database init script
-- Generated from ocp.db schema

CREATE TABLE IF NOT EXISTS `upstreams` (
  `id`             integer PRIMARY KEY AUTOINCREMENT,
  `created_at`     datetime,
  `updated_at`     datetime,
  `name`           text    NOT NULL,
  `base_url`       text    NOT NULL,
  `api_key_enc`    blob,
  `enabled`        numeric NOT NULL DEFAULT true,
  `model_override` text    NOT NULL DEFAULT ""
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_upstreams_name` ON `upstreams`(`name`);

-- Upstream seed data (api_key_enc left null — set real keys via portal or PATCH /api/upstreams/:id)
INSERT OR IGNORE INTO `upstreams` (`name`, `base_url`, `api_key_enc`, `enabled`, `model_override`, `created_at`, `updated_at`) VALUES
  ('minimax', 'https://api.minimaxi.com',                    NULL, 0, 'MiniMax-M2.5', datetime('now'), datetime('now')),
  ('kimi',    'https://api.kimi.com/coding',                 NULL, 0, '',             datetime('now'), datetime('now')),
  ('qwen',    'https://dashscope.aliyuncs.com',              NULL, 0, '',             datetime('now'), datetime('now')),
  ('mimo',    'https://token-plan-cn.xiaomimimo.com',        NULL, 0, 'mimo-v2-pro',  datetime('now'), datetime('now')),
  ('deepseek','https://api.deepseek.com',                    NULL, 1, 'deepseek-chat',datetime('now'), datetime('now'))
;

CREATE TABLE IF NOT EXISTS `access_keys` (
  `id`                   text    PRIMARY KEY,
  `created_at`           datetime,
  `updated_at`           datetime,
  `token`                text    NOT NULL,
  `enabled`              numeric NOT NULL DEFAULT true,
  `name`                 text    NOT NULL DEFAULT "",
  `token_budget`         integer NOT NULL DEFAULT 0,
  `allowed_upstreams`    text    NOT NULL DEFAULT "",
  `expires_at`           datetime,
  `rate_limit_per_minute` integer NOT NULL DEFAULT 0,
  `rate_limit_per_day`   integer NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_access_keys_token` ON `access_keys`(`token`);

CREATE TABLE IF NOT EXISTS `usage_records` (
  `id`            integer PRIMARY KEY AUTOINCREMENT,
  `created_at`    datetime,
  `key_id`        text    NOT NULL,
  `upstream_id`   integer NOT NULL,
  `upstream_name` text    NOT NULL DEFAULT "",
  `input_tokens`  integer,
  `output_tokens` integer,
  `latency_ms`    integer,
  `success`       numeric
);

CREATE INDEX IF NOT EXISTS `idx_usage_records_key_id`      ON `usage_records`(`key_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_records_upstream_id` ON `usage_records`(`upstream_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_records_created_at`  ON `usage_records`(`created_at`);
