# Theo dõi Tiến độ 14 Ngày (`14_day_sprint_progress_tracker.md`)

**Mốc thời gian:** 16/11/2026 – 29/11/2026  
**Nguyên tắc:** Chỉ đánh dấu `[x]` khi có bằng chứng chạy thực tế hoặc vượt qua bài kiểm thử của ngày đó.

---

## Tuần 1: Nền tảng, Database, Auth & Quản lý Server/Kênh

- [x] **Ngày 1 (16/11/2026) — Phân tích yêu cầu & Thiết lập tài liệu**
  - *Mục tiêu:* Chốt phạm vi MoSCoW, quy tắc nghiệp vụ, khởi tạo `docs/` và `api/openapi.yaml`.
  - *Bằng chứng:* Hoàn thiện 6 file tài liệu nền tảng và xác nhận quyết định kỹ thuật.
- [x] **Ngày 2 (17/11/2026) — Chuẩn bị môi trường trên CachyOS**
  - *Mục tiêu:* Cài đặt và kiểm tra phiên bản `git`, `go`, `node`, `npm`, `rustup`, `docker`, `docker compose`, thư viện hệ thống Tauri 2 (`webkit2gtk-4.1`...); khởi tạo repo `discord-clone`.
  - *Bằng chứng:* Tất cả lệnh `--version` và `pkg-config` trả về thành công.
- [ ] **Ngày 3 (18/11/2026) — Khởi tạo Frontend, Backend & Desktop Shell**
  - *Mục tiêu:* Chạy được React + Vite trên trình duyệt, cửa sổ Tauri (`npx tauri dev`), và cấu trúc module Go (`go.mod`).
  - *Bằng chứng:* Mở được cửa sổ Desktop Tauri hiển thị giao diện React mặc định.
- [x] **Ngày 4 (19/11/2026) — Thiết kế & Triển khai Database**
  - *Mục tiêu:* Chạy PostgreSQL & Redis qua `docker-compose.yml`; viết và chạy `0001_init.up.sql` / `.down.sql` với 6 bảng, ràng buộc và index.
  - *Bằng chứng:* Chạy `migrate up` và `migrate down` thành công, kiểm tra constraint không cho phép trùng username/email.
- [x] **Ngày 5 (20/11/2026) — Xây dựng Nền tảng Backend**
  - *Mục tiêu:* Hoàn thiện `config`, `slog` logger, kết nối `pgxpool` & `go-redis`, router Gin, middleware cơ bản (`CORS`, `Recover`, `RequestID`), endpoint `/healthz` và `/readyz`.
  - *Bằng chứng:* Dựng hoàn chỉnh cấu trúc Clean Architecture (`internal/config`, `pkg/logger`, `pkg/idgen`, `pkg/hasher`, `pkg/jwtutil`, `internal/domain`, `internal/repository/postgres`, `internal/repository/redisstore`, `internal/router`); `GET /healthz` và `GET /readyz` trả HTTP `200` (`postgres: ok`, `redis: ok`); kiểm thử CORS đủ 4 origin Web + Tauri và `go vet ./...` / `go test -race ./...` xanh.
- [x] **Ngày 6 (21/11/2026) — Xây dựng Authentication Backend**
  - *Mục tiêu:* Hoàn thiện `register`, `login` (kèm rate limit đăng nhập `429`), `refresh` (xoay vòng token trong Redis), `logout`, auth middleware, và `GET/PATCH /users/@me`.
  - *Bằng chứng:* Hoàn thiện toàn bộ luồng Auth theo `api/openapi.yaml`; unit test `AuthService` (kể cả concurrent refresh race) và `router` pass; integration test `TC-AUTH-01..06` (phần backend) chạy trên PostgreSQL + Redis thật đều pass với `go test -race ./...`.
- [x] **Ngày 7 (22/11/2026) — Xây dựng Authentication Frontend**
  - *Mục tiêu:* Màn hình Login/Register, `httpClient` tự động refresh token khi gặp `401`, Auth store, Route Guard và `StorageAdapter` dùng chung Web/Tauri.
  - *Bằng chứng:* Dựng `src/theme/tokens.css` (`discord-night`), `src/core/platform/` (`StorageAdapter`), `src/core/api/` (`httpClient` single-flight auto-refresh 401 + retry), `useAuthStore`, `ProtectedRoute`/`PublicOnlyRoute`, `LoginPage`/`RegisterPage`; `npm run lint` (0 cảnh báo) & `npm run build` (`tsc -b && vite build`) xanh; unit test `httpClient` + `StorageAdapter` (3/3 pass), kiểm thử `TC-AUTH-05` trực tiếp với API `:8080`, kiểm thử Web E2E (`http://localhost:5173`: đăng ký, F5 giữ phiên, `@me`, đăng xuất, đăng nhập) pass, và `cargo check` Tauri 2 shell xanh *(chạy tương tác cửa sổ GUI `npx tauri dev` gộp cùng kiểm xác nhận Ngày 3)*.

---

## Tuần 2: Chat REST, WebSocket Realtime, Tính năng Should & Đóng gói

