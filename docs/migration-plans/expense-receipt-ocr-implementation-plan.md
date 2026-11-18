# ERBM — Expense, Receipt และ OCR implementation plan

วันที่ปรับปรุง: 15 พฤศจิกายน 2026 · อ้างอิง blueprint `ERBM-DB-0.14`

เอกสารนี้เป็นแผน implementation ที่ผู้ใช้อนุญาตให้เริ่มทำแล้ว ยังไม่ใช่ SQL/API ที่ deploy แล้ว
สถานะล่าสุดและ contract ที่ implement อยู่ใน [Expense API](../expense-api.md)
schema ใช้ [แผนฐานข้อมูลกลาง](erbm-database-plan.md) ร่วมกัน ไม่สร้าง dictionary อีกชุด

## 1. ผลลัพธ์และข้อกำหนดที่ยืนยันแล้ว

```text
แนบบิล -> OCR อ่านรายการ ราคา และยอดรวม แล้วกรอกฟอร์มอัตโนมัติ
       -> PC ตรวจและแก้ข้อมูล -> PC กดยืนยัน -> ใช้ amount ของ CONFIRMED คำนวณเงินจริง
```

- PC จัดการรายจ่ายของ Fund ตนเองได้ขณะ Advance เป็น `ACTIVE` เท่านั้น
- ก่อนเปลี่ยน/ลบ PC ต้องให้ Advance เดิมทุกสถานะเป็น CLOSED รวม DRAFT/APPROVED ตามคำยืนยันล่าสุด
- ห้ามยืนยันยอดเกินเงินคงเหลือ หากเหลือ 10,000 แต่ต้องใช้ 12,000 ต้องปิด Fund เดิม
  คืนเงินคงเหลือ แล้วขอ Fund ใหม่ ไม่เพิ่มยอด Fund หรือยืมข้าม Fund อัตโนมัติ
- ยอดเดียว `amount`; `null` คือยังไม่มีราคา, `"0.00"` คือราคาศูนย์ที่ยืนยันได้
  `status=DRAFT` ยังไม่นับเงินจริง; `CONFIRMED` จึงนับ ยอดห้ามติดลบ ใช้เงินบาทและ exact decimal
- หลังยืนยัน PC กด reopen เพื่อกลับมาแก้ได้ โดยหยุดนับยอดทันทีจน Confirm ใหม่ ตามคำตอบล่าสุด
- OCR กรอกทั้งรายการและราคาให้อัตโนมัติ PC ไม่ต้องเริ่มกรอกข้อมูลจากศูนย์
  OCR/PATCH แก้เฉพาะ DRAFT; รายการที่ยืนยันต้องกด reopen ก่อนแก้
- Expense/receipt ลบจริง เมื่อ `CLOSED` แล้วห้ามแก้ ลบ แนบ ยืนยัน หรือเริ่ม OCR ใหม่
- รับ JPG/JPEG, PNG, PDF; 10 MB ต่อไฟล์; สูงสุด 5 ไฟล์/50 MB ต่อ Expense อัปโหลดทีละไฟล์ (ผู้ใช้ยืนยันคืนขนาดเดิม 17 พฤศจิกายน 2026)
- PDF ไม่เกิน 5 หน้าและไม่ติดรหัสผ่าน
- Local private storage ตอนพัฒนา; GCS Standard private region เดียวกับ backend ตอน deploy
- OCR ผ่าน API ของเซิร์ฟเวอร์ส่วนตัวที่ใช้ Typhoon OCR 7B ตามคำชี้แจงล่าสุดของผู้ใช้
  ไม่อ้าง model alias ของ public Typhoon API ว่าเป็นเซิร์ฟเวอร์นี้
- synchronous ทีละไฟล์ deadline 30 วินาทีรวมทุกหน้า เก็บผลเดิมจนผู้ใช้สั่งอ่านใหม่

## 2. Baseline ก่อนเริ่ม implementation

- Go 1.25.0, Gin, PostgreSQL, GORM; ใช้ `router -> handler -> service -> repository`
- มี migrations 0001–0005; Advance มี create/list/detail/edit/submit/PM review/history
- มี `uq_advances_one_active_per_project` ใน 0005 แล้ว ใช้ต่อโดยไม่สร้างซ้ำ
- ยังไม่มี Expense, storage/OCR adapters, disbursement/activation และ Closing APIs
  การทดสอบ Expense เริ่มจาก fixture `ACTIVE` ได้ แต่ production flow ต้องรอ activation
- Project PATCH ปัจจุบันเปลี่ยน/ลบ PC ได้โดยยังไม่มี guard สำหรับ Fund ที่ต้องเคลียร์
- เงิน Advance ใช้ decimal string อยู่แล้ว แต่ยังไม่มี shared arithmetic สำหรับรวมรายจ่าย
- `/tests/` ถูก ignore อยู่ ต้องกำหนดวิธีส่งมอบ tests โดยไม่อ้างว่าเพิ่มใน Git แล้ว
- ไม่ได้ตรวจฐานจริง ข้อมูลว่า 0001/0002/0004 เคย apply เป็นคำแจ้งก่อนหน้าเท่านั้น
  ห้ามอนุมานว่า 0003/0005 ยัง unapplied ในทุก environment หรือแก้ migration เดิมจากแผนนี้

