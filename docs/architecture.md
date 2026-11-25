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
Lumiflux/
├── discord-clone-api/               # Go Backend
│   ├── cmd/
│   │   ├── api/main.go              # Composition root: khởi tạo & nối các layer
│   │   └── loadtest/main.go         # Bắn tải WebSocket, ghi p50/p95/p99 cho báo cáo
│   ├── internal/
│   │   ├── domain/                  # Entities, Repository & Event interfaces, Domain errors
│   │   ├── service/                 # Business logic (auth, guild, channel, message, typing)
│   │   ├── repository/
│   │   │   ├── postgres/            # PostgreSQL implementations (pgx v5)
│   │   │   └── redisstore/          # TokenStore, TicketStore, Publisher, PresenceStore
│   │   ├── controller/              # HTTP handlers & dto/ (request/response + validation)
│   │   ├── router/                  # Route definitions & middlewares (auth, cors, ratelimit, request_id, recover)
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
│   │   │   │   └── events.ts        # Type + zod schema cho frame WS
│   │   │   └── platform/            # StorageAdapter (Web & Tauri)
│   │   ├── features/                # auth, guilds, channels, messages, members, realtime
│   │   ├── ui/                      # Shared UI components (Button, Modal, Avatar...)
│   │   ├── theme/                   # tokens.css (Dark mode CSS variables)
│   │   └── styles/                  # global.css
│   ├── src-tauri/                   # Tauri 2 Rust shell & tauri.conf.json
│   └── .env.development             # VITE_API_URL, VITE_WS_URL
├── .github/workflows/ci.yml         # CI chạy go vet, go test -race
├── docs/                            # Tài liệu quản lý dự án & kiểm thử
├── api/openapi.yaml                 # Hợp đồng REST API v1
├── Makefile                         # Lệnh tắt chạy dev, migrate, test, loadtest
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

Nếu bước 4 (`PUBLISH`) lỗi, service ghi log mức Warn và vẫn trả `201`: tin đã nằm trong PostgreSQL, client lấy lại qua RESYNC.

### Nguyên tắc Concurrency của WebSocket Hub
- **Actor Model:** Chỉ duy nhất goroutine chạy `Hub.Run(ctx)` được phép đọc/ghi 3 map (`clients`, `users`, `guilds`), loại bỏ hoàn toàn mutex và nguy cơ deadlock/data race.
- **2 Goroutine mỗi kết nối:** `readPump` (đọc frame từ client, cập nhật deadline pong) và `writePump` (ghi frame từ channel đệm `send` 256 phần tử ra socket).
- **Chống Slow Consumer:** Nếu channel `send` của một client bị đầy, Hub không chờ mà ngắt kết nối client đó với mã đóng riêng `4002` để bảo vệ toàn hệ thống; client sẽ tự động kết nối lại và `RESYNC`.
- **Presence & nhiều thiết bị:** map `users` (user → các kết nối) cho phép Hub biết khi nào kết nối cuối cùng của một user đóng, để phát `PRESENCE_UPDATE` `offline`.
- **Tắt server êm:** `signal.NotifyContext` (SIGINT/SIGTERM) hủy `ctx` → `Hub.Run` đóng mọi kết nối bằng mã `1001` → `http.Server.Shutdown` với timeout 10 giây (`Shutdown` không theo dõi các kết nối đã bị hijack).

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
| `rl:{userID}:msg` | Counter (`INCR`) | 10 giây | Giới hạn 30 tin / 10 giây / user (requirements §2.0) |
| `rl:login:{ip}` | Counter (`INCR`) | 60 giây | Giới hạn 10 lần đăng nhập / phút / IP |
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
  - `GUILD_DELETE`: `{ guildId }` — owner xóa server; Hub gỡ guild khỏi map `guilds`, client xóa server khỏi thanh bên. Đổi tên/icon server không phát sự kiện (client thấy sau lần tải lại danh sách).
  - `CHANNEL_CREATE` / `CHANNEL_UPDATE` / `CHANNEL_DELETE`: `{ guildId, channel }`
  - `HEARTBEAT_ACK`: Phản hồi tín hiệu sống
  - `ERROR`: `{ code, message }`
- **Client $\rightarrow$ Server:**
  - `HEARTBEAT`: Gửi định kỳ mỗi `heartbeatIntervalMs` (25 giây)
  - `TYPING_START`: `{ channelId }` (tối đa 1 lần mỗi 3 giây)
- **Mã đóng kết nối:**
  - `1000`: đóng bình thường; `1001`: server tắt (Hub đóng mọi kết nối khi `ctx` bị hủy).
  - `4002`: client đọc quá chậm (tràn hàng đợi gửi) — server đóng; client kết nối lại và `RESYNC`.
  - `4003`: quá 10 giây không nhận `HEARTBEAT_ACK` — **client** chủ động đóng, rồi kết nối lại và `RESYNC`.
  - Vé không hợp lệ hoặc hết hạn: server trả HTTP `401` ngay ở bước nâng cấp, trước khi có WebSocket; client lấy vé mới và thử lại theo backoff. Mã `4001` hiện không dùng.