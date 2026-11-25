# Tài liệu REST API (`docs/api.md`)

Tài liệu mô tả chi tiết các endpoint đã triển khai trong `discord-clone-api`, đồng bộ với hợp đồng đặc tả [api/openapi.yaml](../api/openapi.yaml).

- **Base URL (Local Dev):** `http://localhost:8080`
- **Định dạng dữ liệu:** `application/json; charset=utf-8` (JSON fields dùng `camelCase`)
- **Xác thực:** JWT Bearer Token (`Authorization: Bearer <accessToken>`, thuật toán `HS256`, TTL mặc định `15m`)
- **Refresh Token:** Chuỗi ngẫu nhiên 32 byte (Base64URL), lưu hash SHA-256 trên Redis tại `refresh:{hash}` (TTL `7 ngày`), xoay vòng nguyên tử bằng lệnh `GETDEL`

---

## 1. Quy ước chung

### 1.1. Headers
| Header | Hướng | Mô tả |
| :--- | :--- | :--- |
| `Authorization` | Request | `Bearer <accessToken>` cho các endpoint yêu cầu đăng nhập. |
| `Content-Type` | Request / Response | `application/json` |
| `X-Request-ID` | Request / Response | Trace ID của request. Nếu client không gửi (hoặc không hợp lệ), server tự sinh `UUIDv7` và gắn vào header phản hồi + structured logs (`log/slog`). |
| `Retry-After` | Response (`429`) | Số giây tối thiểu client cần chờ trước khi gửi lại request. |

### 1.2. CORS (Allowed Origins)
Các origin mặc định được phép (cấu hình qua `CORS_ALLOWED_ORIGINS`):
- `http://localhost:5173` (Web Vite dev)
- `tauri://localhost` (Tauri Desktop)
- `http://tauri.localhost`
- `https://tauri.localhost`

Methods cho phép: `GET, POST, PUT, PATCH, DELETE, OPTIONS`.  
Headers cho phép: `Authorization, Content-Type, X-Request-ID`.  
Headers expose: `X-Request-ID, Retry-After`.

### 1.3. Cấu trúc lỗi chuẩn
Mọi phản hồi lỗi đều tuân theo schema `ErrorResponse`:

```json
{
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "invalid username/email or password"
  }
}
```

| HTTP Status | Mã `error.code` phổ biến | Ý nghĩa |
| :--- | :--- | :--- |
| `400` | `VALIDATION_ERROR` | JSON không hợp lệ hoặc dữ liệu không đạt quy tắc kiểm tra. |
| `401` | `UNAUTHORIZED` | Thiếu, sai định dạng hoặc access token hết hạn. |
| `401` | `INVALID_CREDENTIALS` | Sai username/email hoặc mật khẩu. |
| `401` | `INVALID_REFRESH_TOKEN` | Refresh token không tồn tại, đã dùng hoặc đã hết hạn. |
| `403` | `FORBIDDEN` | Không có quyền thực hiện thao tác. |
| `404` | `USER_NOT_FOUND` / `GUILD_NOT_FOUND` / `INVITE_NOT_FOUND` | Không tìm thấy tài nguyên. |
| `409` | `USERNAME_TAKEN` / `EMAIL_TAKEN` / `ALREADY_MEMBER` / `CONFLICT` | Dữ liệu xung đột hoặc đã tồn tại. |
| `429` | `RATE_LIMITED` | Vượt giới hạn tần suất (kèm header `Retry-After`). |
| `500` | `INTERNAL_ERROR` | Lỗi nội bộ server (panic cũng được middleware `Recover` bắt an toàn). |
| `503` | *(xem `/readyz`)* | Phụ thuộc PostgreSQL hoặc Redis không sẵn sàng. |

---

## 2. Health & Readiness Endpoints

### 2.1. `GET /healthz`
Kiểm tra tiến trình API đang sống (liveness probe). Không yêu cầu đăng nhập.

**Response `200 OK`:**
```json
{
  "status": "ok"
}
```

### 2.2. `GET /readyz`
Kiểm tra khả năng kết nối tới PostgreSQL (`pgxpool`) và Redis (`go-redis`) với timeout 2 giây. Không yêu cầu đăng nhập.

