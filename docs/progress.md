# Báo cáo Tiến độ Dự án — Discord Clone (Đồ án cơ sở 4)

- **Khung thời gian triển khai:** 16/11/2026 – 29/11/2026 (14 ngày)
- **Cập nhật lần cuối:** 25/11/2026
- **Trạng thái tổng thể:** Đã hoàn thành Ngày 1, Ngày 2, Ngày 4, Ngày 5, Ngày 6, Ngày 7 (Authentication Frontend + Theme `discord-night`) và Ngày 8 (Guild & Invite Backend); còn xác nhận mở cửa sổ Desktop GUI Ngày 3 (`npx tauri dev`) và chuyển sang Ngày 9 (Channel Backend & Main Layout).

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
| **Ngày 9** | Channel Backend & Giao diện Main Layout (Server/Channel) | **Tiếp theo** | Chưa bắt đầu. |
| **Ngày 10** | Message Backend & Cursor Pagination theo UUIDv7 | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 11** | Chat UI & Optimistic Update (`nonce`) | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 12** | WebSocket Gateway Backend (`Hub`, `Client`, `Ticket` dùng 1 lần) | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 13** | Kết nối Redis Pub/Sub & Frontend Realtime (`GatewayClient`, `RESYNC`) | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 14** | Hoàn thiện nhóm `Should`, kiểm thử toàn luồng, đóng gói & tài liệu hóa | **Kế hoạch** | Chưa bắt đầu. |

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

---

## 3. Việc cần làm tiếp theo
Xác nhận mở cửa sổ Desktop GUI Ngày 3 (`npx tauri dev` tại máy desktop) và bắt đầu **Ngày 9**: Xây dựng **Channel Backend & Giao diện Main Layout 3 cột** (`ServerBar`, `ChannelSidebar`, `ChatArea`).