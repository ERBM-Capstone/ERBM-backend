# Expense, receipt และ OCR

Flow: สร้าง Expense เปล่า → แนบไฟล์ → OCR เติมรายการ/ราคา/ยอดร่าง → PC ตรวจแก้ → ยืนยันยอด
Expense เก็บยอดเดียวใน `amount` และสถานะ `DRAFT / CONFIRMED`
คำนวณเงินจริงจาก `amount` เฉพาะรายการ `CONFIRMED`; ยอดของ `DRAFT` ยังไม่มีผลต่อวงเงิน

## API

ทุก route ใช้ `/api/v1` และ Bearer token

| Method | Path | ผลลัพธ์ |
| --- | --- | --- |
| GET / POST | `/advances/{id}/expenses` | รายการแบบแบ่งหน้า / สร้างร่างด้วย `{}` |
| GET / PATCH / DELETE | `/expenses/{id}` | ดู / แก้ร่าง / ลบจริง |
| POST | `/expenses/{id}/confirm` | ยืนยันยอดของร่าง version ที่ตรวจแล้ว |
| POST | `/expenses/{id}/reopen` | ยกเลิกการยืนยัน กลับมาแก้ไข และหยุดนับยอดจนยืนยันใหม่ |
| POST | `/expenses/{id}/receipts` | multipart field `file` หนึ่งไฟล์ |
| POST | `/expenses/{id}/receipt` | alias รองรับ API เดิม |
| GET | `/expenses/{id}/receipts` | metadata ของไฟล์ |
| GET | `/expenses/{id}/receipts/{receipt_id}/file` | ตรวจสิทธิ์ก่อนส่งไฟล์ ไม่มี public URL |
| DELETE | `/expenses/{id}/receipts/{receipt_id}` | ลบไฟล์และ OCR ของไฟล์นั้น |
| POST | `/expenses/{id}/ocr` | `{"receipt_id":1,"force":false}` |
| GET | `/expenses/{id}/ocr-result?receipt_id=1` | ผล attempt ล่าสุด; ระบุ `extraction_id` เพื่ออ่านย้อนหลัง |
| GET | `/advances/{id}/reconciliation` | ยอดยืนยันรวม / คงเหลือ / จำนวนร่างที่ยังต้องยืนยัน |

PC ต้องเป็น PC ปัจจุบันของ Project และผู้สร้าง Advance นั้นจึงเขียนได้ และ Advance ต้อง `ACTIVE`
ADMIN/ACCOUNTING อ่านได้ตามนโยบาย Advance เดิม ส่วน PM/PC ต้องอยู่ใน Project ที่เกี่ยวข้อง
เปลี่ยนหรือลบ PC ไม่ได้จน Advance ทุกสถานะของ Project เป็น `CLOSED` รวม DRAFT/APPROVED
ยังไม่มี API activation/disbursement/Closing ในงานนี้ จึงทดสอบจาก fixture ACTIVE

PATCH รับ `expected_version` และ field ที่ต้องการเปลี่ยน เช่น

```json
{"expected_version":2,"amount":"123.45","merchant_name":"ร้านตัวอย่าง"}
```

