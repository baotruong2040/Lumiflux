# Đặc tả Yêu cầu & Tiêu chí Nghiệm thu (`requirements.md`)

- **Dự án:** Discord Clone (Web + Desktop đa nền tảng)
- **Thời gian triển khai (Sprint 14 ngày):** 16/11/2026 – 29/11/2026 *(nén từ lộ trình gốc 7 tuần)*
- **Nền tảng phát triển:** CachyOS (Linux)
- **Kiến trúc mục tiêu:** Go (Clean Architecture) + PostgreSQL + Redis + React/TypeScript + Tauri 2

---

## 1. Phân loại Phạm vi Chức năng (MoSCoW)

### 1.1. Nhóm Bắt buộc (`Must` — 100% triển khai)
1. **Xác thực & Phiên đăng nhập:** Đăng ký, đăng nhập, cấp cặp JWT Access Token (15 phút) + Refresh Token (7 ngày) xoay vòng lưu Redis, đăng xuất hủy token, tự động refresh khi gặp HTTP `401`.
2. **Quản lý Server (Guild) & Mã mời (Invite):** Tạo server (tự động tạo kênh `#general` và gán người tạo làm `owner`), xem danh sách server tham gia, cập nhật/xóa server (`owner`), tạo mã mời, tham gia server qua mã mời, rời server (`admin`/`member`).
3. **Quản lý Kênh văn bản (Channel):** Xem danh sách kênh; tạo, đổi tên/mô tả, xóa kênh (`owner`/`admin`).
4. **Nhắn tin & Lịch sử qua REST:** Gửi tin nhắn văn bản (`1–2.000` ký tự) kèm `nonce`; tải lịch sử tin nhắn bằng cursor pagination theo `UUIDv7` (`?before=<id>&limit=50`) kèm thông tin tác giả.
5. **Giao tiếp Realtime qua WebSocket:** Cấp vé dùng một lần qua `POST /api/v1/ws/ticket` (Redis `GETDEL`, TTL 30 giây); duy trì kết nối qua `READY`, `HEARTBEAT`/`HEARTBEAT_ACK`; phát tán sự kiện qua Redis Pub/Sub; tự động reconnect (backoff + jitter) và đồng bộ lại lịch sử (`RESYNC`) sau khi mất mạng.
6. **Giao diện Dark Mode & Đa nền tảng:** Giao diện 3 cột phong cách Discord dùng CSS variables (`discord-night`), chạy mượt trên cả trình duyệt Web (Vite) và cửa sổ Desktop (Tauri 2) từ một codebase.

### 1.2. Nhóm Nên có (`Should` — Đã chốt đưa vào Sprint 14 ngày)
1. **Typing Indicator:** Phát tín hiệu đang nhập (`TYPING_START`), giới hạn tần suất 3 giây/lần trên Redis, hiển thị trạng thái `USER_TYPING` ở kênh tương ứng.
2. **Sửa và Xóa tin nhắn:** Tác giả được sửa nội dung tin nhắn (`PATCH`), tác giả hoặc `owner`/`admin` được xóa mềm tin nhắn (`DELETE`), đồng bộ realtime qua `MESSAGE_UPDATE` và `MESSAGE_DELETE`.
3. **Hiển thị trạng thái Online / Offline (Presence):** Gia hạn key `presence:{userID}` (TTL 60 giây) theo nhịp heartbeat, phát `PRESENCE_UPDATE` khi thay đổi trạng thái.
4. **Danh sách thành viên (Member List):** Hiển thị danh sách thành viên trong server kèm vai trò (`owner`, `admin`, `member`) và trạng thái online/offline; cập nhật realtime qua `GUILD_MEMBER_ADD` và `GUILD_MEMBER_REMOVE`.

### 1.3. Nhóm Tạm hoãn & Ngoài phạm vi
- **Should (Tạm hoãn — chỉ làm nếu hoàn thành sớm toàn bộ mục trên trước Ngày 14):** Hệ thống Theme JSON tùy chỉnh (`steam-blue`, import JSON), Animated Pixel-art Cursor.
- **Could (Đóng băng):** Emoji/Reaction, Tin nhắn riêng 1-1 (DM), Upload hình ảnh qua MinIO, Thông báo Desktop native.
- **Won't (Ngoài phạm vi):** Voice/Video call, ứng dụng Mobile native (iOS/Android), mã hóa đầu cuối (E2EE).

---

## 2. Chi tiết Hành vi & Tiêu chí Nghiệm thu (Acceptance Criteria)

### 2.1. Authentication & User Profile
| Hạng mục | Chi tiết |
| :--- | :--- |
| **Actor** | Người dùng chưa đăng nhập (Register/Login/Refresh) hoặc đã đăng nhập (Logout/Profile). |
| **Input** | `username` (duy nhất, không phân biệt hoa thường), `email` (duy nhất, không phân biệt hoa thường), `displayName`, `password`. |
| **Điều kiện chấp nhận** | `username` và `email` chưa tồn tại trong bảng `users`; mật khẩu được băm bằng `bcrypt` trước khi lưu. |
| **Thay đổi dữ liệu** | Tạo bản ghi `users` (ID `UUIDv7`); khi login/refresh lưu `refresh:{hash}` vào Redis với TTL 7 ngày và xóa hash cũ. |
| **Lỗi cần từ chối** | `400` dữ liệu sai định dạng; `401` sai mật khẩu / token hết hạn / refresh token đã dùng; `409` trùng username hoặc email. |
| **Tiêu chí nghiệm thu** | - Không lưu mật khẩu thô trong DB.<br>- Refresh token cũ không thể dùng lại lần thứ 2.<br>- Frontend tự refresh token và gọi lại API gốc mượt mà trên cả Web và Tauri. |

