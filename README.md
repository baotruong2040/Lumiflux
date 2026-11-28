# Lumiflux — Realtime Team Chat (Web + Desktop)

Ứng dụng chat nhóm thời gian thực (Discord Clone) xây dựng theo kiến trúc **Go Clean Architecture + PostgreSQL + Redis Pub/Sub** ở Backend và **React + TypeScript + Vite + TanStack Query + Zustand + Tauri 2** ở Frontend (chia sẻ 100% logic giữa Web và Desktop).

---

## 1. Tính năng đã hoàn thiện

### Tính năng cốt lõi (`Must`)
- **Authentication & Token Rotation:** Đăng ký, đăng nhập (`bcrypt`, rate limit IP `10 req/phút`), JWT Access Token (`15m`), Refresh Token xoay vòng nguyên tử (`GETDEL refresh:{hash}`, `7d`), tự động gia hạn single-flight trên `httpClient`.
- **Guild, Membership & Invite:** Tạo server kèm `#general` & `owner` trong một DB transaction, chỉnh sửa/xóa server, phân quyền `owner` / `admin` / `member`, tạo và nhập mã mời (`SELECT ... FOR UPDATE`), rời server (chặn `owner`, hủy subscribe WebSocket theo ADR-007).
- **Channel Management:** CRUD kênh văn bản (`owner` / `admin`), chống trùng tên trong cùng guild (`409 CHANNEL_NAME_TAKEN`), giao diện 3 cột Dark Mode (`ServerBar`, `ChannelSidebar`, `ChatArea`).
- **Messaging & Cursor Pagination:** Gửi tin nhắn (`1–2.000` ký tự Unicode, `nonce` optimistic update, rate limit `rl:{userID}:msg` `30 tin / 10 giây`), tải lịch sử bằng cursor `UUIDv7` (`?before=<id>&limit=50`) giữ nguyên vị trí cuộn, sửa tin nhắn + nhãn `(đã chỉnh sửa)`, xóa mềm, bảo vệ tổ hợp IME tiếng Việt Telex/VNI và `overflow-wrap: anywhere`.
- **WebSocket Gateway & Redis Pub/Sub:** Vé nâng cấp dùng một lần (`POST /api/v1/ws/ticket`, `GETDEL wsticket:{ticket}`), `Hub` actor đơn goroutine (`SendBufferSize=256`, ngắt slow consumer mã `4002`, graceful shutdown `1001`), phát sự kiện qua Redis Pub/Sub `guild:{guildID}` giữa nhiều instance backend, `GatewayClient` (validate Zod, timeout `HEARTBEAT_ACK` đóng `4003`, reconnect exponential backoff + jitter, `RESYNC` từ lần `READY` thứ 2).

### Tính năng mở rộng (`Should`)
- **Typing Indicator:** Sự kiện `TYPING_START` → Redis `SET typing:{channelID}:{userID} 1 NX EX 3` → phát `USER_TYPING` tới các thành viên trong server, thanh hiển thị `... đang nhập` tự tắt sau ~3,5 giây hoặc ngay khi tác giả gửi tin.
- **Presence Online/Offline:** Redis `presence:{userID}` (`TTL 60s`) khởi tạo khi mở kết nối, gia hạn mỗi `HEARTBEAT`, xóa và phát `PRESENCE_UPDATE` `offline` chỉ khi tab cuối cùng của user đóng (hỗ trợ mở nhiều tab song song).
- **Danh sách thành viên Realtime:** Thanh thành viên phân nhóm `Trực tuyến` / `Ngoại tuyến`, tự cập nhật khi có `PRESENCE_UPDATE`, `GUILD_MEMBER_ADD`, `GUILD_MEMBER_REMOVE`.

---

## 2. Cấu trúc Thư mục

