# Báo cáo Tiến độ Dự án — Discord Clone (Đồ án cơ sở 4)

- **Khung thời gian triển khai:** 16/11/2026 – 29/11/2026 (14 ngày)
- **Cập nhật lần cuối:** 28/11/2026
- **Trạng thái tổng thể:** Đã hoàn thành toàn bộ Sprint 14 ngày (**Ngày 1–2, 4–14**, gồm cả 3 tính năng `Should`, `cmd/loadtest`, CI, `Makefile`, `README.md` và đóng gói Desktop Tauri 2); còn bước xác nhận tương tác thủ công trên cửa sổ Desktop GUI Ngày 3 nếu cần bổ sung kiểm tra trực tiếp.

---

## 1. Bảng theo dõi tiến độ theo ngày

| Mốc | Nội dung chính | Trạng thái | Bằng chứng nghiệm thu |
| :--- | :--- | :--- | :--- |
| **Ngày 1** | Phân tích yêu cầu, chốt quy tắc nghiệp vụ & tạo bộ tài liệu quản trị | **Hoàn thành** | Khóa toàn bộ phạm vi `Must` + 4 tính năng `Should` (Typing indicator, Sửa/xóa tin nhắn, Presence, Danh sách thành viên)[cite: 1]; hoàn thiện 6 file trong `docs/` và `api/openapi.yaml`. |
| **Ngày 2** | Cài đặt công cụ phát triển trên CachyOS | **Hoàn thành** | Đã xác minh hoạt động của `git`, `go`, `nodejs`/`npm`, `rustup`/`rustc`/`cargo`, `docker` và `docker-compose`. |
| **Ngày 3** | Khởi tạo khung Frontend (Vite + React + Tauri 2) và Go Backend | **Chờ nghiệm thu** | Cần xác nhận mở cửa sổ Desktop bằng `npx tauri dev` và phản hồi `200 OK` từ `GET /healthz`[cite: 1]. |
| **Ngày 4** | Thiết kế & triển khai Database (PostgreSQL, Redis, Migration) | **Hoàn thành** | Container `discord-postgres` và `discord-redis` chạy ổn định; `golang-migrate` thực thi thành công `0001_init.up.sql`[cite: 1]. |
| **Ngày 5** | Nền tảng Backend (`config`, `slog`, `pgx`, `redis`, `/readyz`, Clean Architecture) | **Hoàn thành** | Dựng đầy đủ `internal/config`, `pkg/*`, `internal/domain`, `postgres`/`redisstore`, middleware `RequestID`/`CORS`/`Recover`/`RateLimit`/`Auth`; `GET /readyz` trả `200` (`postgres: ok`, `redis: ok`). |
| **Ngày 6** | Authentication Backend (`register`, `login`, `refresh` xoay vòng, `logout`) | **Hoàn thành** | Hoàn thiện `register`, `login` (`429 RATE_LIMITED`), `refresh` (`GETDEL` xoay vòng `refresh:{hash}`), `logout`, `GET/PATCH /api/v1/users/@me`; unit test + integration test `TC-AUTH-01..06` (backend) + `go vet ./...` + `go test -race ./...` xanh. |
| **Ngày 7** | Authentication Frontend & Route Guard (Web + Tauri) | **Hoàn thành** | Dựng `tokens.css` (`discord-night`), `StorageAdapter`, `httpClient` auto-refresh `401`, `useAuthStore`, `RouteGuards`, `LoginPage`/`RegisterPage`; `npm run lint` & `npm run build` xanh, E2E Web (`TC-AUTH-05` + F5 giữ phiên) pass, `cargo check` Tauri shell xanh. |
| **Ngày 8** | Guild, Membership & Invite Backend | **Hoàn thành** | Hoàn thiện `GuildService`, `InviteService`, `GuildRepository`/`InviteRepository` (transaction + `FOR UPDATE`), `PresenceStore`, hook `Unsubscribe` (ADR-007); `TC-GLD-01..04` + `go vet ./...` + `go test -race ./...` xanh. |
| **Ngày 9** | Channel Backend & Giao diện Main Layout (Server/Channel) | **Hoàn thành** | Hoàn thiện `ChannelService`, `ChannelController`, `ChannelRepository` (`403` member, `409` trùng tên); dựng layout 3 cột Dark Mode + đầy đủ modal Server/Invite/Channel; `TC-CHN-01..02` + `go test -race ./...` + `npm run lint` (`0 warnings`) + `npm run build` + Web E2E 2 tài khoản + `npx tauri dev` launch pass. |
| **Ngày 10** | Message Backend & Cursor Pagination theo UUIDv7 | **Hoàn thành** | Hoàn thiện `MessageRepository`, `MessageService` (rate limit `rl:{userID}:msg` `30/10s`, `nonce`, cursor pagination, RBAC sửa/xóa mềm), `MessageController`, `cmd/seed`; `TC-MSG-01..04`, `TC-GLD-06` + `go test -race ./...` xanh. |
| **Ngày 11** | Chat UI & Optimistic Update (`nonce`) | **Hoàn thành** | Hoàn thiện `messageApi`, `useChannelMessages`, `MessageList`, `MessageComposer`; `npm run lint` (`0 warnings`), `npm run build`, `cargo check` xanh; Web E2E trên 1.000 tin nhắn seed pass (`TC-MSG-02`, `TC-UI-01..02`, sửa/xóa & F5). |
| **Ngày 12** | WebSocket Gateway Backend (`Hub`, `Client`, `Ticket` dùng 1 lần) | **Hoàn thành** | Hoàn thiện `POST /api/v1/ws/ticket` (`GETDEL` TTL `30s`), `GET /ws?ticket=...`, `Hub` actor (`Subscribe`/`Unsubscribe`, shutdown `1001`), `Client` (`readPump`/`writePump`, `4002`); `TC-HUB-01..05`, `goleak`, `TC-WS-01`, `BenchmarkFanout` & `go test -race ./...` xanh. |
| **Ngày 13** | Kết nối Redis Pub/Sub & Frontend Realtime (`GatewayClient`, `RESYNC`) | **Hoàn thành** | Nối `redisstore.EventPublisher` + `gateway.Subscriber` (`MESSAGE_*`, `CHANNEL_*`, `GUILD_MEMBER_*`, `GUILD_DELETE`); dựng `events.ts` (Zod), `GatewayClient.ts` (`4003`, backoff+jitter, `RESYNC`), TanStack Query sync; `22/22` integration test (`TC-WS-01..02`, `TC-WS-06`, `TC-GLD-03/05/07`, `TC-MSG-03`), `gateway_client_test` `4/4`, và live 2 instance `:8080`/`:8081` (`p50=2.83ms`, `p95=4.39ms`) xanh. |
| **Ngày 14** | Hoàn thiện nhóm `Should`, kiểm thử toàn luồng, đóng gói & tài liệu hóa | **Hoàn thành** | Hoàn thiện Typing (`SET NX EX 3`), Presence multi-tab (`TTL 60s`), Member List realtime; `cmd/loadtest` (`2`–`500` client 100% delivery, `500` client `p99=30.69ms`), `.github/workflows/ci.yml`, `Makefile`, `README.md`, WCAG AA `>= 4.57:1`, `25/25` integration test, Chrome E2E và `npx tauri build` (`.msi` + `-setup.exe`) xanh. |

