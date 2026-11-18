# Expense implementation checks

## 17 พฤศจิกายน 2026 — คืนค่า 10 MB ต่อไฟล์ / 50 MB ต่อ Expense

ผู้ใช้ขอคืนข้อจำกัดเดิมและยืนยันว่า migration เพิ่ม 100 MB รันแล้ว จึงคืน
storage/HTTP/service quota/OCR เป็น 10,000,000 bytes ต่อไฟล์ รวม 50,000,000 bytes
สูงสุด 5 ไฟล์ PDF ยังคงไม่เกิน 5 หน้า อัปเดต Swagger, Postman และ boundary tests ให้ตรงกัน

เก็บ migration `20261117000000_increase_receipt_size_limit` ที่ใช้งานแล้วไว้ตามเดิม
เพิ่มคู่ `20261117000001_restore_receipt_size_limit` เพื่อคืน CHECK เป็น 10 MB
ต้อง apply migration ใหม่นี้และ restart server จึงคืนทั้ง schema/runtime ครบ
หากมีใบเสร็จเกิน 10 MB อยู่ การ apply จะปฏิเสธโดยไม่แก้หรือลบข้อมูล
ไม่ได้รัน migration กับฐาน application ในรอบนี้

| คำสั่งที่รัน | ผล |
| --- | --- |
| `go test -json ./...` | tests ใหม่รับ 10 MB/50 MB รวม, ปฏิเสธเกิน 1 byte/ไฟล์ที่ 6 และตรวจเพดาน OCR 10 MB ผ่าน; เหลือ 2 failures เดิมเรื่อง activation logging ตามรายชื่อด้านล่าง |
| `go vet ./...` | ผ่าน |
| `go build ./cmd/server ./cmd/migrate` | ผ่าน |
| `swag init -g cmd/server/main.go -o docs` | ผ่าน; warning เดิมว่า root ไม่มี Go files |
| `git diff --check` | ผ่าน |

ตรวจ Swagger/Postman และ migration up/down ตรงกับข้อจำกัดที่คืนแล้ว
`TestExpensePostgresRestoreReceiptSizeMigration` ข้ามเพราะไม่ตั้งฐาน disposable
และไม่มีการขอสร้างฐานทดสอบซ้ำหลังผู้ใช้ไม่อนุญาตในรอบก่อน
ลบ diagnostic ชั่วคราวที่อ่าน PDF ภายนอกออกแล้ว; tests ไม่ผูกกับไฟล์ของผู้ใช้

## ประวัติรอบก่อน — 17 พฤศจิกายน 2026: 100 MB ต่อไฟล์ / 500 MB ต่อ Expense (ยกเลิกแล้ว)

ผู้ใช้ยืนยันขยายขนาดไฟล์ใบเสร็จเป็น 100,000,000 bytes ต่อไฟล์ รวมสูงสุด
500,000,000 bytes ต่อ Expense และยังรับไม่เกิน 5 ไฟล์ อัปเดต storage validation,
HTTP upload, OCR HTTP adapter, service quota, error messages, Swagger และ Postman
ตัว multipart parser คง memory budget เดิมและ spill ไฟล์ใหญ่ลง temporary storage

เพิ่มคู่ migration `20261117000000_increase_receipt_size_limit` เพื่อขยาย
CHECK ของ expense_documents.size_bytes โดยไม่แก้ 0007 เดิม; ต้อง apply และ restart server
ก่อนใช้งานขนาดใหม่ Down จะไม่สำเร็จหากยังมีไฟล์เกิน 10 MB และไม่ลบข้อมูลเพื่อ downgrade