```text
Lumiflux/
├── .github/workflows/ci.yml    # GitHub Actions CI (go vet, go test -race, oxlint, vite build)
├── api/openapi.yaml            # Hợp đồng REST API v1
├── docs/                       # Kiến trúc, ADR, kiểm thử & theo dõi tiến độ
├── discord-clone-api/          # Go Backend (Clean Architecture)
│   ├── cmd/
│   │   ├── api/main.go         # Composition root
│   │   ├── seed/main.go        # Seed 1.000 tin nhắn mẫu kiểm thử phân trang
│   │   └── loadtest/main.go    # Đo hiệu năng broadcast WebSocket (p50/p95/p99)
│   ├── deployments/            # docker-compose.yml (PostgreSQL 16 + Redis 7)
│   ├── internal/
│   │   ├── config/             # Đọc biến môi trường & CORS/host defaults
│   │   ├── domain/             # Entities, repository & publisher interfaces
│   │   ├── service/            # Auth, Guild, Invite, Channel, Message, Realtime, Ticket
│   │   ├── repository/         # postgres/ (pgxpool) & redisstore/
│   │   ├── controller/         # HTTP handlers & DTO validation
│   │   ├── router/             # Gin router & middlewares
│   │   ├── gateway/            # Actor Hub, Client, Handler, Redis Subscriber
│   │   └── integration/        # Integration tests trên PostgreSQL & Redis thật
│   └── migrations/             # 0001_init.up.sql / .down.sql
├── discord-clone-app/          # React 19 + TypeScript + Vite + Tauri 2
│   ├── src/
│   │   ├── app/                # Providers & Router
│   │   ├── core/               # api/ (httpClient, endpoints), gateway/ (GatewayClient, Zod), platform/
│   │   ├── features/           # auth, workspace, chat, realtime
│   │   └── theme/tokens.css    # Dark theme đạt chuẩn tương phản WCAG AA (>= 4.5:1)
│   └── src-tauri/              # Tauri 2 Rust shell & cấu hình CSP
├── Makefile                    # Lệnh tắt phát triển, migrate, test, build & loadtest
└── README.md
```

---

## 3. Khởi chạy Môi trường Phát triển

### Yêu cầu cài đặt
- Go `>= 1.24`, Node.js `>= 20`, Rust / Cargo + Tauri 2 prerequisites (WebView2 trên Windows hoặc WebKitGTK trên Linux), Docker Desktop / Docker Engine, và `golang-migrate`.

### Các bước chạy nhanh
1. **Khởi động PostgreSQL & Redis và chạy migration:**
   ```bash
   docker compose -f discord-clone-api/deployments/docker-compose.yml up -d
   cd discord-clone-api
   migrate -path migrations -database "postgres://discord:discord_secret_dev@localhost:5432/discord_clone?sslmode=disable" up
   ```
2. **Chạy Backend (`http://127.0.0.1:8080`):**
   ```bash
   cd discord-clone-api
   go run ./cmd/api
   ```
   *(Mặc định `HOST=127.0.0.1` khi `APP_ENV=development` để không bật cảnh báo Windows Defender Firewall; truyền `PORT=8081` nếu cần chạy instance thứ 2.)*
3. **Chạy Frontend Web (`http://127.0.0.1:5173`) hoặc Desktop (Tauri 2):**
   ```bash
   cd discord-clone-app
   npm install
   npm run dev        # Trình duyệt Web
   npx tauri dev      # Cửa sổ Desktop Tauri 2
   ```
4. **Đóng gói Desktop (`npx tauri build`):**
   ```bash
   cd discord-clone-app
   npx tauri build
   ```

---

## 4. Kiểm thử & Đo hiệu năng (Loadtest)

- **Backend unit & race tests:**
  ```bash
  cd discord-clone-api
  go vet ./...
  go test -race -count=1 ./...
  ```
- **Integration tests (yêu cầu PostgreSQL & Redis đang chạy):**
  ```bash
  cd discord-clone-api
  RUN_INTEGRATION_TESTS=1 go test -v -race -count=1 ./internal/integration/...
  ```
- **Frontend lint & production build:**
  ```bash
  cd discord-clone-app
  npm run lint
  npm run build
  ```
- **Chạy loadtest broadcast WebSocket (`2`, `10`, `100`, `500` client):**
  ```bash
  cd discord-clone-api
  go run ./cmd/loadtest -clients 100 -messages 20
  ```
  Số liệu đo đạc chi tiết xem tại [docs/testing.md](docs/testing.md).

---

## 5. Tài liệu tham khảo
- Kiến trúc hệ thống: [docs/architecture.md](docs/architecture.md)
- Quyết định kiến trúc (ADR-001..014): [docs/decisions.md](docs/decisions.md)
- Đặc tả API & WebSocket: [docs/api.md](docs/api.md) và [api/openapi.yaml](api/openapi.yaml)
- Kế hoạch kiểm thử & số liệu độ trễ: [docs/testing.md](docs/testing.md)
- Tiến độ Sprint 14 ngày: [docs/14_day_sprint_progress_tracker.md](docs/14_day_sprint_progress_tracker.md) và [docs/progress.md](docs/progress.md)