- [x] **Ngày 8 (23/11/2026) — Xây dựng Guild & Invite Backend**
  - *Mục tiêu:* CRUD Guild (transaction tạo guild + `#general` + `owner`), danh sách thành viên, rời server (chặn `owner`), tạo và chấp nhận mã mời (`POST /invites/:code/accept`, mã mời hết hạn/hết lượt trả `400`); gán `admin` bằng SQL khi test.
  - *Bằng chứng:* Hoàn thiện `GuildService`, `InviteService`, `GuildRepository`/`InviteRepository` (`pgx` transaction + `SELECT ... FOR UPDATE`), `PresenceStore` (`MGET`), interface/hook `GuildSubscriptionManager.Unsubscribe` (ADR-007), và controller/router; unit test + `router_test.go` + integration test thật trên PostgreSQL/Redis (`TC-GLD-01..04`) pass với `go test -race -count=1 ./...` & `go vet ./...`.
- [x] **Ngày 9 (24/11/2026) — Xây dựng Channel & Giao diện Server**
  - *Mục tiêu:* API CRUD Channel (`owner`/`admin`), dựng layout 3 cột Dark Mode (`ServerBar`, `ChannelSidebar`, `ChatArea`), luồng tạo/tham gia server và chọn kênh trên UI.
  - *Bằng chứng:* Hoàn thiện `ChannelRepository` (`postgres`), `ChannelService`, `ChannelController` (`GET/POST /guilds/:guildId/channels`, `PATCH/DELETE /channels/:channelId`, kiểm tra `owner`/`admin` `403` và trùng tên `409`), unit test + `router_test` + integration test `TC-CHN-01..02` trên PostgreSQL/Redis thật đều pass với `go test -race -count=1 ./...` & `go vet ./...`; dựng 3-column Dark UI (`ServerBar`, `ChannelSidebar`, `ChatArea`, Member List + toàn bộ modal tạo/sửa/rời/xóa server, tạo mã mời, tham gia bằng mã mời, tạo/sửa/xóa kênh); `npm run lint` (`0 warnings, 0 errors`), `npm run build` (`tsc -b && vite build`), `cargo check` xanh; Web E2E (`http://localhost:5173`) pass đủ 10 bước tạo server → tạo/chuyển/trùng tên/sửa/xóa kênh → mời & tham gia/rời server bằng 2 tài khoản (0 lỗi console/CORS); `npx tauri dev` biên dịch thành công (`20.32s`) và khởi chạy `app.exe` + WebView2.
- [x] **Ngày 10 (25/11/2026) — Xây dựng Message Backend**
  - *Mục tiêu:* API gửi tin (`POST /channels/:id/messages` kèm `nonce`), tải lịch sử cursor pagination (`?before=<id>&limit=50`), API sửa (`PATCH`) và xóa mềm (`DELETE`) tin nhắn.
  - *Bằng chứng:* Hoàn thiện `MessageRepository`, `MessageService`, `MessageController`, `cmd/seed` và route `/api/v1/channels/:channelId/messages/**` (trim & validate `1–2.000` ký tự, `nonce <= 64`, rate limit `rl:{userID}:msg` `30/10s` trả `429` kèm `Retry-After`, hook `MessageEventPublisher`); `go vet ./...`, unit test, `router_test.go` và integration test thật (`RUN_INTEGRATION_TESTS=1`: `TC-MSG-01..04`, `TC-GLD-06`, gồm seed 1.000 tin nhắn phân trang 20 trang `limit=50` không trùng/sót) pass với `go test -race -count=1 ./...`.
- [x] **Ngày 11 (26/11/2026) — Xây dựng Giao diện Chat (REST)**
  - *Mục tiêu:* `MessageList`, `MessageComposer`, cuộn lên tải thêm lịch sử, optimistic update theo `nonce`, xử lý trạng thái lỗi và nhập tiếng Việt.
  - *Bằng chứng:* Hoàn thiện `messageApi`, `useChannelMessages` (TanStack Query `useInfiniteQuery`), `MessageList` (giữ vị trí cuộn khi tải thêm trang cũ, sửa/xóa tin nhắn, nhãn `(đã chỉnh sửa)`, `overflow-wrap: anywhere`), `MessageComposer` (IME guard `isComposing`/`keyCode === 229`); `npm run lint` (`0 warnings, 0 errors`), `npm run build`, `cargo check`, unit test cache helper (`2/2` pass), và Web Browser E2E (`http://127.0.0.1:5173`) với 1.000 tin nhắn seed thật (`cmd/seed`): cuộn 4 trang không trùng/sót, gửi tiếng Việt có dấu + F5 giữ nguyên, chuỗi 2.000 `'A'` xuống dòng gọn không tràn layout, sửa hiện `(đã chỉnh sửa)` & xóa mềm thành công, 0 lỗi console/CORS.