แหล่งหลัก: AGENTS.md, skills advance-workflow/backend-resource/migration-plan,
blueprint กลาง, model/DTO/repository/service/handler/router, config, migrations,
runner/Makefile, API docs, Swagger, Postman และ root-level tests ที่อ่านในแชตนี้

## 3. แยกข้อมูล OCR, ร่าง และยอดยืนยัน

| ชุดข้อมูล | ที่เก็บตาม blueprint | ผู้เขียน | นำไปคำนวณเงินหรือไม่ |
| --- | --- | --- | --- |
| ผล OCR ต้นฉบับ | ocr_extractions + ocr_line_items | OCR adapter/service | ไม่ใช้ |
| ฟอร์มที่ OCR เติม/PC แก้ | expenses.amount, line_items และรายละเอียด ขณะ status=DRAFT | OCR service / PC PATCH | ไม่ใช้ |
| ยอดเดิมหลัง PC ยืนยัน | expenses.amount ค่าเดิม เมื่อ status=CONFIRMED + confirmed_by/at | confirmation service | ใช้ |
| ร่างที่ PC เคยยืนยัน | CONFIRM audit snapshot | service | หลักฐานย้อนหลัง ไม่ใช่แหล่งยอดปัจจุบัน |

ข้อเสนอให้เก็บรายการร่างที่แก้ได้ใน `line_items JSONB` เป็น array ของ DTO ที่มี schema
และ validation ชัดเจน ส่วน ocr_line_items เก็บผลอ่านต้นฉบับ ไม่ให้ PATCH แก้ผลต้นฉบับ
ยังไม่เพิ่มตารางรายการทางบัญชีหรือ tax engine โดยไม่มี requirement

`version` เพิ่มเมื่อแก้ข้อมูลหรือ reopen เพื่อกันข้อมูลเก่าเขียนทับ; สถานะยืนยันใช้ status พร้อม confirmed_by/at
OCR/PATCH เขียนได้เฉพาะ DRAFT และไม่ยืนยันเอง
หลัง Confirm ต้องกด reopen อย่างชัดเจนก่อน: เปลี่ยนเป็น DRAFT ล้าง confirmation เพิ่ม version
amount ค่าเดิมยังอยู่ให้แก้ แต่หยุดนับเงินจริงทันทีจนกว่าจะ Confirm ใหม่สำเร็จ

ผลจากหลายไฟล์ต้องมี receipt/extraction ID และ page/line provenance
ไม่รวมยอดของทุกไฟล์โดยอัตโนมัติ เพราะหลายไฟล์อาจเป็นสำเนาบิลเดียวกันหรือคนละหน้า
การรวมผลหลายบิลเข้า Expense เดียวเป็นข้อกำหนดเพิ่มเติมที่ต้องตกลงหาก UI ต้องการ
ในระยะแรก OCR เติมร่างจากไฟล์ที่ PC เลือกอ่านและ PC ตรวจยอดสุดท้ายก่อนยืนยัน

## 4. API ที่เสนอ

ทุก path มี prefix `/api/v1`; ใช้ Bearer auth, DTO envelope และ error mapping เดิม

| Method | Path | หน้าที่ / success |
| --- | --- | --- |
| GET | /advances/{id}/expenses | รายการที่มองเห็นพร้อม page/page_size, meta และ data: []; 200 |
| POST | /advances/{id}/expenses | สร้างรายการก่อนมีราคาได้; amount เป็น null, status=DRAFT; 201 |
| GET | /expenses/{id} | คืนร่าง รายละเอียด ยอดยืนยัน และ version; 200 |
| PATCH | /expenses/{id} | แก้ร่างตาม expected_version; 200 |
| DELETE | /expenses/{id} | hard delete domain rows และจัดการลบไฟล์; 204 หรือ 202 ตามข้อ 8 |
| POST | /expenses/{id}/receipts | multipart/form-data field file หนึ่งไฟล์; 201 |
| GET | /expenses/{id}/receipts | metadata ของใบเสร็จ; 200 |
| GET | /expenses/{id}/receipts/{receipt_id}/file | backend ตรวจสิทธิ์แล้ว stream ไฟล์ private; 200 |
| DELETE | /expenses/{id}/receipts/{receipt_id} | ลบใบเสร็จและ OCR ของไฟล์นั้น; 204 หรือ 202 ตามข้อ 8 |
| POST | /expenses/{id}/ocr | รับ receipt_id และ force; ทำ synchronous/คืนผล cache; 200 |
| GET | /expenses/{id}/ocr-result | receipt_id เลือกไฟล์; extraction_id แบบ optional เลือก attempt; 200 |
| POST | /expenses/{id}/confirm | PC ยืนยันร่าง version ที่ตรวจแล้ว; 200 |
| POST | /expenses/{id}/reopen | PC ยกเลิกการยืนยันและหยุดนับยอดเพื่อกลับมาแก้; 200 |
| GET | /advances/{id}/reconciliation | summary ที่คำนวณใหม่จากยอดยืนยันทั้ง Advance; 200 |

