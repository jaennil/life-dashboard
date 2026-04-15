ALTER TABLE ai_chat_messages
    ALTER COLUMN created_at SET DEFAULT clock_timestamp();
