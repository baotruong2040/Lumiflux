CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            UUID PRIMARY KEY,
    username      CITEXT      NOT NULL UNIQUE,
    email         CITEXT      NOT NULL UNIQUE,
    display_name  VARCHAR(64) NOT NULL,
    password_hash TEXT        NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guilds (
    id         UUID PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    icon_url   TEXT,
    owner_id   UUID         NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE guild_members (
    guild_id  UUID        NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    nickname  VARCHAR(64),
    role      VARCHAR(16) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

-- Truy vấn: user này đang ở những guild nào
CREATE INDEX idx_guild_members_user ON guild_members (user_id);

CREATE TABLE channels (
    id         UUID PRIMARY KEY,
    guild_id   UUID         NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    type       SMALLINT     NOT NULL DEFAULT 0, -- 0 = text, 1 = voice (để dành)
    topic      VARCHAR(255),
    position   INT          NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);

CREATE TABLE messages (
    id         UUID PRIMARY KEY, -- UUIDv7: sắp xếp được theo thời gian
    channel_id UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_id  UUID        NOT NULL REFERENCES users(id),
    content    TEXT        NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    edited_at  TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ, -- xóa mềm
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Phân trang cursor: WHERE channel_id = $1 AND id < $2 ORDER BY id DESC LIMIT 50
CREATE INDEX idx_messages_channel_id ON messages (channel_id, id DESC);

CREATE TABLE invites (
    code       VARCHAR(12) PRIMARY KEY,
    guild_id   UUID        NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    created_by UUID        NOT NULL REFERENCES users(id),
    max_uses   INT,
    uses       INT         NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);