`POST /receipts` ใช้เป็น path หลัก และยังรองรับ `POST /receipt` เดิมเป็น alias
endpoint confirm เป็นสิ่งที่เพิ่มเพื่อให้การบันทึกร่างแยกจากการกดยืนยันอย่างชัดเจน
reconciliation endpoint เป็นข้อเสนอสำหรับหน้าสรุป ส่วน shared calculation จำเป็นต่อ acceptance

สร้างร่างด้วย POST body `{}` ได้ เพื่อเริ่มจากการแนบบิลใน UI
backend สร้าง expense_id ให้ก่อน upload; ไม่บังคับ PC กรอกยอดหรือรายละเอียดก่อน OCR

ตัวอย่าง PATCH หลัง OCR กรอกฟอร์มแล้ว PC แก้ยอด:

```json
{
  "expected_version": 2,
  "expense_date": "2026-11-14",
  "description": "ค่าเดินทางไปโครงการ",
  "amount": "1200.00",
  "line_items": [
    {"line_no": 1, "description": "ค่าเดินทาง", "quantity": "1", "unit_price": "1200.00", "line_total": "1200.00"}
  ]
}
```

ยืนยันด้วย `{"expected_version":3}` ผ่าน POST /confirm
service เปลี่ยนรายการที่ล็อกอยู่เป็น CONFIRMED โดยใช้ amount ค่าเดิม ไม่เชื่อยอดรวมจากผล OCR
หรือ actor ที่ client ส่งมา ปุ่มยืนยันใน UI ต้อง save ร่างล่าสุดแล้วใช้ version ที่ได้กลับมา
กรณีไม่ได้แก้ข้อมูล ใช้ version จาก OCR response ได้เลย

OCR body: `{"receipt_id":45,"force":false}`; `force:true` คือคำสั่งอ่านใหม่โดยผู้ใช้
GET ocr-result ต้องตรวจว่า extraction_id ถ้าระบุตรงกับ receipt และ Expense ที่ขอ
response มีผลอ่าน รายการ/ยอดที่แยกได้ raw text, status/error, cached และ draft_applied
ไม่ส่ง storage path, credentials หรือ provider error ดิบออกสู่ client

ข้อเสนอ PATCH: omit = คงค่า; explicit null ล้างได้เฉพาะ nullable draft fields;
line_items=[] ล้างรายการร่างได้; expected_version และ IDs ต้องเป็นจำนวนบวก
เงินใช้ decimal string สูงสุด 13 หลักหน้าจุดและ 2 หลักหลังจุด ไม่ปัดเศษเงียบ ๆ
ไม่บังคับยอดรวมบิลเท่ากับผลบวกราคาแต่ละบรรทัดจนกติกาส่วนลด/ภาษีได้รับการยืนยัน

ข้อเสนอ HTTP errors: 400 input/เงินผิดรูปแบบ, 401 auth, 403 สิทธิ์,
404 resource ไม่มี, 409 non-ACTIVE/closed/stale version/เกินวงเงิน/จำนวนไฟล์เต็ม/กำลัง OCR,
413 เกินขนาด, 415 MIME ไม่รองรับ, 422 PDF เสีย/เข้ารหัส/เกินหน้า,
502 provider fail/ผลเสียรูปแบบ และ 504 deadline หมด
เก็บ FAILED attempt ด้วย error_code ที่ตัดข้อมูลลับแล้วและให้ GET ผลล้มเหลวได้

## 5. สิทธิ์และการทำงานพร้อมกัน

- writes: PC ต้องเป็นผู้สร้าง Advance และยังเป็น PC ปัจจุบันของ Project; Advance ACTIVE
  ไม่ใช้ expense.created_by จำกัดสิทธิ์เพิ่มหากผู้สร้าง Advance เดิมยังรับผิดชอบ Fund อยู่
- reads เสนอใช้ scope ของ Advance เดิม: PC/PM ตาม Project, ADMIN/ACCOUNTING ตามสิทธิ์เดิม
  CLOSED อ่านได้ตาม scope นี้; การให้ PC เก่าอ่านหลังถูกย้ายไม่ถือว่าได้รับอนุมัติเพิ่ม
- การเปลี่ยนหรือลบ PC จริงต้องตรวจ Fund ที่ค้างภายใต้ Project lock; การส่ง PC ID เดิมไม่ใช่การย้าย
  การกำหนดให้ DRAFT/APPROVED ที่ไม่เคยเปิด Fund บล็อกการย้ายด้วยหรือไม่อยู่ในข้อ 12
- ใช้ lock order Project -> Advance -> Expense -> Document/Extraction ทุก path ที่ใช้หลาย lock
- อ่านสถานะ/สมาชิกซ้ำใน transaction ก่อนเขียน; ใช้ WithContext(ctx) และ bound parameters
- confirm ล็อก Advance แล้วรวมยอดโดยไม่ join receipts/OCR และไม่ใช้ pagination
  `new_total = current_confirmed_total + locked_amount` สำหรับรายการ DRAFT
  ต้อง `new_total <= advances.amount` และยอดยืนยัน >= 0