| คำสั่งที่รัน | ผล |
| --- | --- |
| `go test ./tests -run '^TestExpense' -count=1` | ผ่าน; PostgreSQL cases ข้ามเพราะไม่กำหนดฐานทดสอบ |
| `go test -json ./...` | tests ใหม่รับ 100 MB/500 MB รวม, ปฏิเสธเกิน 1 byte/ไฟล์ที่ 6 และส่งต่อ OCR 100 MB ผ่าน; เหลือ 2 failures เดิมเรื่อง activation logging ตามรายชื่อด้านล่าง |
| `go vet ./...` | ผ่าน |
| `go build ./cmd/server ./cmd/migrate` | ผ่าน |
| `swag init -g cmd/server/main.go -o docs` | ผ่าน; warning เดิมว่า root ไม่มี Go files |
| `git diff --check` | ผ่าน |

ตรวจ Swagger ทั้ง /receipt และ /receipts, Postman upload และค่า CHECK ในคู่ migration ตรงกัน
ตรวจ gofmt แล้วไม่พบไฟล์ Go ที่ยังไม่ได้จัดรูปแบบ
รอบแรก sandbox บล็อก Go build cache จึง rerun tests/checks นอก sandbox
ไม่ได้รัน migration หรือแก้ข้อมูลในฐาน application
การสร้างฐาน PostgreSQL ชั่วคราวเพื่อทดสอบ migration ใหม่ไม่ได้รับอนุญาต จึงไม่ได้รัน
`TestExpensePostgresReceiptSizeMigrationAndSafeRollback`; ยังไม่ยืนยัน up/down กับ PostgreSQL จริงในรอบนี้

## ผลตรวจเดิม — 15 พฤศจิกายน 2026

## ผลตรวจ

| คำสั่งที่รัน | ผล |
| --- | --- |
| `go test ./tests -run '^TestExpense' -skip '^TestExpensePostgres' -count=1 -v` | ผ่าน 26 tests หลักและ subtests; ไม่เรียก PostgreSQL |
| `env -u ERBM_MIGRATION_TEST_DATABASE_URL go test ./tests -run '^TestExpensePostgres' -v` | ข้าม PostgreSQL 4 tests ตามเงื่อนไข |
| `env -u ERBM_MIGRATION_TEST_DATABASE_URL go test ./...` | ไม่ผ่าน 2 tests เดิมเรื่อง activation mail logging; ไม่พบ failure อื่น |
| `go test -race ./tests -run '^TestExpense' -skip '^TestExpensePostgres' -count=1` | ผ่าน ไม่พบ data race |
| `go vet ./...` | ผ่าน |
| `go build ./cmd/server ./cmd/migrate` | ผ่าน |
| `git diff --check` | ผ่าน |
| `go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs` | สร้าง Swagger สำเร็จ; warning ว่า root ไม่มี Go files ไม่ขัดขวางการสร้าง |

ตรวจ generated Swagger มี amount field เดียว ไม่มี draft_amount/confirmed_amount ใน DTO, มี reopen route, auth/error responses และ DELETE 204/202
ตรวจ Postman เป็น JSON ที่อ่านได้ มี 15 ตัวอย่างคำขอ และตรวจ diagram ที่ render แล้ว
dependencies ใหม่ใช้ Go 1.25 ได้ตาม `go.mod`

2 tests ที่ไม่ผ่านเหมือน baseline ก่อนเพิ่ม Expense:

- `TestActivationDeliveryFailureIsLoggedWithoutInvitation`
- `TestActivationDeliverySuccessIsLoggedWithoutInvitation`

ไม่แก้พฤติกรรมหรือตัด tests ของ activation ในงานนี้

## สิ่งที่ยังไม่ตรวจจริง

