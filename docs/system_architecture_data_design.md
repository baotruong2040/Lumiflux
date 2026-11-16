# Tài liệu Kiến trúc Hệ thống (`architecture.md`)

---

## 1. Tổng quan Kiến trúc & Quy tắc Phụ thuộc

Hệ thống gồm hai ứng dụng độc lập giao tiếp qua **REST API (JSON)** và **WebSocket**:
- **Backend (`discord-clone-api`):** Viết bằng Go theo mô hình **Clean Architecture**. Quy tắc bất biến: *mũi tên phụ thuộc chỉ hướng vào trong*.
  ```
  router ──► controller ──► service ──► domain ◄── repository
  gateway (WebSocket) ──────┘
  ```
  - `domain`: Chứa struct thực thể (`User`, `Guild`, `Channel`, `Message`, `Event`) và các interface repository/publisher. Không import thư viện ngoài (`gin`, `pgx`, `redis`).
  - `service`: Chứa toàn bộ quy tắc nghiệp vụ, kiểm tra phân quyền, điều phối repository và phát sự kiện. Không biết `gin.Context` hay câu lệnh SQL.
  - `repository`: Cài đặt chi tiết truy vấn PostgreSQL (`pgx`) và Redis (`go-redis`).
  - `controller` & `gateway`: Tầng giao tiếp bên ngoài (HTTP/WebSocket), parse dữ liệu đầu vào, gọi `service` và ánh xạ lỗi domain sang mã HTTP/WS.
- **Frontend (`discord-clone-app`):** React + TypeScript + Vite, đóng gói Desktop bằng Tauri 2. Logic nghiệp vụ, HTTP client (`TanStack Query`), state (`Zustand`) và `GatewayClient` dùng chung 100% giữa Web và Desktop.

---

## 2. Cấu trúc Thư mục Dự án

```text
discord-clone/
├── discord-clone-api/               # Go Backend
│   ├── cmd/api/main.go              # Composition root: khởi tạo & nối các layer
│   ├── internal/
│   │   ├── domain/                  # Entities, Repository & Event interfaces, Domain errors
│   │   ├── service/                 # Business logic (auth, guild, channel, message, typing)
│   │   ├── repository/
│   │   │   ├── postgres/            # PostgreSQL implementations (pgx v5)
│   │   │   └── redisstore/          # TokenStore, TicketStore, Publisher, PresenceStore
│   │   ├── controller/              # HTTP handlers & DTOs
│   │   ├── router/                  # Route definitions & middlewares (auth, cors, recover)
│   │   ├── gateway/                 # WebSocket Hub, Client, Handler, Inbound, Subscriber
│   │   └── config/                  # Env config loader
│   ├── pkg/                         # jwtutil, hasher, logger, idgen (UUIDv7)
│   ├── migrations/                  # 0001_init.up.sql, 0001_init.down.sql
│   └── deployments/                 # docker-compose.yml (PostgreSQL + Redis)
├── discord-clone-app/               # React + TypeScript + Tauri 2
│   ├── src/
│   │   ├── app/                     # App root, router, providers
│   │   ├── core/
│   │   │   ├── api/                 # httpClient (auto refresh token), endpoints
│   │   │   ├── gateway/             # GatewayClient (heartbeat, backoff, resync)
│   │   │   └── platform/            # StorageAdapter (Web & Tauri)
│   │   ├── features/                # auth, guilds, channels, messages, members, realtime
│   │   ├── ui/                      # Shared UI components (Button, Modal, Avatar...)
│   │   ├── theme/                   # tokens.css (Dark mode CSS variables)
│   │   └── styles/                  # global.css
│   └── src-tauri/                   # Tauri 2 Rust shell & tauri.conf.json
├── docs/                            # Tài liệu quản lý dự án & kiểm thử
├── api/openapi.yaml                 # Hợp đồng REST API v1
└── README.md
```

---

## 3. Luồng Dữ liệu Thời gian thực (Message & WebSocket Hub)

```text
Client A (React/Tauri)
   │
   │ 1. POST /api/v1/channels/:id/messages { content, nonce }
   ▼
MessageController ──► MessageService
                           │
                           ├─► 2. Kiểm tra membership & validate (1..2000 ký tự)
                           ├─► 3. PostgreSQL: INSERT INTO messages (UUIDv7)
                           └─► 4. Redis: PUBLISH guild:{guildID} (JSON frame)
                                        │
                                        ▼
                               Gateway Subscriber (mọi instance BE)
                                        │
                                        ▼
                               Hub.Run (Single-goroutine Actor)
                                 ├─► Duyệt map guilds[guildID]
                                 └─► c.Enqueue(frame) (non-blocking)
                                        │
                         ┌──────────────┴──────────────┐
                         ▼                             ▼
                 Client A (writePump)          Client B (writePump)
                 (thay tin tạm qua nonce)      (hiển thị tin mới)
```