ไม่ส่ง field = คงค่า; `null` ล้าง nullable field; `line_items:[]` ล้างรายการ
เงินเป็น decimal string สูงสุด 13 หลักหน้าจุด 2 หลักหลังจุด อนุญาตศูนย์ ห้ามติดลบ
รายการร่างรับ `line_no` เรียง 1..n, `page_no` (ถ้ามี), `description`, `quantity`, `unit_price`, `line_total`
PC ยืนยันด้วย `{"expected_version":3}` โดยใช้ version ล่าสุดจาก PATCH/OCR response
service ตรวจวงเงินและเปลี่ยนสถานะเป็น CONFIRMED พร้อม audit ใน transaction เดียว
ใช้ amount ค่าเดิม ไม่คัดลอกไป field อื่น ยืนยัน version เดิมซ้ำไม่คิดยอดซ้ำ
PATCH และ OCR ที่เติมฟอร์มทำได้เฉพาะ DRAFT เพื่อไม่เปลี่ยนยอดยืนยันโดยอัตโนมัติ
ต้องกด “กลับมาแก้ไข” ผ่าน POST `/expenses/{id}/reopen` พร้อม `expected_version` ก่อน
service เปลี่ยนเป็น DRAFT ล้างข้อมูลการยืนยัน และเพิ่ม version แต่เก็บ amount ค่าเดิมไว้ให้แก้
รายการจะหยุดนับเงินจริงทันทีจน Confirm ใหม่สำเร็จ; Confirm ใหม่ตรวจวงเงินอีกครั้ง
ประวัติยืนยัน/ยกเลิกยืนยันยังอยู่ใน audit; ระหว่าง OCR อ่านต้องรอให้จบก่อนแก้ไข/Confirm/reopen

## ล็อกระหว่าง OCR

ล็อกที่ backend ระดับ Expense ครอบคลุมใบเสร็จทุกไฟล์และทุก server instance
ขณะมี attempt `PROCESSING` คำขอ PATCH, Confirm, reopen และ POST OCR อีกครั้งคืน **409**
แม้เลือกคนละใบเสร็จหรือใช้ `force:true` ก็เริ่มพร้อมกันไม่ได้ ส่วน Expense อื่นยังทำงานได้
backend บันทึก/ตรวจสถานะใน transaction ที่ล็อก Expense เดียวกัน และไม่ถือ SQL lock ระหว่างเรียก OCR

เมื่อสำเร็จจะเติมราคา/รายการ เพิ่ม `version` และปลดล็อกให้ PC รีวิว/แก้ไขแล้ว Confirm
เมื่อล้มเหลว หมดเวลา ยกเลิกคำขอ หรือบันทึกผลไม่สำเร็จ จะเปลี่ยน attempt เป็น FAILED และปลดล็อก
POST OCR ซ้ำเพื่อ retry ได้โดยไม่ต้อง force ถ้าไม่มีผลสำเร็จเดิมที่ cache อยู่
หากมีผลสำเร็จเก่า จะใช้ผลนั้นจนสั่งอ่านใหม่ด้วย `force:true`

หาก process หยุดหรือฐานข้อมูลขัดข้องจนบันทึกจบไม่ได้ attempt จะหมดอายุตาม `AI_REQUEST_TIMEOUT` (ค่าเริ่มต้น 30 วินาที)
GET OCR result จะแสดง FAILED/OCR_TIMEOUT; คำขอแก้ไข/อ่านถัดไปจะเก็บกวาด attempt ที่หมดอายุก่อนทำงาน
ผลที่กลับมาช้าจะถูกทิ้งและไม่เขียนทับราคา หลังอ่านเสร็จหน้าจอต้องใช้ `version` ล่าสุดเพื่อกันฟอร์มเก่าทับข้อมูล

## ไฟล์และการลบ

JPG/JPEG, PNG, PDF ตรวจจากเนื้อไฟล์จริง สูงสุด **10,000,000 bytes (10 MB)/ไฟล์**
สูงสุด 5 ไฟล์ รวม 50,000,000 bytes (50 MB)/Expense; PDF ไม่เกิน 5 หน้าและห้ามเข้ารหัส
ไฟล์เสีย/เข้ารหัส/เกินหน้าคืน 422, MIME ไม่รองรับ 415, เกินขนาด 413, quota เต็ม 409
มีขีดจำกัดเชิงเทคนิคสำหรับรูปใหญ่เกิน 40 ล้านพิกเซลและ PDF ที่ใช้ทรัพยากรอ่านเกินงบ

