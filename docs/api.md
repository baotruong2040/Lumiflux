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
| `404` | `USER_NOT_FOUND` / `NOT_FOUND` | Không tìm thấy tài nguyên. |
| `409` | `USERNAME_TAKEN` / `EMAIL_TAKEN` / `CONFLICT` | Trùng `username` hoặc `email` (không phân biệt hoa/thường nhờ PostgreSQL `CITEXT`). |
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

## 5. Biến môi trường cấu hình (`internal/config`)

| Biến | Mặc định (Dev) | Mô tả |
| :--- | :--- | :--- |
| `APP_ENV` | `development` | Môi trường (`development`, `test`, `production`). |
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
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173,tauri://localhost,http://tauri.localhost,https://tauri.localhost` | Danh sách origin cho phép, ngăn cách bằng dấu phẩy. |
| `LOGIN_RATE_LIMIT` | `10` | Số lần đăng nhập tối đa trong mỗi cửa sổ/IP. |
| `LOGIN_RATE_WINDOW` | `1m` | Độ dài cửa sổ rate limit đăng nhập. |
| `SHUTDOWN_TIMEOUT` | `10s` | Thời gian tối đa chờ graceful shutdown. |

---

## 6. Trạng thái triển khai theo hợp đồng `openapi.yaml`

| Nhóm API | Endpoint | Trạng thái |
| :--- | :--- | :--- |
| **Health** | `GET /healthz`, `GET /readyz` | Đã triển khai |
| **Auth** | `POST /api/v1/auth/register`, `/login`, `/refresh`, `/logout` | Đã triển khai |
| **Users** | `GET /api/v1/users/@me`, `PATCH /api/v1/users/@me` | Đã triển khai |
| **Guilds & Invites** | `/api/v1/guilds/**`, `/api/v1/invites/{code}/accept` | Kế hoạch (Ngày 8) |
| **Channels** | `/api/v1/guilds/{guildId}/channels`, `/api/v1/channels/{channelId}` | Kế hoạch (Ngày 9) |
| **Messages** | `/api/v1/channels/{channelId}/messages/**` | Kế hoạch (Ngày 10) |
| **WebSocket** | `POST /api/v1/ws/ticket`, `GET /ws?ticket=...` | Kế hoạch (Ngày 12–13) |
