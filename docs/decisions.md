# Nhật ký Quyết định Kiến trúc & Nghiệp vụ (`decisions.md`)

Tài liệu này ghi lại toàn bộ các quyết định đã được chốt cho dự án Discord Clone để đảm bảo tính nhất quán giữa Backend, Frontend và tài liệu báo cáo.

---

## ADR-001: Ngăn xếp công nghệ và chiến lược đa nền tảng
- **Trạng thái:** Đã chốt (theo tài liệu kỹ thuật gốc)
- **Quyết định:**
  - **Frontend:** React + TypeScript + Vite, đóng gói ứng dụng Desktop bằng **Tauri 2** (dùng chung 1 codebase).
  - **Backend:** Go ($\ge 1.22$), `gin-gonic/gin`, `gorilla/websocket`, tổ chức theo **Clean Architecture**.
  - **Database & Cache:** PostgreSQL (truy cập qua `pgx/v5` + SQL thuần, migration bằng `golang-migrate`) và Redis (`go-redis/v9`).
- **Lý do:** Tận dụng một ngôn ngữ TypeScript cho cả Web và Desktop, gói cài đặt Tauri nhẹ hơn Electron, đồng thời phô diễn rõ kỹ năng SQL và quản lý concurrency trong Go.

## ADR-002: Định danh UUIDv7 và phân trang Cursor
- **Trạng thái:** Đã chốt
- **Quyết định:** Sinh khóa chính `UUIDv7` ở tầng Go (`google/uuid`) cho các bảng chính; phân trang lịch sử tin nhắn dùng cursor theo `id` (`WHERE channel_id = $1 AND id < $2 AND deleted_at IS NULL ORDER BY id DESC LIMIT 50`) thay vì `OFFSET`; trang đầu (không có `before`) bỏ điều kiện `id < $2`.
- **Lý do:** `UUIDv7` chứa timestamp ở các bit đầu nên sắp xếp tự nhiên theo thời gian, giúp truy vấn trên index `(channel_id, id DESC)` đạt tốc độ $O(\log N)$ ổn định ngay cả khi kênh có hàng chục nghìn tin nhắn.

## ADR-003: Mô hình gửi tin REST kết hợp phát tán WebSocket & Redis Pub/Sub
- **Trạng thái:** Đã chốt
- **Quyết định:** Thao tác ghi dữ liệu (gửi/sửa/xóa tin nhắn) đi qua REST API để lưu xuống PostgreSQL trước; sau đó Backend publish sự kiện lên kênh Redis `guild:{guildID}` để các instance WebSocket Hub chuyển tiếp xuống client. Client dùng `nonce` để khử trùng tin nhắn lạc quan và gọi lại REST (`RESYNC`) sau mỗi lần reconnect.
- **Lý do:** Redis Pub/Sub có đặc tính giao tin tối đa một lần (*at-most-once*). Việc lấy PostgreSQL làm nguồn chân lý duy nhất (*single source of truth*) đảm bảo không bao giờ mất dữ liệu khi rớt mạng hoặc khởi động lại server.

## ADR-004: Xác thực WebSocket bằng Ticket dùng một lần
- **Trạng thái:** Đã chốt
- **Quyết định:** Client gọi `POST /api/v1/ws/ticket` (kèm header `Authorization: Bearer <access_token>`) để nhận một mã ticket ngẫu nhiên lưu trong Redis (`wsticket:{ticket}`, TTL 30 giây). Khi nâng cấp WebSocket tại `GET /ws?ticket=...`, server dùng lệnh nguyên tử `GETDEL` để xác thực và hủy vé ngay lập tức.
- **Lý do:** Trình duyệt không cho phép gắn custom header `Authorization` khi khởi tạo `new WebSocket(url)`. Dùng vé dùng một lần tránh việc làm lộ JWT dài hạn trong access log của proxy/trình duyệt.