DELETE ลบ domain rows จริงและลบ OCR ลูกด้วย แต่เก็บ audit การทำรายการ
ลบไฟล์สำเร็จคืน **204**; ถ้า storage ขัดข้องคืน **202** และ `data.cleanup_pending=true`
queue ลบไฟล์อยู่ในฐานข้อมูลและ worker retry หลัง restart ได้
worker ตรวจ orphan ที่อายุเกิน 24 ชั่วโมงกับฐานข้อมูลก่อนลบ ไม่ลบไฟล์ที่ยังมีใบเสร็จอ้างอิง
ลบเฉพาะ receipt ไม่เปลี่ยนยอดร่างหรือยอดที่ PC ยืนยันไว้
เมื่อ Advance ไม่ ACTIVE ห้ามแก้/ยืนยัน/แนบ/ลบ/เริ่มอ่าน OCR; อ่านข้อมูลตามสิทธิ์ได้

## Storage

ตั้งค่าผ่าน environment ที่ `config.Load()` อ่าน:

| Variable | Default / การใช้ |
| --- | --- |
| `RECEIPT_STORAGE_PROVIDER` | `local` หรือ `gcs` |
| `RECEIPT_STORAGE_ROOT` | `var/receipts` — private volume ที่เก็บไฟล์ถาวร |
| `RECEIPT_GCS_BUCKET` | ชื่อ bucket; จำเป็นเมื่อเลือก gcs |

Local ใช้ไฟล์สิทธิ์ 0600 และ root directory 0700 ไม่มี static file route
GCS ใช้ Application Default Credentials, object immutable, generation สำหรับอ่าน/ลบ และ Standard class
ผู้ deploy ต้องเตรียม bucket แบบ private เปิด Public Access Prevention/Uniform Bucket-Level Access
อยู่ region เดียวกับ backend และให้ service identity มีสิทธิ์อ่าน/สร้าง/ลบ/list ตามที่จำเป็น
ต้องตรวจ retention/versioning/soft-delete ของ bucket ให้ตรงกับนโยบายลบจริงด้วย
การตั้ง IAM/region จริงและการทดสอบ cloud ไม่ได้ทำโดยโค้ดหรือการทดสอบ local
ถ้ามีไฟล์ local เดิมหลังเปลี่ยนเป็น GCS ต้องคง volume/root เดิมสำหรับอ่านและ cleanup

## สถานะ OCR integration

มี orchestration, cache, per-file deadline ตาม `AI_REQUEST_TIMEOUT` (ค่าเริ่มต้น 30 วินาที) รวมทุกหน้า, FAILED result และ autofill draft แล้ว
ตัว server ส่ง timeout เดียวกันให้ HTTP provider, service deadline และอายุล็อก PROCESSING; ใช้ Go duration เช่น `2m` โดยค่าที่ผิดรูปแบบ ศูนย์ หรือติดลบจะกลับไปใช้ค่าเริ่มต้น
ผล OCR เติม `amount` และรายการใน DRAFT หลังอ่านเสร็จ โดยล็อกไม่ให้ PC แก้ราคา/Confirm ระหว่างรอ
อ่าน cache ไม่เติมร่างซ้ำ; `force:true` สร้าง attempt ใหม่ เก็บ attempt เดิมไว้
POST ใช้ผล SUCCEEDED เดิมเมื่อไม่มีการอ่านค้าง; FAILED ไม่ถือเป็น cache และเริ่ม retry ได้
GET คืน attempt ล่าสุดรวม FAILED/PROCESSING ได้; error ระหว่างอ่านมี extraction metadata ใน `errors[0]`

**ยังไม่เชื่อม private Typhoon OCR 7B จริง** เพราะผู้ใช้จะส่ง custom API contract เพิ่ม
ตัว server ใช้ `ocr.Unconfigured` และคืน 503 อย่างชัดเจน ไม่ส่งไฟล์ออกไปยัง public OCR
`ocr.Provider` เป็นจุดเสียบ adapter; tests ใช้ provider จำลอง
adapter ต้องใช้ deadline เดียว อ่าน PDF ครบทุกหน้า และระบุ `PagesRead` เมื่อมีหลายหน้า
ยังต้องได้ endpoint/auth method/model ID, request/response sample และวิธีส่ง PDF/แต่ละหน้าก่อนทำ adapter/parser

