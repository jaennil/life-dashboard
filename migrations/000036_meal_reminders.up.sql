-- One row per meal the user was reminded about, so a reminder is sent once and
-- not on every pass of the scheduler. The day is stored as the local date the
-- meal belongs to, which is also what makes "уже напоминал сегодня" answerable
-- without reading the clock twice.
CREATE TABLE meal_reminders (
    user_id UUID NOT NULL REFERENCES users(id),
    date DATE NOT NULL,
    meal VARCHAR(20) NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, date, meal)
);
