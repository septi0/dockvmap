CREATE INDEX idx_audit_logs_type_created_at
    ON audit_logs(type, created_at DESC, id DESC);

CREATE INDEX idx_image_tags_image_id_order
    ON image_tags(image_id, tag_order);