### Nguyên tắc Concurrency của WebSocket Hub
- **Actor Model:** Chỉ duy nhất goroutine chạy `Hub.Run(ctx)` được phép đọc/ghi 3 map (`clients`, `users`, `guilds`), loại bỏ hoàn toàn mutex và nguy cơ deadlock/data race.
- **2 Goroutine mỗi kết nối:** `readPump` (đọc frame từ client, cập nhật deadline pong) và `writePump` (ghi frame từ channel đệm `send` 256 phần tử ra socket).
- **Chống Slow Consumer:** Nếu channel `send` của một client bị đầy, Hub không chờ mà ngắt kết nối client đó với mã đóng riêng `4002` để bảo vệ toàn hệ thống; client sẽ tự động kết nối lại và `RESYNC`.

---

## 4. Thiết kế Cơ sở dữ liệu (PostgreSQL & Redis)

### 4.1. Sáu bảng cốt lõi PostgreSQL
1. `users`: `id (UUID PK)`, `username (CITEXT UNIQUE)`, `email (CITEXT UNIQUE)`, `display_name`, `password_hash`, `avatar_url`, `created_at`.
2. `guilds`: `id (UUID PK)`, `name`, `icon_url`, `owner_id (FK -> users.id)`, `created_at`.
3. `guild_members`: `guild_id (FK)`, `user_id (FK)`, `nickname`, `role ('owner'|'admin'|'member')`, `joined_at` — `PRIMARY KEY (guild_id, user_id)`, Index trên `(user_id)`.
4. `channels`: `id (UUID PK)`, `guild_id (FK)`, `name`, `type (SMALLINT, 0=text)`, `topic`, `position`, `created_at` — `UNIQUE (guild_id, name)`.
5. `messages`: `id (UUID PK - UUIDv7)`, `channel_id (FK)`, `author_id (FK)`, `content (CHECK 1..2000)`, `edited_at`, `deleted_at`, `created_at` — Index trên `(channel_id, id DESC)`.
6. `invites`: `code (VARCHAR(12) PK)`, `guild_id (FK)`, `created_by (FK)`, `max_uses`, `uses`, `expires_at`, `created_at`.

### 4.2. Cấu trúc Key trên Redis
| Key / Channel | Kiểu dữ liệu | TTL | Mục đích |
| :--- | :--- | :--- | :--- |
| `refresh:{hash}` | String (`userID`) | 7 ngày | Lưu hash của refresh token để xoay vòng và thu hồi |
| `wsticket:{ticket}` | String (`userID`) | 30 giây | Vé nâng cấp WebSocket dùng một lần (`GETDEL`) |
| `presence:{userID}` | String (`online`) | 60 giây | Trạng thái trực tuyến, gia hạn mỗi lần nhận `HEARTBEAT` |
| `typing:{channelID}:{userID}` | String (`1`) | 3 giây | Rate limit sự kiện `TYPING_START` (`SET NX EX 3`) |
| `rl:{userID}:msg` | Counter (`INCR`) | 10 giây | Giới hạn tốc độ gửi tin nhắn |
| `guild:{guildID}` | Pub/Sub Channel | Không | Kênh truyền sự kiện realtime tới mọi instance backend |

---

## 5. Giao thức Sự kiện WebSocket

Mọi bản tin WebSocket tuân theo phong bì JSON: `{"t": "<EVENT_NAME>", "d": <PAYLOAD>}`.

- **Server $\rightarrow$ Client:**
  - `READY`: `{ userId, guildIds, heartbeatIntervalMs }`
  - `MESSAGE_CREATE`: `{ id, channelId, guildId, author, content, nonce, createdAt }`
  - `MESSAGE_UPDATE`: `{ id, channelId, guildId, content, editedAt }`
  - `MESSAGE_DELETE`: `{ id, channelId, guildId }`
  - `USER_TYPING`: `{ channelId, guildId, userId, username, timestamp }`
  - `PRESENCE_UPDATE`: `{ userId, status }` (`online` | `offline`)
  - `GUILD_MEMBER_ADD` / `GUILD_MEMBER_REMOVE`: `{ guildId, user }`
  - `CHANNEL_CREATE` / `CHANNEL_UPDATE` / `CHANNEL_DELETE`: `{ guildId, channel }`
  - `HEARTBEAT_ACK`: Phản hồi tín hiệu sống
  - `ERROR`: `{ code, message }`
- **Client $\rightarrow$ Server:**
  - `HEARTBEAT`: Gửi định kỳ mỗi `heartbeatIntervalMs` (25 giây)
  - `TYPING_START`: `{ channelId }` (tối đa 1 lần mỗi 3 giây)
- **Mã đóng kết nối riêng (Custom Close Codes):**
  - `4001`: Vé WS không hợp lệ hoặc đã hết hạn.
  - `4002`: Client đọc quá chậm (tràn hàng đợi gửi).
  - `4003`: Quá hạn phản hồi `HEARTBEAT_ACK`.