## ADR-005: Quyền tạo mã mời (Invite)
- **Trạng thái:** Đã chốt (Ngày 1 — 16/11/2026)
- **Quyết định:** Cho phép **mọi thành viên** trong server (`owner`, `admin`, `member`) tạo mã mời thông qua `POST /api/v1/guilds/:guildId/invites`. Mặc định mã mời không giới hạn số lượt (`max_uses = NULL`) và có thời hạn 7 ngày (hoặc tùy chọn theo tham số gửi lên).
- **Lý do:** Khớp với bảng đặc tả API gốc và tăng tính tiện dụng khi kiểm thử mời người dùng tham gia server.

## ADR-006: Quy tắc rời server của Owner
- **Trạng thái:** Đã chốt (Ngày 1 — 16/11/2026)
- **Quyết định:** Chặn `owner` gọi API tự rời server (`DELETE /api/v1/guilds/:guildId/members/@me`), hệ thống trả về mã lỗi `403 FORBIDDEN`. Nếu `owner` muốn thoát khỏi server do mình tạo, bắt buộc phải thực hiện xóa server (`DELETE /api/v1/guilds/:guildId`).
- **Lý do:** Cột `guilds.owner_id` có ràng buộc `NOT NULL REFERENCES users(id)` và mỗi server chỉ có 1 owner duy nhất; việc chặn owner rời ngang giữ toàn vẹn dữ liệu mà không cần thêm luồng chuyển nhượng quyền sở hữu phức tạp trong 14 ngày.

## ADR-007: Hủy đăng ký luồng WebSocket khi rời server (`hub.Unsubscribe`)
- **Trạng thái:** Đã chốt (sau khi xác nhận)
- **Quyết định:** Bổ sung phương thức `hub.Unsubscribe(userID, guildID)` vào `gateway.Hub` (đối xứng với `hub.Subscribe(userID, guildID)`). Khi người dùng rời server (`DELETE /guilds/:guildId/members/@me`), Service gọi `hub.Unsubscribe` để gỡ toàn bộ kết nối WebSocket đang hoạt động của user đó khỏi danh sách nhận broadcast của `guildID`. Kế hoạch gốc (§3.1) chỉ có `Hub.Subscribe`; `Unsubscribe` là bổ sung của dự án. Kiểm thử: `TC-GLD-05`.
- **Lý do:** Ngăn chặn rò rỉ dữ liệu realtime (user đã rời server nhưng tab đang mở vẫn nhận được tin nhắn mới của server đó trước khi F5).

## ADR-008: Chiến lược lưu trữ Token trên Web và Tauri
- **Trạng thái:** Đã chốt (Ngày 1 — 16/11/2026)
- **Quyết định:** Tách logic lưu trữ phiên đăng nhập qua interface `StorageAdapter` trong `src/core/platform/`. Trong phạm vi 14 ngày, cả bản Web và bản Desktop (Tauri 2) đều sử dụng cài đặt `localStorage` thông qua adapter này.
- **Lý do:** WebView của Tauri chạy trên origin riêng biệt (`tauri://localhost` hoặc `http://tauri.localhost`), tách biệt hoàn toàn với trình duyệt bên ngoài, giúp dùng chung 100% logic mà vẫn giữ kiến trúc mở để nâng cấp lên `@tauri-apps/plugin-store` sau này.

## ADR-009: Khóa phạm vi tính năng `Should` cho Sprint 14 ngày
- **Trạng thái:** Đã chốt (Ngày 1 — 16/11/2026)
- **Quyết định:** Bên cạnh toàn bộ nhóm `Must`, sprint 14 ngày sẽ triển khai **4 tính năng `Should`**:
  1. Typing indicator (`TYPING_START` / `USER_TYPING`)
  2. Sửa và xóa mềm tin nhắn (`MESSAGE_UPDATE` / `MESSAGE_DELETE`)
  3. Hiển thị trạng thái online/offline (`PRESENCE_UPDATE`)
  4. Danh sách thành viên (`GET /guilds/:guildId/members` + `GUILD_MEMBER_ADD` / `GUILD_MEMBER_REMOVE`)
  Các tính năng còn lại (`Custom Theme JSON`, `Animated Pixel Cursor`, và toàn bộ nhóm `Could`) được tạm hoãn để bảo đảm chất lượng cốt lõi.