**Response `200 OK`:**
```json
{
  "status": "ready",
  "checks": {
    "postgres": "ok",
    "redis": "ok"
  }
}
```

**Response `503 Service Unavailable`:**
```json
{
  "status": "not_ready",
  "checks": {
    "postgres": "ok",
    "redis": "unavailable"
  }
}
```

---

## 3. Authentication Endpoints

### 3.1. `POST /api/v1/auth/register`
Đăng ký tài khoản mới, tạo ID `UUIDv7`, băm mật khẩu bằng `bcrypt`, lưu vào PostgreSQL và cấp cặp `accessToken` + `refreshToken`.

**Request Body:**
```json
{
  "username": "truong_dev",
  "email": "truong@example.com",
  "displayName": "Truong",
  "password": "Password123!"
}
```

**Quy tắc validate:**
- `username`: 3–32 ký tự, chỉ gồm chữ cái, số, `.`, `_`, `-` (`^[a-zA-Z0-9._-]{3,32}$`), duy nhất không phân biệt hoa thường.
- `email`: định dạng email hợp lệ, tối đa 254 ký tự, duy nhất không phân biệt hoa thường.
- `displayName`: 1–64 ký tự (sau `TrimSpace`).
- `password`: tối thiểu 8 ký tự, tối đa 72 byte (giới hạn `bcrypt`).

**Response `201 Created`:**
```json
{
  "accessToken": "<jwt-access-token>",
  "refreshToken": "<opaque-refresh-token>",
  "user": {
    "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
    "username": "truong_dev",
    "email": "truong@example.com",
    "displayName": "Truong",
    "avatarUrl": null,
    "createdAt": "2026-11-25T13:20:00Z"
  }
}
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `409 USERNAME_TAKEN`, `409 EMAIL_TAKEN`.

---

### 3.2. `POST /api/v1/auth/login`
Đăng nhập bằng `username` hoặc `email` (không phân biệt hoa thường) và mật khẩu.

- **Rate Limit:** Giới hạn mặc định **10 request / 1 phút / IP** (Redis key `rl:login:{ip}`). Request thứ 11 trong cùng cửa sổ thời gian trả về `429 RATE_LIMITED` kèm header `Retry-After`.

**Request Body:**
```json
{
  "login": "truong@example.com",
  "password": "Password123!"
}
```

**Response `200 OK`:** Cấu trúc `AuthResponse` giống `/api/v1/auth/register`.

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 INVALID_CREDENTIALS`, `429 RATE_LIMITED`.

---

### 3.3. `POST /api/v1/auth/refresh`
Xoay vòng (rotate) refresh token. Server băm SHA-256 `refreshToken`, tiêu thụ nguyên tử key `refresh:{hash}` trên Redis bằng `GETDEL` rồi cấp cặp token mới. Refresh token cũ không thể tái sử dụng.

**Request Body:**
```json
{
  "refreshToken": "<opaque-refresh-token>"
}
```

**Response `200 OK`:** Cấu trúc `AuthResponse` với `accessToken` và `refreshToken` mới.

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 INVALID_REFRESH_TOKEN`.

---

### 3.4. `POST /api/v1/auth/logout`
Đăng xuất và thu hồi `refreshToken` khỏi Redis (`DEL refresh:{hash}`).

- **Yêu cầu xác thực:** `Authorization: Bearer <accessToken>`

**Request Body:**
```json
{
  "refreshToken": "<opaque-refresh-token>"
}
```

**Response:** `204 No Content` (không có body).

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`.

---

## 4. User Profile Endpoints

Tất cả endpoint trong nhóm `/api/v1/users` yêu cầu header `Authorization: Bearer <accessToken>`.

### 4.1. `GET /api/v1/users/@me`
Lấy thông tin hồ sơ của người dùng đang đăng nhập.

**Response `200 OK`:**
```json
{
  "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
  "username": "truong_dev",
  "email": "truong@example.com",
  "displayName": "Truong",
  "avatarUrl": null,
  "createdAt": "2026-11-25T13:20:00Z"
}
```

**Lỗi có thể trả về:** `401 UNAUTHORIZED`, `404 USER_NOT_FOUND`.

---

