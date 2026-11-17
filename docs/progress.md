# Báo cáo Tiến độ Dự án — Discord Clone (Đồ án cơ sở 4)

- **Khung thời gian triển khai:** 16/11/2026 – 29/11/2026 (14 ngày)
- **Cập nhật lần cuối:** 17/11/2026
- **Trạng thái tổng thể:** Vượt tiến độ 2 ngày (đã hoàn thành Ngày 1, Ngày 2 và Ngày 4; đang chờ xác nhận chạy thử shell Ngày 3 để mở khóa Ngày 5).

---

## 1. Bảng theo dõi tiến độ theo ngày

| Mốc | Nội dung chính | Trạng thái | Bằng chứng nghiệm thu |
| :--- | :--- | :--- | :--- |
| **Ngày 1** | Phân tích yêu cầu, chốt quy tắc nghiệp vụ & tạo bộ tài liệu quản trị | **Hoàn thành** | Khóa toàn bộ phạm vi `Must` + 4 tính năng `Should` (Typing indicator, Sửa/xóa tin nhắn, Presence, Danh sách thành viên)[cite: 1]; hoàn thiện 6 file trong `docs/` và `api/openapi.yaml`. |
| **Ngày 2** | Cài đặt công cụ phát triển trên CachyOS | **Hoàn thành** | Đã xác minh hoạt động của `git`, `go`, `nodejs`/`npm`, `rustup`/`rustc`/`cargo`, `docker` và `docker-compose`. |
| **Ngày 3** | Khởi tạo khung Frontend (Vite + React + Tauri 2) và Go Backend | **Chờ nghiệm thu** | Cần xác nhận mở cửa sổ Desktop bằng `npx tauri dev` và phản hồi `200 OK` từ `GET /healthz`[cite: 1]. |
| **Ngày 4** | Thiết kế & triển khai Database (PostgreSQL, Redis, Migration) | **Hoàn thành** | Container `discord-postgres` và `discord-redis` chạy ổn định; `golang-migrate` thực thi thành công `0001_init.up.sql`[cite: 1]. |
| **Ngày 5** | Nền tảng Backend (`config`, `slog`, `pgx`, `redis`, `/readyz`, Clean Architecture) | **Tiếp theo** | Chưa bắt đầu. |
| **Ngày 6** | Authentication Backend (`register`, `login`, `refresh` xoay vòng, `logout`) | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 7** | Authentication Frontend & Route Guard (Web + Tauri) | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 8** | Guild, Membership & Invite Backend | **Kế hoạch** | Chưa bắt đầu. |
| **Ngày 9** | Channel Backend & Giao diện Main Layout (Server/Channel) | **Kế hoạch** | Chưa bắt đầu. |
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

---

## 3. Việc cần làm tiếp theo
Bắt đầu **Ngày 5**: Dựng cấu trúc Clean Architecture hoàn chỉnh cho `discord-clone-api` (`internal/config`, `pkg/logger` dùng `log/slog`, kết nối `pgxpool` + `go-redis`, middleware `RequestID`/`CORS`/`Recover`, và hai endpoint `/healthz`, `/readyz`)[cite: 1].