---

## 2. Nhật ký bằng chứng kiểm thử đã xác nhận

### Mốc Ngày 4 — Cơ sở dữ liệu & Ràng buộc toàn vẹn (17/11/2026)
- **Migration:** Thực thi `migrate -path migrations -database "$DB_URL" up` / `down 1` / `up` thành công.
- **Danh sách bảng (`\dt`):** Xác nhận đủ 7 relation trong schema `public`: `users`, `guilds`, `guild_members`, `channels`, `messages`, `invites`, và `schema_migrations`[cite: 1].
- **Kiểm thử `CITEXT` & Unique Constraint:**
  - Chèn bản ghi `'Truong'` thành công (`INSERT 0 1`).
  - Chèn bản ghi `'truong'` bị chặn đúng thiết kế: `ERROR: duplicate key value violates unique constraint "users_username_key"`.
  - Dọn dẹp dữ liệu kiểm thử thành công (`DELETE 1`).

### Mốc Ngày 5 & Ngày 6 — Nền tảng Backend & Authentication (25/11/2026)
- **Kiểm tra tĩnh & race detector:** `go vet ./...` thoát mã `0`; toàn bộ unit test (`internal/config`, `pkg/logger`, `pkg/idgen`, `pkg/hasher`, `pkg/jwtutil`, `internal/service`, `internal/router`) pass với `go test -race ./...`.
- **Kiểm thử tích hợp trên PostgreSQL & Redis thật (`RUN_INTEGRATION_TESTS=1`):**
  - `GET /healthz` → `200 OK`, `GET /readyz` → `200 OK` (`postgres: ok`, `redis: ok`).
  - `TC-AUTH-01`: Đăng ký tài khoản mới trả `201 Created`, `id` là `UUIDv7`, `password_hash` trong DB được băm `bcrypt`.
  - `TC-AUTH-02`: Trùng `username` hoặc `email` khác hoa/thường bị chặn `409 Conflict` qua `CITEXT`.
  - `TC-AUTH-03`: Đăng nhập trả `accessToken` (15 phút) và `refreshToken`, lưu `refresh:{hash}` trong Redis (TTL 7 ngày).
  - `TC-AUTH-04`: `POST /api/v1/auth/refresh` xoay vòng token nguyên tử (`GETDEL`), dùng lại token cũ trả `401`; `POST /api/v1/auth/logout` xóa key trên Redis; `GET/PATCH /api/v1/users/@me` cập nhật `displayName` & `avatarUrl`.
  - `TC-AUTH-05` *(phần backend)*: Access token hết hạn trả `401`, gọi `/auth/refresh` cấp token mới và gọi lại `/users/@me` trả `200` (phần auto-retry `httpClient` sẽ nghiệm thu ở Ngày 7).
  - `TC-AUTH-06`: Đăng nhập sai 10 lần trả `401`, lần thứ 11 trả `429 RATE_LIMITED` kèm `Retry-After` và key `rl:login:{ip}` TTL ≤ 60 giây.