- รวมยอดเฉพาะ status=CONFIRMED; ห้ามใช้ amount > 0 หรือ amount IS NOT NULL แทนสถานะยืนยัน
- duplicate confirmation ของ version เดิมเสนอคืนผลเดิมโดยไม่เพิ่ม audit/ยอดซ้ำ
- receipt count/size ตรวจซ้ำขณะถือ Expense lock ก่อน commit กัน upload พร้อมกันเกิน 5 ไฟล์/50 MB
- external storage/OCR อยู่นอก database transaction; recheck auth/state/version ตอน finalize
- ระหว่าง OCR บล็อก PATCH/Confirm/reopen/OCR ซ้ำทั้ง Expense ด้วย 409 รวมถึงคนละใบเสร็จ
  เมื่อจบแล้วเติมราคาใน DRAFT และเพิ่ม Version ก่อนเปิดให้ PC แก้/ยืนยัน; cache hit ไม่เติมทับร่างซ้ำ
  timeout/cancel/provider error/บันทึกผลล้มเหลวต้องปลดล็อก; attempt หลัง crash เกิน 30 วินาทีเก็บกวาดก่อนแก้หรือเริ่มใหม่
- delete receipt/expense ระหว่าง OCR ต้องไม่ทำให้ผลที่กลับมาสร้าง record ที่ลบแล้วขึ้นใหม่
- Closing ต้องมี integration contract ป้องกัน OCR completion เขียนหลัง CLOSED
  เสนอให้ finalize รอ attempt ที่กำลังทำงานจบ/ถูกยกเลิก และจัดการ stale attempt ภายใต้ lock
  ไม่ถือ SQL transaction ยาวตลอดการอ่าน OCR

## 6. OCR adapter และการกรอกฟอร์ม

ใช้ interface เช่น Extract(ctx, document) -> raw result + normalized candidates
เรียกเซิร์ฟเวอร์ส่วนตัวจาก Go ผ่าน HTTP โดยกำหนด base URL/model/auth ผ่าน config.Load()
ก่อนเขียน adapter ต้องได้ API contract และตัวอย่าง response ที่ลบข้อมูลส่วนตัวแล้ว
ยังไม่ทราบว่า endpoint เป็น OpenAI-compatible, multipart หรือ schema เฉพาะ
ไม่ถือว่ามี JSON ราคาแต่ละบรรทัดโดยอัตโนมัติเพียงเพราะเป็นโมเดล OCR

pipeline: ตรวจไฟล์ -> เตรียมแต่ละหน้า -> เรียก OCR -> parse ข้อความ/ตาราง
-> สร้างรายการและ draft total -> เติมร่างให้อัตโนมัติ -> คืนฟอร์มให้ PC ตรวจ
การแปลงผลเป็นรายการต้องแยก parser/version ออกจาก HTTP adapter
ค่าที่อ่านไม่ได้/กำกวมต้องมี warning หรือ null ไม่เดายอดเป็น 0
OCR สำเร็จแต่ไม่พบราคาได้ต้องให้ PC เห็นผลและกรอก/แก้ได้ก่อนยืนยัน

deadline 30 วินาทีใช้ร่วมกันทั้งการเตรียมหน้าและทุก OCR call ของไฟล์
ไม่มีการเริ่มนับ 30 วินาทีใหม่ต่อหน้า/ต่อ retry ถ้าไม่ครบทุกหน้าต้องไม่ cache ว่าสำเร็จทั้งไฟล์
การ cancel/timeout ต้องทำให้ attempt สิ้นสุดเป็น FAILED ด้วย bounded cleanup context
ที่ไม่อาศัย request context ซึ่งหมดเวลาไปแล้ว และไม่ทำให้ financial writes หลุดจาก transaction

technical statuses เสนอ PROCESSING/SUCCEEDED/FAILED; force ใหม่สร้าง attempt ใหม่
มีผลสำเร็จเดิมให้คืน cache จน force:true; force ล้มเหลวไม่ลบผลสำเร็จเดิม
กรณีไม่มีผลสำเร็จและ attempt ก่อนหน้าล้มเหลว ต้องแสดง failure และให้ PC สั่งอ่านใหม่
ไม่ยิง provider ซ้ำเองจากการ GET หรือจากการเปิดฟอร์ม
ใช้ receipt ID + hash ของไฟล์ immutable ระบุ cache; ไม่เปลี่ยนผลเดิมเพราะ config model เปลี่ยน

## 7. File storage และ validation

interface: Put/Open/Delete พร้อม context; Local/GCS และ fake สำหรับ tests
เพิ่มการค้น staging ที่หมดอายุสำหรับ recovery ภายใน backend ไม่เปิดเป็น public API
เก็บ provider/key/generation/hash ใน metadata; client ใช้ receipt ID
ตรวจชนิดไฟล์จากเนื้อหาและ decode/parse ไม่เชื่อ extension/Content-Type จาก client อย่างเดียว
ตรวจไฟล์ไม่ว่าง, PDF encryption/page count, image dimensions/resource limits และชื่อไฟล์
ใช้ key ที่ server สร้าง ไม่ใช้ original filename เป็น path

