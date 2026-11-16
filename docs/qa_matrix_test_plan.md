# Kế hoạch Kiểm thử & Đảm bảo Chất lượng (`testing.md`)

---

## 1. Chiến lược & Công cụ Kiểm thử

- **Backend (Go):**
  - `go test -race ./...`: Kiểm tra unit test, integration test và phát hiện data race trong các goroutine của WebSocket Hub.
  - `go vet ./...`: Kiểm tra lỗi tĩnh.
  - `go.uber.org/goleak`: Đảm bảo không rò rỉ goroutine sau khi đóng kết nối WebSocket hoặc dừng Hub.
- **Frontend (React + TypeScript):**
  - `npm run build` (`tsc` + `vite build`): Đảm bảo không có lỗi kiểu dữ liệu TypeScript.
  - Kiểm thử tích hợp thủ công song song trên **Trình duyệt (Chrome/Firefox)** và **Cửa sổ Desktop (Tauri 2)**.

---

## 2. Ma trận Ca kiểm thử Chức năng (Test Cases)

### 2.1. Authentication & Token Rotation
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-AUTH-01` | Đăng ký tài khoản mới hợp lệ | Trả về `201 Created`, mật khẩu trong DB được băm `bcrypt`. | `[ ]` |
| `TC-AUTH-02` | Đăng ký trùng `username` hoặc `email` (kể cả khác chữ hoa/thường) | Trả về `409 Conflict` nhờ cột `CITEXT`. | `[ ]` |
| `TC-AUTH-03` | Đăng nhập đúng thông tin | Trả về `accessToken` (15p) và `refreshToken` (7 ngày), lưu hash vào Redis. | `[ ]` |
| `TC-AUTH-04` | Gọi `POST /auth/refresh` bằng refresh token hợp lệ | Cấp cặp token mới; dùng lại refresh token cũ ngay sau đó bị từ chối `401`. | `[ ]` |
| `TC-AUTH-05` | Access token hết hạn khi client đang mở | `httpClient` tự động gọi `/auth/refresh` và thực thi lại request gốc thành công. | `[ ]` |

### 2.2. Guild, Membership, Invite & Channel (RBAC)
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-GLD-01` | Tạo server mới | Tạo đồng thời guild, kênh `#general` và gán người tạo làm `owner`. | `[ ]` |
| `TC-GLD-02` | `Owner` gọi `DELETE /guilds/:id/members/@me` | Từ chối với mã `403 FORBIDDEN`. | `[ ]` |
| `TC-GLD-03` | `Member` tạo mã mời và tài khoản khác nhập mã mời | Tham gia server thành công; danh sách thành viên tự cập nhật qua `GUILD_MEMBER_ADD`. | `[ ]` |
| `TC-GLD-04` | Nhập mã mời đã hết hạn hoặc hết `max_uses` | Từ chối tham gia với mã lỗi phù hợp. | `[ ]` |
| `TC-CHN-01` | `Member` thường gọi `POST` hoặc `DELETE` channel | Từ chối với mã `403 FORBIDDEN`. | `[ ]` |
| `TC-CHN-02` | `Owner`/`Admin` tạo 2 kênh trùng tên trong cùng server | Từ chối với mã `409 Conflict`. | `[ ]` |

### 2.3. Message & Cursor Pagination
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-MSG-01` | Gửi tin nhắn rỗng (`"   "`) hoặc dài > 2.000 ký tự | Từ chối với mã `400 Bad Request`. | `[ ]` |
| `TC-MSG-02` | Tải lịch sử với 1.000 tin nhắn seed sẵn | Cuộn lên trên tải từng trang 50 tin qua `?before=<id>`, không trùng và không sót tin. | `[ ]` |
| `TC-MSG-03` | Tác giả sửa tin nhắn; `Admin` xóa tin nhắn của `Member` | Cập nhật `edited_at` / `deleted_at` và phát `MESSAGE_UPDATE` / `MESSAGE_DELETE` tới các client. | `[ ]` |

### 2.4. WebSocket Realtime, Resync & Presence
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-WS-01` | Dùng lại vé `ticket` đã kết nối trước đó | Bị từ chối `401 Unauthorized` (do Redis `GETDEL`). | `[ ]` |
| `TC-WS-02` | Client A gửi liên tiếp 20 tin nhắn | Client A thay thế tin tạm bằng tin thật qua `nonce` (không hiện 40 tin); Client B nhận đủ 20 tin đúng thứ tự. | `[ ]` |
| `TC-WS-03` | Ngắt mạng Client B 10 giây, Client A gửi 5 tin, sau đó nối lại mạng Client B | Client B tự động reconnect, lấy ticket mới, kích hoạt `RESYNC` và hiển thị đủ 5 tin bị lỡ. | `[ ]` |
| `TC-WS-04` | Client A gõ phím trong ô chat | Client B thấy chỉ báo "... đang nhập" và tự tắt sau ~3 giây. | `[ ]` |
| `TC-WS-05` | Client B đóng ứng dụng / mất kết nối quá 60 giây | Trạng thái của Client B chuyển sang `offline` trên danh sách thành viên của Client A. | `[ ]` |

### 2.5. Giao diện & Đa nền tảng (Web + Tauri 2)
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-UI-01` | Gõ tiếng Việt có dấu (Telex và VNI) trong ô nhập tin | Không mất dấu, không gửi nhầm tin khi đang gõ tổ hợp phím. | `[ ]` |
| `TC-UI-02` | Gửi chuỗi 2.000 ký tự không dấu cách hoặc URL rất dài | Tự động xuống dòng (`overflow-wrap: anywhere`), không làm vỡ layout khung chat. | `[ ]` |
| `TC-UI-03` | Chạy song song Web (`localhost:5173`) và Tauri (`tauri://localhost`) | Không gặp lỗi CORS ở cả REST lẫn WebSocket `CheckOrigin`. | `[ ]` |

---

## 3. Bảng Ghi nhận Số liệu Độ trễ (Phục vụ Báo cáo)

| Kịch bản đo | Môi trường / Mạng | Số client | p50 (ms) | p95 (ms) | p99 (ms) | Ghi chú |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| Echo RTT (REST POST -> WS `MESSAGE_CREATE`) | Localhost / Wi-Fi | 2 | | | | Mục tiêu: p50 $\le$ 100ms |
| Multi-client Broadcast | Localhost / Wi-Fi | 10 | | | | |
| Multi-client Broadcast | Localhost / Wi-Fi | 100 | | | | |