### 4.2. `PATCH /api/v1/users/@me`
Cập nhật một phần hồ sơ người dùng (`displayName` và/hoặc `avatarUrl`).

**Request Body:**
```json
{
  "displayName": "Truong Updated",
  "avatarUrl": "https://cdn.example.com/avatar.png"
}
```

**Hành vi của trường `avatarUrl`:**
- **Bỏ qua trường `avatarUrl`** trong JSON: giữ nguyên giá trị hiện tại trong DB.
- **Truyền `"avatarUrl": null`** (hoặc chuỗi rỗng `""`): xóa avatar hiện tại (đặt `NULL` trong DB).
- **Truyền URL:** phải là URL `http://` hoặc `https://` hợp lệ, tối đa 2048 ký tự.

**Response `200 OK`:** Trả về đối tượng `User` sau khi cập nhật.

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `404 USER_NOT_FOUND`.

---

## 5. Guild, Membership & Invite Endpoints

Tất cả endpoint trong nhóm `/api/v1/guilds` và `/api/v1/invites` yêu cầu header `Authorization: Bearer <accessToken>`.

### 5.1. `POST /api/v1/guilds`
Tạo server mới trong **1 DB transaction**: chèn bản ghi `guilds`, tạo kênh mặc định `#general` (`type = 0`, `position = 0`), và thêm người tạo vào `guild_members` với vai trò `owner`.

**Request Body:**
```json
{
  "name": "Lumiflux HQ",
  "iconUrl": "https://cdn.example.com/icon.png"
}
```
- `name`: 1–100 ký tự (sau `TrimSpace`).
- `iconUrl`: tùy chọn; nếu có phải là URL `http://` hoặc `https://` hợp lệ (tối đa 2048 ký tự).

**Response `201 Created`:**
```json
{
  "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6c",
  "name": "Lumiflux HQ",
  "iconUrl": "https://cdn.example.com/icon.png",
  "ownerId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
  "createdAt": "2026-11-25T14:00:00Z"
}
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`.

### 5.2. `GET /api/v1/guilds`
Lấy danh sách các server mà người dùng hiện tại đang tham gia (`[]` nếu chưa tham gia server nào).

**Response `200 OK`:** Mảng `Guild[]`.

### 5.3. `GET /api/v1/guilds/:guildId`
Xem chi tiết một server (yêu cầu người gọi là thành viên của server).

**Response `200 OK`:** Đối tượng `Guild`.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.4. `PATCH /api/v1/guilds/:guildId`
Cập nhật `name` và/hoặc `iconUrl` của server (**chỉ `owner`**).

**Request Body:**
```json
{
  "name": "Lumiflux Updated",
  "iconUrl": null
}
```
- Bỏ qua `iconUrl`: giữ nguyên giá trị cũ; truyền `null` hoặc `""`: xóa icon.

**Response `200 OK`:** Đối tượng `Guild` sau khi cập nhật.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.5. `DELETE /api/v1/guilds/:guildId`
Xóa server (**chỉ `owner`**; cascade xóa kênh, thành viên và mã mời liên quan).

**Response:** `204 No Content`.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.6. `GET /api/v1/guilds/:guildId/members`
Lấy danh sách thành viên trong server kèm vai trò (`owner`, `admin`, `member`), thông tin `user` và trạng thái `online` (đọc qua Redis `MGET presence:{userID}`).