## Migration และการทดสอบ

คำยืนยันล่าสุดวันที่ 17 พฤศจิกายน 2026 คืนข้อจำกัดเดิม: 10 MB/ไฟล์, 50 MB/Expense
ไม่เกิน 5 ไฟล์ และ PDF ไม่เกิน 5 หน้า
ผู้ใช้ยืนยันว่า migration เพิ่มขนาดเป็น 100 MB รันแล้ว จึงเก็บไฟล์เดิมไว้และเพิ่ม
`20261117000001_restore_receipt_size_limit` เพื่อคืน CHECK ฐานข้อมูลเป็น 10 MB
ต้อง apply migration ใหม่นี้และ restart server; ยังไม่ได้รัน migration กับฐาน application ในงานนี้
หากมีใบเสร็จเกิน 10 MB อยู่ migration จะปฏิเสธโดยไม่ลบข้อมูล ต้องจัดการไฟล์ผ่าน lifecycle ปกติก่อน

เพิ่มคู่ migration 0007 และ 0008 ต่อจาก 0005; ไม่สร้าง placeholder 0006 ที่จองไว้สำหรับ disbursement
ไม่มีการรัน migration กับฐาน application และไม่แก้ 0001–0005
ผู้ใช้ยืนยันว่า 0007/0008 ยังไม่เคยรัน จึงปรับ 0007 เดิมเป็น amount เดียวโดยไม่เพิ่ม migration version ใหม่
รุ่นที่ล็อก OCR ต้องมีทั้ง 0007 และ 0008 ก่อนเปิดให้ PATCH/Confirm/reopen แม้ provider ยังไม่พร้อม
ถ้าถอน 0008 ต้องหยุด paths ที่ตรวจ OCR lock ก่อน; ไม่เปลี่ยนยอด Amount ที่เก็บไว้ใน 0007
rollback 0008 ไม่เปลี่ยนยอดยืนยัน; rollback 0007 ทำลายข้อมูล Expense จึงต้องสำรองและจัดการไฟล์ก่อน

ตัด `DraftVersionAtRequest`, `DirtyCount`, `ExpenseAuditEvent.Reason` และ `OCRLineItem.RawData`
เก็บ `Expense.Version` สำหรับฟอร์มเก่า และใช้ UnconfirmedCount โดยตรงสำหรับจำนวนรายการที่ยังไม่ยืนยัน
field `needs_confirmation_count` เดิมยังคืนจาก UnconfirmedCount เพื่อคง response contract

ทดสอบใหม่อยู่ใน `tests/expense*_test.go` ตามกติกา repository แต่ `/tests/` ยังถูก Git ignore ตามเดิม
ก่อนส่งผ่าน Git ต้องเพิ่มไฟล์เหล่านี้แบบเจาะจงด้วย `git add -f tests/expense*_test.go`

```sh
go test ./tests -run '^TestExpense' -skip '^TestExpensePostgres' -count=1
go vet ./...
go build ./cmd/server ./cmd/migrate
```

PostgreSQL tests ใช้ `ERBM_MIGRATION_TEST_DATABASE_URL` ที่เลือกเป็นฐาน disposable อย่างชัดเจนเท่านั้น
สร้าง schema แยกต่อ test และลบทิ้งเมื่อจบ; ไม่โหลด app config ใช้ `go test ./tests -run '^TestExpensePostgres' -v`
มี concurrency tests สำหรับ overspend/quota, audit rollback, cleanup lease และ migration down/up
ดูตัวอย่างคำขอใน `postman/ERBM-Expense.postman_collection.json` และ Swagger

Expense ใช้ชื่อ line_items และ version; ไม่มี confirmed_version ใช้ status/confirmed_by/confirmed_at สำหรับการยืนยัน ส่วน expected_version ยังคงใช้ตรวจข้อมูลเก่าก่อนแก้ไข