ข้อเสนอแปล MB เป็น 1,000,000 bytes ให้เอกสารและ tests ใช้เกณฑ์เดียวกัน
body limit เผื่อ multipart overhead แต่ตรวจ actual file bytes <= 10,000,000
ค่าใช้จ่ายรวมไฟล์ <= 50,000,000 และจำนวน <= 5 ภายใต้ lock
ห้าม overwrite ไฟล์เดิมด้วย key เดิม; เปลี่ยนไฟล์ด้วยลบ/แนบใหม่

อัปโหลดไฟล์ private ก่อน จากนั้น transaction ตรวจสิทธิ์/สถานะ/เพดานซ้ำและบันทึก metadata/audit
ถ้าล้มเหลวให้ลบ staging/orphan file พร้อม durable cleanup เมื่อการลบทันทีล้มเหลว
ถ้า DB ใช้ไม่ได้จนบันทึก cleanup task ไม่ได้ ต้องมี expiry/reconciliation ของ staging prefix
เพื่อเก็บตกไฟล์หลัง restart; อย่าอ้างว่า DB transaction rollback ลบ bytes ใน storage ได้
Local และ GCS เปิดผ่าน backend ที่ตรวจสิทธิ์ ไม่คืน public URL
ก่อน deploy ตรวจ bucket region/Standard/private และ retention/versioning/soft-delete policy
ให้ตรงคำสั่งลบจริง ไม่เปลี่ยน cloud settings ในงานวางแผน

## 8. Hard delete, audit และการลบไฟล์ที่ retry ได้

PostgreSQL กับ Local/GCS ไม่มี transaction ร่วม จึงเสนอ technical table
`file_deletion_tasks` ใน DB-07 เก็บเฉพาะตำแหน่งไฟล์/สถานะ retry ไม่เก็บไฟล์หรือยอดการเงิน
table นี้มีเหตุผลเพื่อให้ hard delete ไม่ทิ้งไฟล์ที่ไม่มี record อ้างอิงเมื่อ storage ล้มเหลว

transaction ของ DELETE: ตรวจสิทธิ์/ACTIVE -> สร้าง deletion tasks
-> hard delete OCR items/extractions/documents/Expense ตามขอบเขต
-> เขียน DELETE audit ที่อ้าง deleted ID โดยไม่ใช้ FK บังคับให้ Expense ต้องยังอยู่ -> commit
หลัง commit ลองลบไฟล์ทันที; ถ้าครบคืน 204 ถ้ายังมีงานค้างเสนอคืน 202 พร้อมสถานะ cleanup
worker ใน backend process เดิมรับงานที่ค้างและ retry หลัง restart ได้ ไม่มี restore API
ถ้า database commit ล้มเหลว ยังไม่ลบไฟล์จริง
หาก storage ลบสำเร็จแต่บันทึก completion ล้มเหลว retry Delete ต้องถือ not-found ว่าสำเร็จ

task เก็บ provider/key/generation เพื่อไม่ลบ object รุ่นอื่น
ใช้ lease/lock สั้นตอน claim task ไม่ถือ DB lock ระหว่าง storage call
audit มี actor/action/time, recorded_expense_id และ before/after ที่จำเป็น
ไม่คัดลอก file bytes/raw OCR ลง audit; audit schema ใช้ nullable FK/recorded ID เพื่ออยู่ต่อหลัง hard delete
การลบ Expense ไม่ใช่การลบ workflow/audit ทั้ง Advance
การลบเฉพาะ receipt ไม่เปลี่ยน amount/สถานะของ Expense ที่ยังอยู่โดยอัตโนมัติ
การเปลี่ยนยอดเงินจริงยังต้องผ่าน PC confirm หรือการลบ Expense ทั้งรายการ

เมื่อ Closing snapshots ถูกเพิ่มใน DB-09 ต้องกำหนด FK/การแก้ไขระหว่าง Closing ก่อนเปิดใช้
ห้ามเพิ่ม CASCADE ไปทำลายหลักฐาน Closing ที่ถูก finalize แล้ว

## 9. Migration units และ recovery

เลขด้านล่างเป็นข้อเสนอจากแผนกลาง ยังไม่จองข้าม branch; integration owner ต้องตรวจเลขก่อน merge
แต่ละ stem ต้องส่งทั้ง `.up.sql` และ `.down.sql`

| Version / file stem | Purpose / objects | Depends on | Owner / reviewer | Readiness / gate |
| --- | --- | --- | --- | --- |
| 0007_create_expense_records | expenses + draft/confirmation + audit + documents + file_deletion_tasks + keys/indexes | 0005, users/projects | Expense owner / Advance reviewer | SQL/API implement แล้ว ยังไม่ apply/deploy |
| 0008_create_ocr_evidence | extractions + immutable OCR items + Expense-wide in-flight guard ใน service + per-document DB guard | 0007 | OCR owner / Expense reviewer | SQL/orchestration implement แล้ว; adapter/parser จริงรอ private API contract |

คู่ไฟล์ที่เสนอใน repository:

