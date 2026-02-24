-- Memory store tables for Postgres/Supabase backend.
-- Run this in Supabase SQL Editor or via migration.

CREATE TABLE IF NOT EXISTS memories (
    id              TEXT PRIMARY KEY,
    text            TEXT NOT NULL,
    embedding       BYTEA,
    source          TEXT DEFAULT '',
    session_id      TEXT DEFAULT '',
    metadata        JSONB DEFAULT '{}',
    decay_level     INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL,
    last_referenced TIMESTAMPTZ NOT NULL,
    access_count    INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS memory_tags (
    memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    tag       TEXT NOT NULL,
    PRIMARY KEY (memory_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);
CREATE INDEX IF NOT EXISTS idx_memories_decay ON memories(decay_level);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
CREATE INDEX IF NOT EXISTS idx_memories_referenced ON memories(last_referenced);
