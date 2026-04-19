DROP INDEX IF EXISTS idx_ai_chat_messages_user_order;

ALTER TABLE ai_chat_messages
ALTER COLUMN message_order DROP DEFAULT;

ALTER TABLE ai_chat_messages
DROP COLUMN IF EXISTS message_order;

DROP SEQUENCE IF EXISTS ai_chat_messages_message_order_seq;
