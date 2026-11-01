ALTER TABLE images ADD COLUMN current_digest TEXT;
ALTER TABLE images ADD COLUMN current_digest_tag TEXT;
ALTER TABLE images ADD COLUMN current_digest_since DATETIME;