- [x] **Ngày 12 (27/11/2026) — Xây dựng WebSocket Backend**
  - *Mục tiêu:* API `POST /api/v1/ws/ticket` (Redis `GETDEL`), `Hub` actor, `Client` (`readPump`/`writePump`), `READY`, `HEARTBEAT`/`HEARTBEAT_ACK`, xử lý slow consumer (`4002`).
  - *Bằng chứng:* Hoàn thiện `TicketStore` (Redis `SET` 30s + `GETDEL`), `TicketService`, `WSTicketController`, và `internal/gateway` (`Hub` actor đơn goroutine hỗ trợ `Subscribe`/`Unsubscribe` ADR-007, `Client` `readPump`/`writePump` buffer `256` ngắt slow consumer `4002`, `READY`, `HEARTBEAT`/`HEARTBEAT_ACK`, graceful shutdown `1001`, `Handler` kiểm tra `Origin` Web + Tauri); `go vet ./...`, `go test -v -race -count=1 ./internal/gateway/...` (`TC-HUB-01..05` + `goleak.VerifyNone` + `goleak.VerifyTestMain`), `BenchmarkFanout` (`1.000`: `124688 ns/op`, `6 B/op`; `10.000`: `1867519 ns/op`, `2 B/op`), toàn bộ `go test -race -count=1 ./...` và integration test thật `TC-WS-01` (`18/18` pass) đều xanh.
- [x] **Ngày 13 (28/11/2026) — Kết nối Redis Pub/Sub & Frontend Realtime**
  - *Mục tiêu:* Redis Publisher/Subscriber nối với `Hub`; `GatewayClient.ts` (reconnect + backoff/jitter + `RESYNC`), đồng bộ `MESSAGE_CREATE`, `MESSAGE_UPDATE`, `MESSAGE_DELETE` vào TanStack Query cache; đo độ trễ lần 1 (đường cơ sở, 2 client) và kiểm thử `TC-WS-06` (2 instance backend).
  - *Bằng chứng:* Hoàn thiện `redisstore.EventPublisher`, `gateway.Subscriber` (FIFO ordering với `Hub` cho `GUILD_DELETE`, `GUILD_MEMBER_ADD`/`REMOVE`, `CHANNEL_*`, `MESSAGE_*`), sửa `GetMember` join user, nối tất cả service; hoàn thiện FE `wsApi`, `events.ts` (Zod), `GatewayClient.ts` (timeout ACK `4003`, backoff+jitter, `RESYNC` từ lần `READY` thứ 2) và `realtimeHandlers`/`useGatewayConnection`. Xác minh `go vet ./...`, `go test -race -count=1 ./internal/gateway/...` (`8/8` + `goleak`), integration test (`22/22` pass: `TC-WS-01`, `TC-WS-02`, `TC-WS-06`, `TC-GLD-03`, `TC-GLD-05`, `TC-GLD-07`, `TC-MSG-03`, `CHANNEL_*`), FE `oxlint`/`tsc`/`vite build`/`cargo check`, unit test `gateway_client_test.mjs` (`4/4` pass), kiểm thử live 2 tiến trình `:8080` + `:8081` (`TC-WS-06` 20/20 tin đúng thứ tự, `TC-WS-03` reconnect `RESYNC` 5/5 tin, Echo RTT `p50=2.83ms`, `p95=4.39ms`, `p99=5.06ms`), và Web Chrome E2E 2 tab (`http://127.0.0.1:5173`: `/ws` kết nối sạch, gửi tin dedup theo `nonce` không lặp, sửa `(đã chỉnh sửa)`, xóa mềm, tạo kênh `#pubsub-live` và `GUILD_DELETE` đồng bộ tức thì không cần F5, 0 lỗi console/CORS).
- [ ] **Ngày 14 (29/11/2026) — Hoàn thiện Tính năng Should, Kiểm thử & Đóng gói**
  - *Mục tiêu:* Hoàn thiện `Typing indicator`, `Presence online/offline`, `Danh sách thành viên` realtime; chạy `go test -race ./...`, `npm run build`, `npx tauri build`; đo độ trễ lần 2 (số liệu cuối, 10/100/500 client); cập nhật đầy đủ `README.md` và báo cáo.
  - *Bằng chứng:* Đóng gói thành công bản Web và bản Desktop; hoàn tất toàn bộ checklist QA.
- [ ] **30/11/2026 — Đệm: hoàn tất báo cáo & slide**
  - *Mục tiêu:* Chương 5–6, tóm tắt, tài liệu tham khảo, slide tiếng Anh, video demo dự phòng.
- [ ] **01/12/2026 — Nộp**
  - *Mục tiêu:* Tập thuyết trình hai lượt; mở thử file nộp trên một máy khác.

---

## Lộ trình Viết Báo cáo Song song

| Ngày | Chương báo cáo |
| :--- | :--- |
| 1–4 | Chương 1–2: Giới thiệu, cơ sở lý thuyết |
| 5–9 | Chương 3: ERD, API, sơ đồ tuần tự |
| 10–13 | Chương 4: Triển khai hệ thống |
| 13–14 | Chương 5: Kiểm thử, số đo thực tế |
| 30/11 | Chương 6, tóm tắt, tài liệu tham khảo, slide |