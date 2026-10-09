-- Extra gateway admission rule, configured entirely through the admin UI.
INSERT INTO settings (key, value, updated_at)
VALUES ('gateway_header_authentication', '{"enabled":true,"header_name":"User-Agent","required_substring":"XundaAI"}', NOW())
ON CONFLICT (key) DO NOTHING;
