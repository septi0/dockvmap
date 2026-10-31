ALTER TABLE images ADD COLUMN last_requested_at DATETIME;
ALTER TABLE images ADD COLUMN last_pulled_at DATETIME;
ALTER TABLE images ADD COLUMN last_pulled_tag TEXT;

ALTER TABLE proxy_tokens ADD COLUMN last_used_at DATETIME;
