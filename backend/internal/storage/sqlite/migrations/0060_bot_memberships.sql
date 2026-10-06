-- +goose Up
-- A bot takes part in a group through a thread of its own; seen is the seq of
-- the last group message shown to it there.
CREATE TABLE IF NOT EXISTS bot_memberships (
  group_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  thread_id TEXT NOT NULL UNIQUE,
  seen INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (group_id, bot_id),
  FOREIGN KEY (group_id) REFERENCES threads(id) ON DELETE CASCADE,
  FOREIGN KEY (bot_id) REFERENCES threads(id) ON DELETE CASCADE,
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS bot_memberships;
