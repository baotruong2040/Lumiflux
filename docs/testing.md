# Kế hoạch Kiểm thử & Đảm bảo Chất lượng (`testing.md`)

---

## 1. Chiến lược & Công cụ Kiểm thử

- **Backend (Go):**
  - `go test -race ./...`: Kiểm tra unit test, integration test và phát hiện data race trong các goroutine của WebSocket Hub.
  - `go vet ./...`: Kiểm tra lỗi tĩnh.
  - `go.uber.org/goleak`: Đảm bảo không rò rỉ goroutine sau khi đóng kết nối WebSocket hoặc dừng Hub.
  - **CI:** `.github/workflows/ci.yml` chạy `go vet ./...` và `go test -race ./...` mỗi lần push.
  - **Integration test:** repository và `MessageService` chạy trên PostgreSQL/Redis thật bằng `testcontainers-go` (hoặc compose dev).
  - **Hub:** bốn tình huống ở mục 2.6, `BenchmarkFanout` với 1.000 và 10.000 client giả (một goroutine drain `send`), và `net/http/pprof` để xem số goroutine.
- **Frontend (React + TypeScript):**
  - `npm run build` (`tsc` + `vite build`): Đảm bảo không có lỗi kiểu dữ liệu TypeScript.
  - Kiểm thử tích hợp thủ công song song trên **Trình duyệt (Chrome/Firefox)** và **Cửa sổ Desktop (Tauri 2)**.

---

## 2. Ma trận Ca kiểm thử Chức năng (Test Cases)

### 2.1. Authentication & Token Rotation
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-AUTH-01` | Đăng ký tài khoản mới hợp lệ | Trả về `201 Created`, mật khẩu trong DB được băm `bcrypt`. | `[x]` |
| `TC-AUTH-02` | Đăng ký trùng `username` hoặc `email` (kể cả khác chữ hoa/thường) | Trả về `409 Conflict` nhờ cột `CITEXT`. | `[x]` |
| `TC-AUTH-03` | Đăng nhập đúng thông tin | Trả về `accessToken` (15p) và `refreshToken` (7 ngày), lưu hash vào Redis. | `[x]` |
| `TC-AUTH-04` | Gọi `POST /auth/refresh` bằng refresh token hợp lệ | Cấp cặp token mới; dùng lại refresh token cũ ngay sau đó bị từ chối `401`. | `[x]` |
| `TC-AUTH-05` | Access token hết hạn khi client đang mở | `httpClient` tự động gọi `/auth/refresh` và thực thi lại request gốc thành công. | `[x]` |
| `TC-AUTH-06` | Đăng nhập sai 11 lần trong 1 phút từ một IP | Lần thứ 11 trả `429 RATE_LIMITED`. | `[x]` |

### 2.2. Guild, Membership, Invite & Channel (RBAC)
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-GLD-01` | Tạo server mới | Tạo đồng thời guild, kênh `#general` và gán người tạo làm `owner`. | `[x]` |
| `TC-GLD-02` | `Owner` gọi `DELETE /guilds/:id/members/@me` | Từ chối với mã `403 FORBIDDEN`. | `[x]` |
| `TC-GLD-03` | `Member` tạo mã mời và tài khoản khác nhập mã mời | Tham gia server thành công; danh sách thành viên tự cập nhật qua `GUILD_MEMBER_ADD`. | `[x]` |
| `TC-GLD-04` | Nhập mã mời đã hết hạn hoặc hết `max_uses` | Từ chối `400` với `INVITE_EXPIRED` hoặc `INVITE_EXHAUSTED`. | `[x]` |
| `TC-GLD-05` | Thành viên rời server khi tab đang mở (ADR-007) | Tab đó không còn nhận `MESSAGE_CREATE` của server cũ; những người còn lại nhận `GUILD_MEMBER_REMOVE`. | `[x]` |
| `TC-GLD-06` | Gán `admin` bằng SQL, rồi `POST` kênh và xóa tin của `member` | Thành công; `member` thường vẫn bị `403`. | `[x]` |
| `TC-GLD-07` | `Owner` xóa server khi thành viên khác đang online | Client kia nhận `GUILD_DELETE`, server biến mất khỏi thanh bên, không cần F5. | `[x]` |
| `TC-CHN-01` | `Member` thường gọi `POST` hoặc `DELETE` channel | Từ chối với mã `403 FORBIDDEN`. | `[x]` |
| `TC-CHN-02` | `Owner`/`Admin` tạo 2 kênh trùng tên trong cùng server | Từ chối với mã `409 Conflict`. | `[x]` |

