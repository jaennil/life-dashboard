CREATE SEQUENCE IF NOT EXISTS ai_chat_messages_message_order_seq;

ALTER TABLE ai_chat_messages
ADD COLUMN IF NOT EXISTS message_order BIGINT;

WITH ordered AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            ORDER BY created_at ASC, CASE role WHEN 'user' THEN 0 ELSE 1 END ASC, id ASC
        ) AS rn
    FROM ai_chat_messages
    WHERE message_order IS NULL
)
UPDATE ai_chat_messages m
SET message_order = ordered.rn
FROM ordered
WHERE m.id = ordered.id;

SELECT setval(
    'ai_chat_messages_message_order_seq',
    COALESCE((SELECT MAX(message_order) FROM ai_chat_messages), 0) + 1,
    false
);

ALTER TABLE ai_chat_messages
ALTER COLUMN message_order SET DEFAULT nextval('ai_chat_messages_message_order_seq');

ALTER SEQUENCE ai_chat_messages_message_order_seq
OWNED BY ai_chat_messages.message_order;

ALTER TABLE ai_chat_messages
ALTER COLUMN message_order SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ai_chat_messages_user_order
ON ai_chat_messages(user_id, message_order);