```text
migrations/0007_create_expense_records.up.sql
migrations/0007_create_expense_records.down.sql
migrations/0008_create_ocr_evidence.up.sql
migrations/0008_create_ocr_evidence.down.sql
```

DB-07 รวม parent/children/audit/cleanup เพราะเป็น capability เดียวที่ต้องสร้าง แก้ ยืนยัน
และลบได้ครบ พร้อม composite FK `(expense_id, advance_id)` ป้องกันผูกคนละ Fund
up ไม่มี backfill ถ้าฐานจริงยังไม่มีตารางเหล่านี้; ตรวจ schema จริงก่อน execute
ตารางใหม่เป็น additive ต่อ app เดิม แต่ห้ามใช้ GORM AutoMigrate แทน migration
หากพบ populated/manual Expense schema ต้องออก expand/backfill/validate plan ก่อน ไม่ drop ทิ้ง
FK ไป users/advances RESTRICT; document/item lifecycle cascade เฉพาะ children ที่ผู้ใช้สั่งลบ
audit ใช้ SET NULL + recorded ID และ cleanup tasks ไม่มี FK ไป domain rows ที่ต้องถูกลบ
ข้อจำกัดเงิน/confirmation tuple อยู่ใน DB; การคุมยอดรวม/จำนวนไฟล์อยู่ใน transaction service

DB-08 แยกจาก DB-07 เพราะ Expense/storage/confirmation ต้องใช้ได้เมื่อ OCR provider ล้มเหลว
รุ่นที่เพิ่ม Expense-wide OCR lock ต้อง apply ทั้ง 0007/0008 ก่อนเปิด PATCH/Confirm/reopen
provider ล้มเหลวต่างจาก schema 0008 ไม่มีอยู่; ถอน 0008 ต้องหยุด paths ที่พึ่ง lock ก่อน แต่ยอด Expense เดิมไม่เปลี่ยน
up เพิ่ม OCR FK/index/technical status และ guard สำหรับ attempt ที่กำลังทำงาน
ไม่มี FK ย้อนจาก DB-07 ไปตาราง DB-08; provenance ที่บันทึกในร่าง/audit ไม่สร้าง dependency cycle
rollback DB-08 ลบ OCR items ก่อน extractions และไม่ลบยอดยืนยันของ Expense

ลำดับ dependency ของงานนี้: 0005 -> 0007 -> 0008
0006 disbursement ไม่ใช่ schema dependency ของ Expense แต่เป็น prerequisite ของ production ACTIVE flow
ถ้ามี 0006 ที่ review พร้อมใน release runner จะเรียง 0005 -> 0006 -> 0007 -> 0008
อย่าสร้าง placeholder 0006 เพื่อให้เลขต่อกัน หรือส่ง migration อื่นที่ยังไม่พร้อมให้ runner รัน
ก่อน run ต้องแก้ template discovery ที่รวม `.template.up.sql`; ไม่ renumber migration เดิม
แต่ละ file commit แยกกัน ถ้า 0008 fail ให้ตรวจว่า 0007 applied แล้วและ resume เฉพาะ pending version
runner ไม่มี target version และใช้ CREATE INDEX CONCURRENTLY ใน transaction ไม่ได้

down: ถอนแอป path ที่ใช้ OCR -> 0008 down -> หยุด path Expense/cleanup worker
-> ให้ pending cleanup จบหรือ export งานที่ยังค้าง -> 0007 down
ถ้า 0009 มีแล้วต้องถอน dependency ของ Closing ก่อน
down ลบข้อมูล ไม่กู้ไฟล์ที่เคย hard delete; up ซ้ำไม่คืนข้อมูล ต้อง backup/restore หรือ forward fix
ทดสอบ round trip บน disposable DB/storage fixtures เท่านั้น

## 10. ลำดับ implementation และ deliverables

| ขั้น | งาน / ไฟล์ที่ได้รับผล | เงื่อนไขจบ |
| --- | --- | --- |
| 1 | ปรับ blueprint/API contract, จองเลข migration, เก็บ private OCR API contract | null/zero/draft/confirmation, payload และ FK ใช้ตรงกัน |
| 2 | DB-07, model/expense.go, dto/expense.go, repository/service/handler/expense.go | CRUD ร่าง/confirm/delete, audit rollback, money และ lock tests ผ่าน |
| 3 | storage interface + local/GCS adapters, receipt slice, config wiring, cleanup worker | upload/download/delete พร้อม failure recovery และ quota concurrency |
| 4 | DB-08, model/dto/repository/service/handler OCR และ HTTP/parser adapter | OCR auto-fill, PC correction, cache/force, timeout และ malformed response ผ่าน fake server |
| 5 | reconciliation service/DTO/handler และ Project PC reassignment guard | ใช้ amount เฉพาะ CONFIRMED, กัน overspend/ย้าย PC ที่ยังเคลียร์ Fund ไม่เสร็จ |
| 6 | router.New, cmd/server, errors, Swagger annotations/generated docs, Postman, README | API และเอกสารตรง route; integration checks พร้อมหลักฐาน |

