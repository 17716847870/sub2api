-- This quota is managed in the admin UI, independently of deployment env/config.
INSERT INTO settings (key, value, updated_at)
VALUES ('daily_ip_token_quota', '{"enabled":true,"daily_token_limit":100000000,"timezone":"Asia/Shanghai"}', NOW())
ON CONFLICT (key) DO NOTHING;