### Mốc Ngày 7 & Ngày 8 — Authentication Frontend & Guild/Invite Backend (25/11/2026)
- **Frontend (`discord-clone-app/`):**
  - `npm run lint` (`oxlint`): `0 warnings, 0 errors` (17 file).
  - `npm run build` (`tsc -b && vite build`): build thành công (`dist/index.html`, JS + CSS bundle).
  - `cargo check` trong `discord-clone-app/src-tauri`: biên dịch kiểm tra Tauri 2 Rust shell thành công (`Finished dev profile`).
  - Unit test `StorageAdapter` & `httpClient` (`http_client_test.ts`): 3/3 pass — xác minh 3 request đồng thời nhận `401` chỉ kích hoạt duy nhất 1 request `/api/v1/auth/refresh` (single-flight) và gọi lại toàn bộ request gốc; kiểm thử live với API `:8080` (`LIVE_TC_AUTH_05_PASS`) xoay vòng token thật thành công.
  - Web Browser E2E (`http://localhost:5173`): route guard chuyển `/ -> /login`, đăng ký mới -> `/app`, F5 giữ nguyên phiên qua `StorageAdapter`, nút đồng bộ `@me`, đăng xuất xóa phiên và đăng nhập lại thành công, không lỗi console/CORS.
- **Backend (`discord-clone-api/`):**
  - Kiểm tra phụ thuộc Clean Architecture (`go list` trên `internal/domain` và `internal/service`): không có import `gin`, `pgx`, hay `redis`.
  - `go vet ./...` và `go test -race -count=1 ./...` pass toàn bộ package (`config`, `logger`, `idgen`, `hasher`, `jwtutil`, `service`, `router`, `integration`).
  - Integration test trên PostgreSQL + Redis thật (`RUN_INTEGRATION_TESTS=1`): `TC-GLD-01` (tạo guild + `#general` + `owner` trong 1 transaction, CRUD + phân quyền `owner`), `TC-GLD-02` (chặn `owner` rời server `403 FORBIDDEN`), `TC-GLD-03` (tạo/nhận invite, liệt kê members + presence online, rời server gọi hook `Unsubscribe` ADR-007), `TC-GLD-04` (`400 INVITE_EXPIRED`, `400 INVITE_EXHAUSTED`, `404 INVITE_NOT_FOUND`) đều pass.

