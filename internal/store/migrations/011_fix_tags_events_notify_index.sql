DROP INDEX idx_tags_events_notif_pending;

CREATE INDEX idx_tags_events_notify_pending
    ON tags_events(created_at, id)
    WHERE notify = 1 AND notif_sent_at IS NULL;