- **Thứ tự cắt khi trễ (theo kế hoạch gốc):** cắt presence và sửa/xóa tin nhắn trước; giữ typing indicator.
- **Khác kế hoạch gốc:** kế hoạch gốc khuyên tối đa 2 mục `Should` nếu làm một mình (typing + pixel cursor) và coi theme + pixel cursor là điểm nhấn. Sprint này chọn 4 mục khác và hoãn theme/cursor. Nếu còn thời gian cuối Ngày 12, làm lát nhỏ của theme: chuyển `data-theme` giữa `discord-night` và `steam-blue` (chỉ CSS, `theme/tokens.css` đã nằm trong kế hoạch).

## ADR-010: Quy ước lỗi, mã lỗi mã mời và rate limit
- **Trạng thái:** Đề xuất
- **Quyết định:** Mọi lỗi trả `{"error": {"code", "message"}}` với mã HTTP trong `400/401/403/404/409/429`. Mã mời hết hạn hoặc hết lượt trả `400` (`INVITE_EXPIRED`, `INVITE_EXHAUSTED`). Rate limit trả `429 RATE_LIMITED`: 30 tin / 10 giây / user, 10 lần đăng nhập / phút / IP.
- **Lý do:** Giữ trong tập mã của kế hoạch gốc (không có `410`). Ngưỡng 30 để TC-WS-02 (20 tin liên tiếp) không bị chặn.

## ADR-011: RESYNC thay cache thay vì ghép
- **Trạng thái:** Đề xuất
- **Quyết định:** Từ lần `READY` thứ hai, client invalidate cache `['messages']` (và danh sách thành viên) rồi tải lại bằng REST; kết quả thay thế cache.
- **Lý do:** Khớp `useGateway` trong kế hoạch gốc §3.2; không cần tham số `after`; tin sửa/xóa lỡ lúc mất mạng cũng được cập nhật.

## ADR-012: Presence offline do Hub phát
- **Trạng thái:** Đề xuất
- **Quyết định:** Khi kết nối cuối cùng của user đóng, Hub phát `PRESENCE_UPDATE` offline. Key `presence:{userID}` (TTL 60 giây) chỉ để `GET members` trả đúng khi server chết đột ngột.
- **Lý do:** Key Redis hết hạn không tự phát sự kiện; Hub đã có map `users`. Giới hạn đã biết: với 2 instance, user nối cả hai có thể bị báo offline sớm tối đa một nhịp heartbeat (25 giây).

## ADR-013: Vé WebSocket sai trả HTTP 401
- **Trạng thái:** Đề xuất
- **Quyết định:** Vé sai hoặc hết hạn bị từ chối bằng HTTP `401` ở bước nâng cấp; bỏ mã `4001`. Mã `4003` do client phát khi mất heartbeat.
- **Lý do:** Khớp `openapi.yaml`, TC-WS-01 và code mẫu §3.1; client luôn lấy vé mới khi kết nối lại nên không cần mã riêng.

## ADR-014: Vai trò `admin` gán bằng SQL trong sprint
- **Trạng thái:** Đề xuất
- **Quyết định:** Không thêm API đổi vai trò; `admin` được gán bằng `UPDATE guild_members` khi kiểm thử. Nếu còn giờ sau Ngày 8, thêm `PATCH /guilds/{guildId}/members/{userId}` (chỉ Owner).
- **Lý do:** Kế hoạch gốc có các thao tác "Owner/admin" nhưng không có endpoint gán vai trò; API nhỏ nhưng không thuộc `Must`/`Should` đã chốt.