### Mốc Ngày 9 — Channel Backend & 3-Column Dark Mode UI (25/11/2026)
- **Backend (`discord-clone-api/`):**
  - Bổ sung `Role.CanManageChannels()`, `UpdateChannelParams`, `domain.ChannelRepository`, `postgres.ChannelRepository`, `service.ChannelService`, `controller.ChannelController`, DTO và route `GET/POST /api/v1/guilds/:guildId/channels`, `PATCH/DELETE /api/v1/channels/:channelId`.
  - `go list` trên `internal/domain` và `internal/service` giữ nguyên không có dependency framework/driver.
  - `go vet ./...` và `go test -race -count=1 ./...` pass toàn bộ package (bao gồm `channel_service_test.go` và `TestRouter_ChannelRoutes`).
  - Integration test thật (`RUN_INTEGRATION_TESTS=1 go test -v -race -count=1 ./internal/integration/...`): **13/13 pass** (`Readyz`, `TC-AUTH-01..06`, `TC-GLD-01..04`, `TC-CHN-01` member 403 / admin SQL + owner CRUD, `TC-CHN-02` trùng tên kênh `409 CHANNEL_NAME_TAKEN`).
- **Frontend (`discord-clone-app/`):**
  - Bổ sung `guildApi`, `channelApi`, `useWorkspaceStore`, `ServerBar`, `ChannelSidebar`, `ChatArea` (kèm Member List thu gọn), và các modal (`CreateServerModal`, `EditServerModal`, `JoinServerModal`, `CreateInviteModal`, `ChannelFormModal`, `ConfirmActionModal`), đồng bộ URL `/app/guilds/:guildId/channels/:channelId`.
  - `npm run lint` (`oxlint`): `0 warnings, 0 errors` (27 file).
  - `npm run build` (`tsc -b && vite build`): build thành công (`dist/`).
  - Web Browser E2E (`http://localhost:5173`): pass 10/10 bước với 2 tài khoản (`Owner` tạo server, tạo/chuyển kênh, F5 giữ phiên & kênh, kiểm tra `409` trùng tên, sửa kênh, tạo mã mời; `Member` tham gia bằng mã mời, xác minh ẩn nút quản trị kênh & chỉ có "Rời server", rời server; `Owner` xóa kênh & xóa server), 0 lỗi console/CORS.
  - Desktop (`discord-clone-app/src-tauri/`): `cargo check` pass; `npx tauri dev` biên dịch thành công (`20.32s`) và khởi chạy `target\debug\app.exe` cùng tiến trình WebView2.