- PostgreSQL: มี tests สำหรับ OCR สอง service/สองใบเสร็จพร้อมกัน, ปลดล็อกหลัง success/failure/timeout, simultaneous confirmation/quota, audit rollback, cleanup lease และ migration down/up แต่ข้ามเพราะไม่ได้เลือกฐาน disposable สำหรับรอบตรวจนี้
- ไม่ได้รัน migrations กับฐาน application ไม่เริ่ม containers และไม่แก้ migration 0001–0005
- ผู้ใช้ยืนยันว่า 0007/0008 ยังไม่เคยรัน จึงปรับ SQL 0007/0008 เดิมตาม model ล่าสุด โดยไม่สร้าง version เพิ่ม
- GCS: ยังไม่ทดสอบกับ bucket/IAM/region จริง
- OCR: ทดสอบกับ provider จำลอง; private Typhoon OCR 7B HTTP adapter/parser รอ custom API contract จากผู้ใช้ และ server ปัจจุบันคืน 503 เมื่อเริ่ม OCR จริง
- ยังไม่มี activation/disbursement/Closing API ในขอบเขตนี้ จึงใช้ ACTIVE fixtures และต้องเชื่อม shared reconciliation/lock contract เมื่องาน Closing ถูก implement

## ไฟล์ทดสอบ

สร้าง `tests/expense_test.go`, `expense_files_test.go`, `expense_handler_test.go`,
`expense_pc_transfer_test.go`, `expense_postgres_test.go`, `expense_ocr_lock_test.go`,
`expense_ocr_cancel_test.go`, `expense_ocr_lock_postgres_test.go`

ทั้ง 8 ไฟล์ยังถูก ignore ด้วย `/tests/` ตามค่าเดิม ไม่เปลี่ยน ignore ของ tests หรือ stage ไฟล์ให้ผู้ใช้
ถ้าส่งผ่าน Git ต้องเพิ่ม 8 ไฟล์นี้แบบเจาะจง (`git add -f tests/expense*_test.go`)

อ่าน [Expense API](expense-api.md) สำหรับ routes, payload, storage settings และสิ่งที่ต้องได้จาก private OCR server

## การปรับเป็น amount เดียว

Expense มี amount เดียวและ DRAFT/CONFIRMED; เพิ่ม POST /expenses/{id}/reopen ตามคำตอบผู้ใช้: ยกเลิกการยืนยัน/หยุดนับยอดทันที แต่เก็บราคาให้แก้และ Confirm ใหม่ได้ ตรวจยอดรวมเฉพาะ CONFIRMED; tests เพิ่มการ reopen/audit rollback/stale version/reconfirmation budget, response ไม่มี draft_amount/confirmed_amount และ OCR ไม่ทับหลัง confirm/reopen

## ลด field ของ Expense

เปลี่ยน DraftLineItems/draft_line_items เป็น LineItems/line_items และ DraftVersion/draft_version เป็น Version/version; ตัด ConfirmedVersion/confirmed_version ออก เก็บ confirmed_by/confirmed_at และ expected_version เพื่อรักษาประวัติและกันข้อมูลเก่าเขียนทับ ปรับ DTO/repository/service/SQL 0007/tests/Swagger/Postman ให้ตรงกัน โดยไม่รัน migration

## ล็อกระดับ Expense ระหว่าง OCR

ใช้ PROCESSING ของ attempt เป็นล็อกที่ตรวจภายใต้ transaction ของ Expense เดียวกัน บล็อก PATCH/Confirm/reopen/OCR ซ้ำทุกใบเสร็จด้วย 409 ไม่ถือ SQL lock ระหว่าง provider ทำงาน ตัด DraftVersionAtRequest, DirtyCount, ExpenseAuditEvent.Reason และ OCRLineItem.RawData โดยเก็บ Expense.Version และ raw_result

ทดสอบปลดล็อกเมื่อสำเร็จ/ล้มเหลว/timeout/cancel/provider panic/storage failure/บันทึกผลล้มเหลว, retry ได้โดยไม่ force หลัง failed, recovery หลัง process หยุด, ทิ้งผล late provider และฟอร์มเก่าถูกปฏิเสธด้วย Version

หาก DB ขัดข้องจนบันทึกจบไม่ได้ ล็อกหมดอายุใน 30 วินาที: GET แสดง FAILED/OCR_TIMEOUT และการแก้ไข/อ่านครั้งถัดไปเก็บกวาด attempt ก่อนทำงาน; สิ่งนี้ยังต้องพิสูจน์กับฐาน PostgreSQL และ private server จริงใน integration environment