ใหม่ที่เสนอ: internal/storage/, internal/ocr/, internal/pkg/money/;
service ไม่เรียก SQL โดยตรงและ handler ไม่เรียก provider/storage ข้าม service
ใช้ shared exact arithmetic โดยไม่มี float64; aggregate ที่อาจโตเกิน NUMERIC(15,2)
ต้องรองรับได้ด้วย decimal/big integer ไม่ truncate หรือ overflow int โดยไม่ตรวจ
config เพิ่ม storage provider/root/bucket และ OCR base URL/model/auth/timeout โดยไม่บันทึก secret ใน repo
PDF parser/renderer และ GCS SDK ต้องเลือกและ review dependency ก่อนเพิ่มจริง

Expense owner ทำ steps 2/3; OCR owner ทำ step 4 หลัง document contract คงที่
Advance/Closing owner review steps 5/closing integration; integration owner ดูแล version/router/docs
roles เหล่านี้รวมเป็นคนเดียวได้ ไม่กำหนดจำนวนคนหรือ deadline ที่ผู้ใช้ไม่ได้ระบุ
test fixtures/fake server/parser samples เตรียมขนานกับ implementation ได้เมื่อ interface คงที่

## 11. Tests และ definition of done

tests ใหม่อยู่ root-level tests/ และไม่พึ่ง helper ที่ถูก ignore โดยไม่มีวิธีส่งมอบ

| กลุ่ม | Acceptance cases |
| --- | --- |
| Workflow/สิทธิ์ | ทุก non-ACTIVE เขียนไม่ได้; CLOSED อ่านตามสิทธิ์ได้; role ผิด/PC อื่น/actor ปลอม/cross-Expense IDs ถูกปฏิเสธ |
| Draft/confirm | create ก่อนมียอด; OCR auto-fill รายการ/ราคา/ยอดรวม; PC แก้แล้ว confirm ใช้ค่าที่แก้; null ต่างจาก 0; ไม่รับ negative/float JSON/overflow/excess precision |
| Money | confirm เท่าวงเงินได้ เกินไม่ได้; reopen หยุดนับยอดจน Confirm ใหม่; zero ยังมี confirmed_by/at; หลายไฟล์/attempt ไม่ทำยอดซ้ำ |
| Concurrency | confirm สองรายการพร้อมกันไม่เกินวงเงิน; stale confirm/PATCH ไม่ทับข้อมูล; เปลี่ยน PC/close แข่งกับ mutation; audit/commit fail rollback |
| Files | รับเฉพาะชนิดที่กำหนด; ขอบเขต 10 MB/5 files/50 MB; upload พร้อมกันไม่ทะลุ; PDF 1/5 หน้าได้ 6 หน้า/เข้ารหัส/เสียไม่ได้; spoofed MIME/path/empty file |
| OCR | ทุกหน้าอยู่ใน deadline เดียว; timeout, cancel, HTTP error, invalid JSON/markdown, อ่านราคาไม่ได้; GET/success cache ไม่เรียก provider; FAILED retry ได้; force fail ยังเก็บผลสำเร็จเดิม |
| Data loss | PC แก้/Confirm/อ่านซ้ำระหว่าง OCR ไม่ได้; หลังจบทุก outcome ใช้งานต่อได้; cache ไม่ reset ร่าง; result หลังลบไม่สร้างข้อมูลคืน; OCR ไม่แก้ amount ของ CONFIRMED รวมทั้ง 0 และไม่เปลี่ยนสถานะยืนยัน |
| Delete/recovery | domain hard delete จริง; audit ยังอ่านได้; storage fail/restart แล้ว retry สำเร็จ; ลบ object generation ถูกตัว; ไม่มี 204 หาก cleanup ยังไม่เสร็จ |
| Reconciliation | รวมทั้ง Advance ไม่ใช่หน้าปัจจุบัน; ไม่รวม unconfirmed; ไม่ join OCR/receipts จนยอดซ้ำ; ใช้สูตร Closing เดียวกัน |
| Migration | clean up, upgrade จาก 0005 พร้อมข้อมูลเดิม, nullable/zero/tuple/FKs, down/up ที่รักษาตาราง upstream; DB-08 down ไม่กระทบ confirmed amounts |