### Mốc Ngày 10 & Ngày 11 — Message Backend, Cursor Pagination & Chat UI (25/11/2026)
- **Backend (`discord-clone-api/`):**
  - Bổ sung `domain.Message`, `domain.MessageEventPublisher`, `domain.NewRateLimitError`, `postgres.MessageRepository`, `service.MessageService`, `controller.MessageController`, DTO, route `/api/v1/channels/:channelId/messages/**`, `MESSAGE_RATE_LIMIT`/`MESSAGE_RATE_WINDOW` trong `internal/config`, và CLI `cmd/seed`.
  - `go list` trên `internal/domain` và `internal/service` giữ nguyên không có phụ thuộc framework/driver.
  - `go vet ./...` và `go test -race -count=1 ./...` pass toàn bộ package (`config`, `logger`, `idgen`, `hasher`, `jwtutil`, `service`, `router`, `integration`).
  - Integration test thật (`RUN_INTEGRATION_TESTS=1 go test -v -race -count=1 ./internal/integration/...`): **17/17 pass** (`Readyz`, `TC-AUTH-01..06`, `TC-CHN-01..02`, `TC-GLD-01..04`, `TC-GLD-06`, `TC-MSG-01..04`, gồm seed 1.000 tin nhắn và đọc hết 20 trang `limit=50` không trùng/sót).
- **Frontend (`discord-clone-app/`):**
  - Bổ sung `messageApi`, `useChannelMessages` (`useInfiniteQuery` + optimistic `nonce` upsert/resolve/fail/retry), `MessageList` (auto scroll-up pagination giữ nguyên vị trí cuộn, sửa inline & `(đã chỉnh sửa)`, xóa mềm kèm modal xác nhận, `overflow-wrap: anywhere`), `MessageComposer` (guard `isComposing`/`keyCode === 229` cho bộ gõ tiếng Việt).
  - `npm run lint` (`oxlint`): `0 warnings, 0 errors` (33 file); `npm run build`: build thành công (`dist/`); `cargo check`: thành công.
  - Unit test helper (`message_ui_test.ts`): `2/2` pass (`flattenPagesChronologically` và optimistic cache transforms).
  - Web Browser E2E (`http://127.0.0.1:5173/`): seed 1.000 tin nhắn qua `cmd/seed`, cuộn lên tải 4 trang cũ (`#1000` xuống `#0751`) không trùng/sót và giữ vị trí cuộn, gửi tin tiếng Việt có dấu + F5 giữ nguyên, gửi chuỗi 2.000 ký tự `'A'` không tràn layout (`fitsInsideContainerWidth: true`), sửa hiện `(đã chỉnh sửa)` & xóa mềm biến mất cả sau F5, 0 lỗi console/CORS.

### Mốc Ngày 12 — WebSocket Ticket Auth & Actor Hub Gateway (25/11/2026)
- **Backend (`discord-clone-api/`):**
  - Bổ sung `github.com/gorilla/websocket v1.5.3` và `go.uber.org/goleak v1.3.0`.
  - Bổ sung `domain.TicketStore`, `redisstore.TicketStore` (`SET` 30s + `GETDEL`), `service.TicketService`, `controller.WSTicketController` (`POST /api/v1/ws/ticket`), và package `internal/gateway` (`protocol.go`, `client.go`, `hub.go`, `handler.go`) với `Hub` actor đơn goroutine (nối `Subscribe`/`Unsubscribe` vào `GuildService` và `InviteService` tại `cmd/api/main.go`).
  - `go vet ./...` và `go test -race -count=1 ./...` pass toàn bộ package (`config`, `gateway`, `integration`, `router`, `service`, `pkg/*`).
  - `go test -v -race -count=1 ./internal/gateway/...`: **7/7 test pass** với `goleak.VerifyTestMain` & `goleak.VerifyNone` sạch (`TestHandler_TicketAuthAndOriginCheck`, `TestHub_ReadyHeartbeatAndDynamicSubscriptions`, `TC-HUB-01` `4002`, `TC-HUB-02`, `TC-HUB-03`, `TC-HUB-04` `1001`, `TC-HUB-05` `pprof` goroutine baseline).
  - `BenchmarkFanout`: `clients_1000` đạt `124688 ns/op`, `6 B/op`, `0 allocs/op`; `clients_10000` đạt `1867519 ns/op`, `2 B/op`, `0 allocs/op`.
  - Integration test thật (`RUN_INTEGRATION_TESTS=1 go test -v -race -count=1 ./internal/integration/...`): **18/18 pass**, bao gồm `TC-WS-01` (ticket TTL ≤ 30s trên Redis, upgrade `/ws` nhận `READY` + `HEARTBEAT_ACK`, dùng lại vé bị từ chối `401`).