### 2.2. Guild, Membership & Invite
| Hạng mục | Chi tiết |
| :--- | :--- |
| **Actor** | Người dùng đã đăng nhập; phân quyền theo `owner`, `admin`, `member`. |
| **Quy tắc nghiệp vụ** | - Tạo server thực hiện trong 1 DB Transaction: tạo `guilds` + tạo `channels` (`general`) + thêm `guild_members` (`role = 'owner'`).<br>- Mọi thành viên trong server đều có quyền tạo mã mời (`POST /guilds/:guildId/invites`).<br>- `Owner` **không được** gọi API rời server (`403 FORBIDDEN`); muốn thoát phải xóa server (`DELETE /guilds/:guildId`).<br>- Chỉ `Owner` được đổi tên/icon hoặc xóa server. |
| **Lỗi cần từ chối** | `403` không phải thành viên hoặc không đủ quyền; `404` server/mã mời không tồn tại; `409` đã là thành viên; `410/400` mã mời hết hạn hoặc hết lượt `max_uses`. |
| **Tiêu chí nghiệm thu** | - Tạo server xong thấy ngay kênh `#general` và quyền `owner`.<br>- Tài khoản khác nhập mã mời tham gia được server; danh sách thành viên cập nhật ngay lập tức. |

### 2.3. Channel (Kênh văn bản)
| Hạng mục | Chi tiết |
| :--- | :--- |
| **Actor** | Thành viên server (`GET`); `Owner` hoặc `Admin` (`POST`, `PATCH`, `DELETE`). |
| **Điều kiện chấp nhận** | Tên kênh hợp lệ (`1–100` ký tự), không trùng tên trong cùng một server (`UNIQUE (guild_id, name)`). |
| **Lỗi cần từ chối** | `403` `member` cố tình tạo/sửa/xóa kênh; `409` trùng tên kênh trong server. |
| **Tiêu chí nghiệm thu** | - `Owner`/`Admin` tạo, đổi tên, xóa kênh thành công.<br>- `Member` bị chặn ở cả giao diện lẫn API (`403`). |

### 2.4. Message (Nhắn tin, Lịch sử, Sửa & Xóa)
| Hạng mục | Chi tiết |
| :--- | :--- |
| **Actor** | Thành viên server chứa kênh đó. |
| **Điều kiện chấp nhận** | Nội dung sau `TrimSpace` có độ dài từ `1` đến `2.000` ký tự. Kèm `nonce` (tối đa 64 ký tự) khi gửi mới. |
| **Thay đổi dữ liệu** | - **Gửi:** `INSERT` vào `messages` (`UUIDv7`), sau đó publish `MESSAGE_CREATE` lên Redis.<br>- **Sửa:** Chỉ tác giả được sửa; cập nhật `content` và `edited_at`, publish `MESSAGE_UPDATE`.<br>- **Xóa:** Tác giả hoặc `owner`/`admin` được xóa; cập nhật `deleted_at` (soft delete), publish `MESSAGE_DELETE`. |
| **Tải lịch sử** | `GET /channels/:channelId/messages?before=<id>&limit=50` truy vấn `WHERE channel_id = $1 AND id < $2 AND deleted_at IS NULL ORDER BY id DESC LIMIT 50` kèm `JOIN users`. |
| **Tiêu chí nghiệm thu** | - Gửi tin lạc quan (optimistic update) thay thế tin tạm bằng tin thật qua `nonce`, không bao giờ hiển thị trùng 2 tin.<br>- Cuộn lên trên tải thêm lịch sử mượt mà với dữ liệu 1.000 tin nhắn.<br>- Tin đã sửa hiện nhãn "(đã chỉnh sửa)", tin đã xóa biến mất khỏi khung chat của mọi client. |

### 2.5. Realtime Gateway, Typing & Presence
| Hạng mục | Chi tiết |
| :--- | :--- |
| **Cơ chế kết nối** | Client gọi `POST /api/v1/ws/ticket` nhận vé 30s -> mở `GET /ws?ticket=...` -> Server tiêu thụ vé bằng `GETDEL`. |
| **Heartbeat & Reconnect** | Server gửi `heartbeatIntervalMs` (25.000 ms) trong `READY`. Client gửi `HEARTBEAT` định kỳ; nếu quá 10 giây không nhận `HEARTBEAT_ACK`, client chủ động ngắt và reconnect theo exponential backoff + jitter (tối đa 30s), xin vé mới và kích hoạt `RESYNC`. |
| **Typing & Presence** | - `TYPING_START`: kiểm tra quyền thành viên, dùng `SET typing:{channelID}:{userID} 1 NX EX 3` trên Redis để chặn spam.<br>- `Presence`: mỗi `HEARTBEAT` gia hạn `SET presence:{userID} online EX 60`. |
| **Tiêu chí nghiệm thu** | - Hai client chat qua lại nhận tin dưới 100 ms trên mạng nội bộ.<br>- Ngắt mạng 10 giây rồi bật lại: tự kết nối lại và đồng bộ đủ tin nhắn bị lỡ trong lúc rớt mạng.<br>- `go test -race ./...` không phát hiện data race hay rò rỉ goroutine. |