**Response `200 OK`:** Mảng `GuildMember[]`:
```json
[
  {
    "guildId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6c",
    "userId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
    "nickname": null,
    "role": "owner",
    "online": false,
    "joinedAt": "2026-11-25T14:00:00Z",
    "user": {
      "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
      "username": "truong_dev",
      "email": "truong@example.com",
      "displayName": "Truong",
      "avatarUrl": null,
      "createdAt": "2026-11-25T13:20:00Z"
    }
  }
]
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.7. `DELETE /api/v1/guilds/:guildId/members/@me`
Rời khỏi server và kích hoạt hook `GuildSubscriptionManager.Unsubscribe` (chuẩn bị cho WebSocket Hub ADR-007).  
- Theo **ADR-006**, **`owner` bị chặn** với `403 FORBIDDEN` (muốn thoát phải dùng `DELETE /api/v1/guilds/:guildId`).

**Response:** `204 No Content`.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.8. `POST /api/v1/guilds/:guildId/invites`
Tạo mã mời 8 ký tự cho server (**mọi thành viên** đều có quyền tạo theo **ADR-005**). Request body là tùy chọn.

**Request Body (tùy chọn):**
```json
{
  "maxUses": 10,
  "expiresInHours": 24
}
```
- Bỏ qua body hoặc bỏ qua `expiresInHours`: mặc định hết hạn sau **7 ngày** (`168h`), `maxUses = null` (không giới hạn).
- Truyền `"expiresInHours": null`: không giới hạn thời gian.

**Response `201 Created`:**
```json
{
  "code": "aB3k9ZpQ",
  "guildId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6c",
  "createdBy": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
  "maxUses": 10,
  "uses": 0,
  "expiresAt": "2026-11-26T14:00:00Z",
  "createdAt": "2026-11-25T14:00:00Z"
}
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 5.9. `POST /api/v1/invites/:code/accept`
Chấp nhận mã mời trong 1 transaction (`SELECT ... FOR UPDATE`), tăng `uses`, thêm thành viên với vai trò `member` và gọi `GuildSubscriptionManager.Subscribe`.

**Response `200 OK`:** Đối tượng `Guild`.  
**Lỗi có thể trả về:**
- `400 INVITE_EXPIRED`: mã mời đã quá hạn `expiresAt`.
- `400 INVITE_EXHAUSTED`: mã mời đã đạt `maxUses`.
- `404 INVITE_NOT_FOUND`: mã mời không tồn tại.
- `409 ALREADY_MEMBER`: người dùng đã là thành viên server này.

---

## 6. Channel (Kênh văn bản)

Tất cả endpoint nhóm Channel yêu cầu header `Authorization: Bearer <accessToken>` và xác thực tư cách thành viên guild.  
Các thao tác ghi (`POST`, `PATCH`, `DELETE`) chỉ cho phép vai trò `owner` hoặc `admin`; vai trò `member` bị từ chối với `403 FORBIDDEN`.

### 6.1. `GET /api/v1/guilds/:guildId/channels`
Lấy danh sách kênh của server (sắp xếp theo `position ASC, created_at ASC, id ASC`).

**Response `200 OK`:** Mảng đối tượng `Channel`:
```json
[
  {
    "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6d",
    "guildId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6c",
    "name": "general",
    "type": 0,
    "topic": null,
    "position": 0,
    "createdAt": "2026-11-25T14:00:00Z"
  }
]
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`.

### 6.2. `POST /api/v1/guilds/:guildId/channels`
Tạo kênh văn bản mới (`type = 0`), chỉ `owner` hoặc `admin`. Tự động tăng `position` tiếp nối trong guild.

**Request Body:**
```json
{
  "name": "backend-api",
  "topic": "Thảo luận kiến trúc Go & PostgreSQL"
}
```
- `name`: chuỗi `1–100` ký tự sau khi `TrimSpace`, duy nhất trong cùng guild (`UNIQUE (guild_id, name)`).
- `topic`: tùy chọn (`null` hoặc tối đa `255` ký tự).

**Response `201 Created`:** Đối tượng `Channel`.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 GUILD_NOT_FOUND`, `409 CHANNEL_NAME_TAKEN`.

### 6.3. `PATCH /api/v1/channels/:channelId`
Đổi tên hoặc cập nhật/xóa `topic` của kênh, chỉ `owner` hoặc `admin`.

**Request Body:**
```json
{
  "name": "backend-core",
  "topic": null
}
```

**Response `200 OK`:** Đối tượng `Channel` đã cập nhật.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND`, `409 CHANNEL_NAME_TAKEN`.

### 6.4. `DELETE /api/v1/channels/:channelId`
Xóa kênh khỏi server, chỉ `owner` hoặc `admin`.