### 2.3. Message & Cursor Pagination
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-MSG-01` | Gửi tin nhắn rỗng (`"   "`) hoặc dài > 2.000 ký tự | Từ chối với mã `400 Bad Request`. | `[x]` |
| `TC-MSG-02` | Tải lịch sử với 1.000 tin nhắn seed sẵn | Cuộn lên trên tải từng trang 50 tin qua `?before=<id>`, không trùng và không sót tin. | `[x]` |
| `TC-MSG-03` | Tác giả sửa tin nhắn; `Admin` xóa tin nhắn của `Member` | Cập nhật `edited_at` / `deleted_at` và phát `MESSAGE_UPDATE` / `MESSAGE_DELETE` tới các client. | `[x]` |
| `TC-MSG-04` | Một user gửi 31 tin trong 10 giây | Tin thứ 31 nhận `429 RATE_LIMITED`; 30 tin trước đó không bị ảnh hưởng. | `[x]` |

### 2.4. WebSocket Realtime, Resync & Presence
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-WS-01` | Dùng lại vé `ticket` đã kết nối trước đó | Bị từ chối `401 Unauthorized` (do Redis `GETDEL`). | `[x]` |
| `TC-WS-02` | Client A gửi liên tiếp 20 tin nhắn | Client A thay thế tin tạm bằng tin thật qua `nonce` (không hiện 40 tin); Client B nhận đủ 20 tin đúng thứ tự. | `[x]` |
| `TC-WS-03` | Ngắt mạng Client B 10 giây, Client A gửi 5 tin, sau đó nối lại mạng Client B | Client B tự động reconnect, lấy ticket mới, kích hoạt `RESYNC` và hiển thị đủ 5 tin bị lỡ. | `[x]` *(xác minh unit test `GatewayClient` `4003` -> reconnect -> `READY` #2 `RESYNC` invalidate `['messages']` & `['members']` + live `:8080`/`:8081` khôi phục 5 tin bị lỡ)* |
| `TC-WS-04` | Client A gõ phím trong ô chat | Client B thấy chỉ báo "... đang nhập" và tự tắt sau ~3 giây. | `[x]` *(backend `SET NX EX 3` + `USER_TYPING` integration pass; FE `typingStore` auto-expiry unit pass)* |
| `TC-WS-05` | Client B đóng ứng dụng / mất kết nối quá 60 giây | Danh sách của Client A chuyển B sang `offline` trong vài giây (Hub phát khi kết nối cuối đóng); nếu server chết đột ngột, `GET members` trả `offline` sau $\le$ 60 giây. | `[x]` *(integration test đóng tab cuối -> `PRESENCE_UPDATE` `offline`, xóa key Redis `presence:{userID}` TTL 60s, `GET members` `offline`)* |
| `TC-WS-06` | Chạy 2 instance backend (cổng 8080 và 8081); Client A nối instance 1, Client B nối instance 2; A gửi 20 tin | B nhận đủ 20 tin đúng thứ tự (chứng minh Redis Pub/Sub, ADR-003). | `[x]` |
| `TC-WS-07` | Một tài khoản mở hai tab; thành viên khác theo dõi trạng thái | Cả hai tab nhận tin; `online` giữ cho tới khi đóng tab cuối rồi mới `offline`. | `[x]` *(unit test Hub + integration test 2 tab + Chrome E2E)* |

### 2.5. Giao diện & Đa nền tảng (Web + Tauri 2)
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-UI-01` | Gõ tiếng Việt có dấu (Telex và VNI) trong ô nhập tin | Không mất dấu, không gửi nhầm tin khi đang gõ tổ hợp phím. | `[x]` *(đã kiểm gửi/hiển thị đủ dấu tiếng Việt qua Web E2E + cài guard `isComposing`/`keyCode === 229`; kiểm tra bộ gõ IME OS thực tế nếu cần thêm bằng tay)* |
| `TC-UI-02` | Gửi chuỗi 2.000 ký tự không dấu cách hoặc URL rất dài | Tự động xuống dòng (`overflow-wrap: anywhere`), không làm vỡ layout khung chat. | `[x]` |
| `TC-UI-03` | Chạy song song Web (`localhost:5173`) và Tauri ở cả hai chế độ: `tauri dev` (origin `http://localhost:5173`) và bản `tauri build` (origin `tauri://localhost`) | Không gặp lỗi CORS ở cả REST lẫn WebSocket `CheckOrigin`. | `[x]` *(integration test CORS & WS `CheckOrigin` đủ 5 origin Web/Tauri + chặn origin lạ `403`, kết hợp `tauri dev` & `tauri build`)* |
| `TC-UI-04` | Đo độ tương phản chữ/nền của dark theme bằng Chrome DevTools | Đạt tối thiểu 4,5:1 (WCAG AA). | `[x]` *(kiểm toán 16 cặp token trong `tokens.css`: tất cả $\ge$ 4,5:1, thấp nhất `4,57:1`)* |

### 2.6. Hub & Client (`go test -race`)
| Mã TC | Kịch bản kiểm thử | Kết quả kỳ vọng | Trạng thái |
| :--- | :--- | :--- | :--- |
| `TC-HUB-01` | Một client không đọc, hàng đợi `send` (256) đầy | Hub ngắt client đó bằng mã `4002`; các client khác vẫn nhận đủ frame. | `[x]` |
| `TC-HUB-02` | Client đóng đột ngột giữa lúc Hub đang fan-out | Hub gỡ client khỏi cả 3 map; `goleak.VerifyNone` sạch. | `[x]` |
| `TC-HUB-03` | `Unregister` gọi hai lần cho một client | Không panic, không lỗi. | `[x]` |
| `TC-HUB-04` | Hủy `ctx` của `Hub.Run` | Mọi kết nối đóng bằng `1001`, `Run` thoát, không rò rỉ goroutine. | `[x]` |
| `TC-HUB-05` | `BenchmarkFanout` với 1.000 và 10.000 client giả, rồi ngắt hết client | Ghi `ns/op`, `B/op`; số goroutine về mức nền (pprof). (`clients_1000`: `124688 ns/op`, `6 B/op`, `0 allocs/op`; `clients_10000`: `1867519 ns/op`, `2 B/op`, `0 allocs/op`) | `[x]` |

---

## 3. Bảng Ghi nhận Số liệu Độ trễ (Phục vụ Báo cáo)

| Kịch bản đo | Môi trường / Mạng | Số client | p50 (ms) | p95 (ms) | p99 (ms) | Ghi chú |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| Echo RTT (REST POST -> WS `MESSAGE_CREATE`) | Localhost (`127.0.0.1:8080` $\leftrightarrow$ `:8081`) | 2 | 2.83 | 4.39 | 5.06 | Mục tiêu: p50 $\le$ 100, p95 $\le$ 300; đo trên 20 tin nhắn qua Redis Pub/Sub |
| Multi-client Broadcast (`cmd/loadtest`) | Localhost (`127.0.0.1:8080`) | 2 | 4.36 | 5.02 | 6.60 | `40/40` frame (`20` tin, `min=2.96`, `max=6.60`) |
| Multi-client Broadcast | Localhost (`127.0.0.1:8080`) | 10 | 4.69 | 5.82 | 9.75 | `cmd/loadtest`: `200/200` frame (`min=3.78`, `max=9.75`) |
| Multi-client Broadcast | Localhost (`127.0.0.1:8080`) | 100 | 5.07 | 6.24 | 10.92 | `cmd/loadtest`: `2.000/2.000` frame (`min=3.75`, `max=10.92`) |
| Multi-client Broadcast | Localhost (`127.0.0.1:8080`) | 500 | 15.66 | 24.24 | 30.69 | `cmd/loadtest`: `10.000/10.000` frame (`min=8.13`, `max=32.72`) |
| Ngắt mạng rồi nối lại (`RESYNC`) | Localhost (`:8080` $\leftrightarrow$ `:8081`) | 2 | — | — | — | Khôi phục đủ 5/5 tin bị lỡ; `goleak` & pprof goroutine về nền |