### Mốc Ngày 13 — Redis Pub/Sub & Frontend Realtime (`GatewayClient`, TanStack Query Sync) (28/11/2026)
- **Backend (`discord-clone-api/`):**
  - Bổ sung `internal/domain/events.go` (`ChannelEventPublisher`, `GuildEventPublisher`), sửa `GuildRepository.GetMember` join `users` để có đủ thông tin `User` khi phát `GUILD_MEMBER_REMOVE`, tạo `redisstore.EventPublisher` (`PUBLISH guild:{guildID}`), và nối `ChannelService`, `GuildService`, `InviteService`, `cmd/api/main.go`.
  - Cập nhật `gateway.Hub` xử lý tuần tự các hành động FIFO qua `broadcastCh` (`BroadcastAndRemoveGuild`, `UnsubscribeAndBroadcast`, `SubscribeAndBroadcast`) và tạo `gateway.Subscriber` (`PSUBSCRIBE guild:*` với `context.AfterFunc` đóng sạch `pubsub`).
  - `go vet ./...` và `go test -race -count=1` pass toàn bộ unit test; `internal/gateway/...` **8/8 pass** với `goleak.VerifyTestMain`; `internal/integration/...` (`RUN_INTEGRATION_TESTS=1`) **22/22 pass** (`TC-WS-01`, `TC-WS-02`, `TC-WS-06` 2 instance độc lập, `TC-GLD-03`, `TC-GLD-05`, `TC-GLD-07`, `TC-MSG-03`, `CHANNEL_CREATE/UPDATE/DELETE`).
  - Kiểm thử live 2 tiến trình backend `:8080` và `:8081` trên PostgreSQL & Redis thật: `TC-WS-06` (20/20 tin nhắn chuyển tiếp qua Redis Pub/Sub đúng thứ tự, Echo RTT `p50=2.83ms`, `p95=4.39ms`, `p99=5.06ms`) và `TC-WS-03` (ngắt kết nối, gửi 5 tin, kết nối lại & `RESYNC` nhận đủ 5/5 tin) pass.
- **Frontend (`discord-clone-app/`):**
  - Bổ sung `wsApi` (`POST /api/v1/ws/ticket`), `src/core/gateway/events.ts` (Zod v4 discriminated union cho mọi frame WS), `src/core/gateway/GatewayClient.ts` (heartbeat `25s`, ACK timeout `10s` đóng mã `4003`, exponential backoff + jitter, `RESYNC` từ lần `READY` thứ 2), `src/features/realtime/` (`applyGatewayEventToCache`, `handleGatewayResync`, `useGatewayConnection`), banner tự kết nối lại trên `WorkspacePage`, và dedup `nonce` trong `messageHelpers.ts`.
  - `npm run lint` (`oxlint`: `0 warnings, 0 errors` trên 39 file), `npm run build` (`tsc -b && vite build`), `cargo check`, và unit test `gateway_client_test.ts` (**4/4 pass**) xanh.
  - Web Chrome E2E 2 tab (`http://127.0.0.1:5173/`): đăng ký `rt_e2e_chrome`, tạo server `Realtime E2E Guild`, mở tab 2 tự hydrate và nối `/ws`; Tab 1 gửi `Cross-tab realtime check` (dedup `nonce` hiện đúng 1 tin, tab 2 nhận ngay), sửa `Edited across tabs` (`(đã chỉnh sửa)` cập nhật live), xóa mềm, tạo kênh `#pubsub-live` và xóa server `Realtime E2E Guild` (`GUILD_DELETE` đưa cả 2 tab về màn hình chào mừng) đều cập nhật tức thì không cần F5, **0 lỗi console/CORS**.