**Response `204 No Content`** (không có body).  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND`.

---

## 7. Message (Tin nhắn & Cursor Pagination)

Tất cả endpoint nhóm Message yêu cầu `Authorization: Bearer <accessToken>` và xác thực tư cách thành viên của guild chứa kênh.

### 7.1. `GET /api/v1/channels/:channelId/messages`
Tải lịch sử tin nhắn chưa bị xóa mềm (`deleted_at IS NULL`) bằng cursor pagination theo `UUIDv7` giảm dần (`WHERE channel_id = $1 AND id < $before ORDER BY id DESC LIMIT $limit`).

**Query Parameters:**
- `before` *(tùy chọn, UUID)*: ID của tin nhắn cũ nhất ở trang trước; bỏ qua để lấy trang mới nhất.
- `limit` *(tùy chọn, số nguyên `1–100`, mặc định `50`)*.

**Response `200 OK`:** Mảng đối tượng `Message`:
```json
[
  {
    "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a70",
    "channelId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6d",
    "author": {
      "id": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6b",
      "username": "truong",
      "displayName": "Truong",
      "avatarUrl": null
    },
    "content": "Xin chào mọi người!",
    "nonce": null,
    "editedAt": null,
    "createdAt": "2026-11-25T15:00:00Z"
  }
]
```

**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND`.

### 7.2. `POST /api/v1/channels/:channelId/messages`
Gửi tin nhắn mới kèm kiểm tra độ dài sau `TrimSpace` (`1–2.000` ký tự Unicode), tùy chọn `nonce` (tối đa `64` ký tự), và giới hạn tốc độ theo user trên Redis (`rl:{userID}:msg`, mặc định `30` tin / `10` giây).

**Request Body:**
```json
{
  "content": "Xin chào mọi người!",
  "nonce": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response `201 Created`:** Đối tượng `Message` (trả lại `nonce` khớp từ request).  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND`, `429 RATE_LIMITED` (kèm header `Retry-After`).

### 7.3. `PATCH /api/v1/channels/:channelId/messages/:messageId`
Sửa nội dung tin nhắn và cập nhật `editedAt`; chỉ **tác giả** tin nhắn được phép thực hiện.

**Request Body:**
```json
{
  "content": "Nội dung đã cập nhật"
}
```

**Response `200 OK`:** Đối tượng `Message` với `editedAt != null`.  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND` / `MESSAGE_NOT_FOUND`.

### 7.4. `DELETE /api/v1/channels/:channelId/messages/:messageId`
Xóa mềm tin nhắn (`deleted_at = now()`); cho phép **tác giả** hoặc **`owner`/`admin`** của guild.

**Response `204 No Content`** (không có body).  
**Lỗi có thể trả về:** `400 VALIDATION_ERROR`, `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 CHANNEL_NOT_FOUND` / `MESSAGE_NOT_FOUND`.

---

## 8. WebSocket Ticket & Gateway (`/ws`)

### 8.1. `POST /api/v1/ws/ticket`
Cấp vé dùng một lần để nâng cấp kết nối WebSocket (ADR-004). Yêu cầu header `Authorization: Bearer <accessToken>`. Server lưu khóa `wsticket:{ticket}` trên Redis với TTL mặc định **30 giây** (`WS_TICKET_TTL`).

**Response `200 OK`:**
```json
{
  "ticket": "2pG9c4kM..."
}
```

**Lỗi có thể trả về:** `401 UNAUTHORIZED`.

### 8.2. `GET /ws?ticket=<ticket>`
Nâng cấp kết nối lên WebSocket sau khi:
1. Kiểm tra `Origin` nằm trong `CORS_ALLOWED_ORIGINS` (`http://localhost:5173`, `http://127.0.0.1:5173`, `tauri://localhost`, `http://tauri.localhost`, `https://tauri.localhost`; không cho phép origin lạ -> `403 FORBIDDEN`; cho phép client không gửi `Origin`).
2. Tiêu thụ vé nguyên tử bằng lệnh Redis `GETDEL wsticket:{ticket}` -> trả `401 UNAUTHORIZED` (`INVALID_TICKET`) nếu thiếu, sai hoặc đã dùng trước đó.
3. Tải danh sách server user tham gia, đăng ký vào `Hub` actor và gửi frame `READY`.

