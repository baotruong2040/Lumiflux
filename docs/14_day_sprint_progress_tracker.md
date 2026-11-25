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
- [ ] **Ngày 7 (22/11/2026) — Xây dựng Authentication Frontend**
  - *Mục tiêu:* Màn hình Login/Register, `httpClient` tự động refresh token khi gặp `401`, Auth store, Route Guard và `StorageAdapter` dùng chung Web/Tauri.
  - *Bằng chứng:* Đăng ký, đăng nhập, F5 giữ nguyên phiên trên cả trình duyệt và cửa sổ Tauri.

---

## Tuần 2: Chat REST, WebSocket Realtime, Tính năng Should & Đóng gói

- [ ] **Ngày 8 (23/11/2026) — Xây dựng Guild & Invite Backend**
  - *Mục tiêu:* CRUD Guild (transaction tạo guild + `#general` + `owner`), danh sách thành viên, rời server (chặn `owner`), tạo và chấp nhận mã mời (`POST /invites/:code/accept`, mã mời hết hạn/hết lượt trả `400`); gán `admin` bằng SQL khi test.
  - *Bằng chứng:* Kiểm thử phân quyền `owner` / `admin` / `member` và mã mời hết hạn/hết lượt.
- [ ] **Ngày 9 (24/11/2026) — Xây dựng Channel & Giao diện Server**
  - *Mục tiêu:* API CRUD Channel (`owner`/`admin`), dựng layout 3 cột Dark Mode (`ServerBar`, `ChannelSidebar`, `ChatArea`), luồng tạo/tham gia server và chọn kênh trên UI.
  - *Bằng chứng:* Đăng nhập, tạo server, tạo kênh và chuyển qua lại giữa các kênh trên giao diện.
- [ ] **Ngày 10 (25/11/2026) — Xây dựng Message Backend**
  - *Mục tiêu:* API gửi tin (`POST /channels/:id/messages` kèm `nonce`), tải lịch sử cursor pagination (`?before=<id>&limit=50`), API sửa (`PATCH`) và xóa mềm (`DELETE`) tin nhắn.
  - *Bằng chứng:* Seed 1.000 tin nhắn và kiểm tra phân trang cursor không bị trùng/sót bản ghi.
- [ ] **Ngày 11 (26/11/2026) — Xây dựng Giao diện Chat (REST)**
  - *Mục tiêu:* `MessageList`, `MessageComposer`, cuộn lên tải thêm lịch sử, optimistic update theo `nonce`, xử lý trạng thái lỗi và nhập tiếng Việt.
  - *Bằng chứng:* Gửi và đọc tin nhắn mượt mà qua REST, F5 không mất lịch sử.
- [ ] **Ngày 12 (27/11/2026) — Xây dựng WebSocket Backend**
  - *Mục tiêu:* API `POST /api/v1/ws/ticket` (Redis `GETDEL`), `Hub` actor, `Client` (`readPump`/`writePump`), `READY`, `HEARTBEAT`/`HEARTBEAT_ACK`, xử lý slow consumer (`4002`).
  - *Bằng chứng:* `go test -race ./internal/gateway/...` xanh hoàn toàn, không rò rỉ goroutine; `TC-HUB-01..04` xanh.
- [ ] **Ngày 13 (28/11/2026) — Kết nối Redis Pub/Sub & Frontend Realtime**
  - *Mục tiêu:* Redis Publisher/Subscriber nối với `Hub`; `GatewayClient.ts` (reconnect + backoff/jitter + `RESYNC`), đồng bộ `MESSAGE_CREATE`, `MESSAGE_UPDATE`, `MESSAGE_DELETE` vào TanStack Query cache; đo độ trễ lần 1 (đường cơ sở, 2 client) và kiểm thử `TC-WS-06` (2 instance backend).
  - *Bằng chứng:* Hai client (Web + Desktop) chat realtime; ngắt mạng 10s rồi bật lại tự động đồng bộ đủ tin bị lỡ.
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