### Mốc Ngày 14 — Tính năng Should, Loadtest, Đóng gói Tauri 2 & QA Cuối (28/11/2026)
- **Backend (`discord-clone-api/`):**
  - Bổ sung `domain.PresenceStore`, `domain.TypingLimiter`, `PresenceEventPublisher`, `TypingEventPublisher`, `redisstore.PresenceStore` (`SetOnline` TTL `60s` / `SetOffline`), `redisstore.TypingStore` (`SET NX EX 3`), `redisstore.EventPublisher` (`PRESENCE_UPDATE`, `USER_TYPING`), `service.RealtimeService` (`realtime_service_test.go`), và `gateway.BindRealtimeHooks` + worker `hookCh` không block `Hub.Run`.
  - `go vet ./...`, `go list` (`domain`/`service` sạch dependency), `internal/gateway/...` (**9/9 pass** + `goleak`, gồm `TestHub_MultiTabPresenceHooks`), và `internal/integration/...` (`RUN_INTEGRATION_TESTS=1`: **25/25 pass**, gồm `TC-WS-04` Typing `SET NX EX 3`, `TC-WS-05`/`07` Presence multi-tab + tab cuối offline, `TC-UI-03` CORS & WS `CheckOrigin` đủ Web + Tauri origin).
  - Thêm `cmd/loadtest/main.go` và đo thực tế với `127.0.0.1:8080` (`20` tin/lượt): `2` client (`40/40`, `p50=4.36ms`, `p95=5.02ms`, `p99=6.60ms`), `10` client (`200/200`, `p50=4.69ms`, `p95=5.82ms`, `p99=9.75ms`), `100` client (`2.000/2.000`, `p50=5.07ms`, `p95=6.24ms`, `p99=10.92ms`), `500` client (`10.000/10.000`, `p50=15.66ms`, `p95=24.24ms`, `p99=30.69ms`).
- **Frontend & Desktop (`discord-clone-app/`):**
  - Bổ sung `typingStore.ts` (auto-expiry ~`3,5s`, tự xóa khi nhận `MESSAGE_CREATE`), throttle gửi `TYPING_START` (`2,5s`) trong `MessageComposer.tsx`, hiển thị thanh `... đang nhập` & Member List nhóm `Trực tuyến` / `Ngoại tuyến` trong `ChatArea.tsx` (sử dụng selector `EMPTY_TYPING_ENTRIES` ổn định), và chuẩn hóa màu WCAG AA trong `tokens.css` (`TC-UI-04`: `16/16` cặp token $\ge$ `4,5:1`, thấp nhất `4,57:1`).
  - `npm run lint` (`oxlint`: `0 warnings, 0 errors`), `npm run build` (`tsc -b && vite build`), unit test `gateway_client_test.ts` (**5/5 pass**), Chrome E2E 2 tab (`Trực tuyến — 1`, gửi/nhận realtime, đóng tab phụ vẫn giữ trạng thái online), và `npx tauri build` tạo `Lumiflux_0.1.0_x64_en-US.msi` & `Lumiflux_0.1.0_x64-setup.exe`.
- **DevOps & Tài liệu:** Bổ sung `.github/workflows/ci.yml`, `Makefile`, hoàn thiện `README.md`, `docs/api.md`, `docs/testing.md`.

---

## 3. Việc cần làm tiếp theo
Hoàn tất triển khai toàn bộ tính năng và kiểm thử Sprint 14 ngày; sẵn sàng tổng hợp báo cáo đồ án và thuyết trình.