**Sự kiện đã triển khai (Ngày 12):**
- Server $\rightarrow$ Client: `READY` (`{ userId, guildIds, heartbeatIntervalMs }`), `HEARTBEAT_ACK` (`{}`), `ERROR` (`{ code, message }`).
- Client $\rightarrow$ Server: `HEARTBEAT` (làm mới read deadline & trả `HEARTBEAT_ACK`), `TYPING_START` *(hook sẵn sàng cho Ngày 13–14)*.
- Mã đóng WebSocket: `1000` (đóng bình thường), `1001` (server shutdown), `4002` (slow consumer — tràn hàng đợi gửi 256 frame), `4003` (client tự đóng khi mất heartbeat).

---

## 9. Biến môi trường cấu hình (`internal/config`)

| Biến | Mặc định (Dev) | Mô tả |
| :--- | :--- | :--- |
| `APP_ENV` | `development` | Môi trường (`development`, `test`, `production`). |
| `HOST` | `127.0.0.1` *(dev)* / *(rỗng trong production)* | Địa chỉ bind HTTP. Mặc định `127.0.0.1` khi dev trên Windows để tránh cảnh báo Windows Firewall; đặt `0.0.0.0` hoặc rỗng khi deploy container. |
| `PORT` | `8080` | Cổng lắng nghe HTTP. |
| `LOG_LEVEL` | `info` | Mức log `slog` (`debug`, `info`, `warn`, `error`). |
| `DATABASE_URL` | `postgres://discord:discord_secret_dev@localhost:5432/discord_clone?sslmode=disable` | Chuỗi kết nối PostgreSQL (`pgxpool`). |
| `REDIS_ADDR` | `localhost:6379` | Địa chỉ Redis. |
| `REDIS_PASSWORD` | *(rỗng)* | Mật khẩu Redis (nếu có). |
| `REDIS_DB` | `0` | Chỉ số database Redis. |
| `JWT_SECRET` | *(dev secret)* | Khóa bí mật ký HS256 (bắt buộc tùy chỉnh ≥ 32 ký tự khi `APP_ENV=production`). |
| `JWT_ACCESS_TTL` | `15m` | Thời hạn Access Token. |
| `JWT_REFRESH_TTL` | `168h` | Thời hạn Refresh Token (7 ngày). |
| `BCRYPT_COST` | `10` | Chi phí băm `bcrypt`. |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173,tauri://localhost,http://tauri.localhost,https://tauri.localhost` | Danh sách origin cho phép, ngăn cách bằng dấu phẩy. |
| `LOGIN_RATE_LIMIT` | `10` | Số lần đăng nhập tối đa trong mỗi cửa sổ/IP. |
| `LOGIN_RATE_WINDOW` | `1m` | Độ dài cửa sổ rate limit đăng nhập. |
| `MESSAGE_RATE_LIMIT` | `30` | Số tin nhắn tối đa trong mỗi cửa sổ/user. |
| `MESSAGE_RATE_WINDOW` | `10s` | Độ dài cửa sổ rate limit gửi tin nhắn. |
| `WS_TICKET_TTL` | `30s` | Thời hạn sống của vé nâng cấp WebSocket trên Redis. |
| `WS_HEARTBEAT_INTERVAL` | `25s` | Chu kỳ heartbeat gửi cho client trong frame `READY`. |
| `SHUTDOWN_TIMEOUT` | `10s` | Thời gian tối đa chờ graceful shutdown. |

---

## 10. Trạng thái triển khai theo hợp đồng `openapi.yaml`

| Nhóm API | Endpoint | Trạng thái |
| :--- | :--- | :--- |
| **Health** | `GET /healthz`, `GET /readyz` | Đã triển khai |
| **Auth** | `POST /api/v1/auth/register`, `/login`, `/refresh`, `/logout` | Đã triển khai |
| **Users** | `GET /api/v1/users/@me`, `PATCH /api/v1/users/@me` | Đã triển khai |
| **Guilds & Invites** | `/api/v1/guilds/**`, `/api/v1/invites/{code}/accept` | Đã triển khai |
| **Channels** | `/api/v1/guilds/{guildId}/channels`, `/api/v1/channels/{channelId}` | Đã triển khai |
| **Messages** | `/api/v1/channels/{channelId}/messages/**` | Đã triển khai |
| **WebSocket** | `POST /api/v1/ws/ticket`, `GET /ws?ticket=...` | Đã triển khai *(Redis Pub/Sub & `GatewayClient` FE vào Ngày 13)* |
