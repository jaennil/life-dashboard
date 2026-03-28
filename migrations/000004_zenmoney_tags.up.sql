CREATE TABLE zenmoney_tags (
    id VARCHAR(36) PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    parent_id VARCHAR(36),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