ลำดับ checks เมื่อ implement Go/SQL จริง:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./cmd/server ./cmd/migrate
git diff --check
```

PostgreSQL tests ใช้ ERBM_MIGRATION_TEST_DATABASE_URL เฉพาะ disposable DB และ isolated schema
ถ้าไม่มีให้รายงาน skip; storage/fake OCR ใช้ temporary fixtures ไม่ส่งใบเสร็จจริงไป provider
การพิสูจน์ private server 30 วินาที/หลายหน้าเป็น integration acceptance แยกจาก unit tests
ไม่อ้างว่าผ่าน provider จริงจากการใช้ fake HTTP server

หลักฐาน baseline ในแชตก่อนหน้า: vet/build ผ่าน; test suite fail สอง activation mail logging tests
เป็นงานเดิม ไม่ใช่ผลของ Expense/OCR ซึ่งยังไม่ได้ implement; ไม่แก้หรือตัด test เหล่านั้นใน ticket นี้
รอบวางแผนนี้ตรวจ source/status/document consistency เท่านั้น ไม่รัน Go tests/SQL/provider อีกครั้ง

## 12. Integration gates และข้อเสนอที่ยังไม่ใช่ requirement ยืนยัน

| Gate | ต้องตัดสิน/จัดเตรียม | กระทบ | งานที่เริ่มได้ก่อน |
| --- | --- | --- | --- |
| G1 Private OCR contract | URL, auth method, exact 7B model identifier, request/response และ PDF processing contract; ไม่ส่ง key ในแชต | adapter/parser step 4, DB-08 metadata | fake provider, DTO/storage/Expense/confirmation tests |
| G2 Closing | จะ freeze ตั้งแต่เริ่ม Closing หรือเมื่อ CLOSED; เสนอให้เคลียร์ unconfirmed/dirty draft และ in-flight OCR ก่อน finalize; วิธีคืน/ตรวจเงินและ actual disbursement ยังอยู่ D04/D07 | production close/reassignment, DB-09 | shared totals, ACTIVE guards, fixture-based tests |
| G3 Request versus Fund | ยืนยันแล้ว: เปลี่ยน/ลบ PC ต้องให้ Advance ทุกสถานะ CLOSED; ไม่เปลี่ยนนโยบายสร้าง DRAFT เดิมใน ticket นี้ | Project guard implement แล้ว; Advance Create policy คงเดิม | Expense CRUD/confirm/quota/OCR |
| G4 Contract review | ผู้ใช้อนุญาตเริ่ม implementation แล้ว; ใช้ version/confirm/202 cleanup และ audit ตามแผน | มี SQL/API/Swagger/Postman และ tests แล้ว ยังไม่ deploy | adapter จริงยังรอ G1 |

ไม่เพิ่ม closing status, ยกเลิก DRAFT, VAT/WHT, การรวมหลายบิล หรือการโอน Fund ให้ PC ใหม่จากการเดา
งานถัดไปที่เริ่มได้คือ review contract/จองเลข และเตรียม failing tests ของ DB-07/Expense/storage
production end-to-end พร้อมเมื่อ disbursement/activation, Closing contract และ private OCR integration ผ่านด้วย

## 13. Implementation handoff

มี migration 0007/0008, DTO/model/repository/service/handler/router, money arithmetic, private local/GCS storage และ maintenance worker แล้ว เอกสาร contract ปัจจุบันอยู่ใน [Expense API](../expense-api.md) พร้อม Postman/Swagger

ทดสอบใหม่ครอบคลุม ACTIVE/สิทธิ์, ยอดศูนย์/ติดลบ/เกินวงเงิน/version, OCR autofill/cache/failure/timeout/concurrent edit, PDF/ไฟล์ และ cleanup retry; PostgreSQL tests รันเฉพาะฐาน disposable ที่ตั้งค่าอย่างชัดเจน ยังไม่ทดสอบ cloud/เซิร์ฟเวอร์ OCR จริง ไม่มี migration ที่รันกับฐาน application และไม่มี Closing/activation API เพิ่มใน ticket นี้

## 14. การปรับเป็นยอดเดียวตามคำยืนยันล่าสุด

ใช้ amount เดียวพร้อม DRAFT/CONFIRMED แทนสอง field เงิน; PC กด POST /expenses/{id}/reopen พร้อม expected_version เพื่อยกเลิกการยืนยันและหยุดนับยอดก่อนแก้ แล้ว Confirm ใหม่โดยตรวจวงเงินปัจจุบันอีกครั้ง OCR ไม่ยืนยันเองและระหว่าง OCR บล็อก confirm/reopen และใช้ Version ป้องกันหน้าจอเก่าทับผลหลังอ่านสำเร็จ

ผู้ใช้ยืนยันว่า 0007/0008 ยังไม่เคย apply จึงแก้ 0007 เดิมเป็น amount/status ไม่เพิ่ม migration version ใหม่ และไม่มีการรัน migration กับฐาน application

## 15. ล็อกระหว่าง OCR ตามคำยืนยันล่าสุด

Expense-wide PROCESSING ล็อก PATCH/Confirm/reopen/OCR ซ้ำทุกไฟล์ โดยตรวจใน transaction ที่ล็อก Project -> Advance -> Expense; ไม่ใช้ mutex ใน process และไม่ถือ SQL lock ระหว่าง OCR สำเร็จจะเพิ่ม Version/เติมราคาใน DRAFT ให้รีวิว ล้มเหลว/timeout/cancel/provider panic หรือบันทึกผลล้มเหลวจะสิ้นสุด attempt และเปิดให้แก้/ลองใหม่; หลัง crash ใช้ StartedAt หมดอายุ 30 วินาที ป้องกันค้างถาวรและทิ้งผลเก่า

ตัด DraftVersionAtRequest, DirtyCount, ExpenseAuditEvent.Reason และ OCRLineItem.RawData ออกจาก model/repository/SQL/tests โดยเก็บ Expense.Version และ raw_result ต้นฉบับ; needs_confirmation_count เดิม map จาก UnconfirmedCount เพื่อคง API response contract
