PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO schema_migrations(version) VALUES(1);
CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,totp_secret TEXT,totp_enabled INTEGER NOT NULL DEFAULT 0,enabled INTEGER NOT NULL DEFAULT 1,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE IF NOT EXISTS sessions(id INTEGER PRIMARY KEY,token_hash TEXT NOT NULL UNIQUE,user_id INTEGER NOT NULL,expires_at DATETIME NOT NULL,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,ip_address TEXT,user_agent TEXT,FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);
-- Domain-level routing rules enforced by sing-box (route + dns modules). "applied" is
-- intentionally not tracked: the whole sing-box configuration is regenerated from this
-- table (and geosite_categories/app_settings) on every change, so every enabled row is
-- always reflected the next time the config is (re)generated.
CREATE TABLE IF NOT EXISTS dns_rules(id INTEGER PRIMARY KEY,domain TEXT NOT NULL UNIQUE,action TEXT NOT NULL CHECK(action IN ('direct','proxy-route','block','static')),static_ip TEXT,enabled INTEGER NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX IF NOT EXISTS idx_dns_rules_action ON dns_rules(action,enabled);
-- Geosite categories are resolved directly by sing-box as remote sing-geosite rule-sets
-- (https://github.com/SagerNet/sing-geosite); the panel only stores which tags were
-- selected and how each should be routed, it never downloads or parses geo data itself.
CREATE TABLE IF NOT EXISTS geosite_categories(id INTEGER PRIMARY KEY,tag TEXT NOT NULL UNIQUE,action TEXT NOT NULL DEFAULT 'direct' CHECK(action IN ('direct','proxy-route','block')),selected INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX IF NOT EXISTS idx_geosite_selected ON geosite_categories(selected,action);
-- nftables source allow-lists. "applied" here is meaningful: it tracks whether the rule
-- has actually been pushed into the live nftables set (a real incremental apply step,
-- unlike dns_rules above).
CREATE TABLE IF NOT EXISTS acl_rules(id INTEGER PRIMARY KEY,scope TEXT NOT NULL CHECK(scope IN ('panel','api','dns','socks')),cidr TEXT NOT NULL,ip_version INTEGER NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,applied INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,UNIQUE(scope,cidr));
CREATE INDEX IF NOT EXISTS idx_acl_scope ON acl_rules(scope,enabled);
CREATE TABLE IF NOT EXISTS audit_logs(id INTEGER PRIMARY KEY,user_id INTEGER,action TEXT NOT NULL,details TEXT,ip_address TEXT,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at DESC);
-- Simple key/value store for every operator-tunable runtime setting (DNS upstreams,
-- default routing policy, SOCKS5 enable/listen/credentials, ...). Everything that used
-- to require editing YAML or SSHing into the box lives here and is edited from the
-- Settings page; internal/repository/settings_repo.go documents the known keys.
CREATE TABLE IF NOT EXISTS app_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
