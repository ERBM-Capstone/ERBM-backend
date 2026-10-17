# ERBM Backend

REST API สำหรับ **Expense Reimbursement and Bill Management System (ERBM)** ระบบจัดการเงินทดรองจ่าย ค่าใช้จ่าย และการเบิกคืนของโครงการ พัฒนาด้วย Go, Gin และ PostgreSQL

## ความสามารถปัจจุบัน

- เข้าสู่ระบบด้วยอีเมลและรหัสผ่าน พร้อม JWT access token และ refresh token
- เปิดใช้งานบัญชีผ่านลิงก์อีเมล ตั้งรหัสผ่านครั้งแรก และส่งลิงก์ใหม่
- ตรวจสอบผู้ใช้ปัจจุบันและควบคุมสิทธิ์ตามบทบาท
- จัดการพนักงาน สร้างหลายรายการในคำขอเดียว และปิดใช้งานบัญชีเมื่อพ้นสภาพพนักงาน
- จัดการโครงการ งบประมาณ PM และ PC พร้อมสิทธิ์การดู แก้ไข และปิดโครงการ
- ดูรายการ Advance และสร้าง DRAFT พร้อมรหัสแยกตาม Project และประวัติการสร้าง
- ดูเอกสาร API ผ่าน Swagger UI

Advance มี API ดูรายการและสร้าง DRAFT แล้ว รายละเอียดอยู่ใน [GET Advances](docs/api/advance-list.md) และ [POST Advances](docs/api/advance-create.md) ส่วนการส่งอนุมัติ การเบิกจ่าย Expense, OCR และการปิดเงินทดรองจ่ายยังเป็นงานถัดไป

## เทคโนโลยี

| ส่วน                 | เทคโนโลยี                                                |
| -------------------- | -------------------------------------------------------- |
| ภาษา                 | Go ตามเวอร์ชันใน [go.mod](go.mod) ซึ่งประกาศ `go 1.25.0` |
| HTTP framework       | Gin                                                      |
| ฐานข้อมูล            | PostgreSQL; Docker Compose ใช้ `postgres:15-alpine`      |
| ORM                  | GORM และ PostgreSQL driver                               |
| Migration            | CLI ภายในโปรเจกต์ ใช้ pgx และไฟล์ SQL                    |
| Authentication       | JWT HS256 และ bcrypt                                     |
| อีเมลเปิดใช้งานบัญชี | SMTP                                                     |
| เอกสาร API           | Swaggo / Swagger UI                                      |

## โครงสร้างโปรเจกต์

```text
.
├── cmd/
│   ├── server/          # จุดเริ่มต้นของ API server
│   └── migrate/         # คำสั่ง migration up / down
├── config/             # โหลด configuration ผ่าน config.Load()
├── dto/                # Request, response และ error ที่ใช้ใน API
├── internal/
│   ├── handler/         # รับ HTTP request และส่ง response
│   ├── service/         # Business rules และ transaction orchestration
│   ├── repository/      # การเข้าถึงฐานข้อมูล
│   ├── model/           # โมเดลและสถานะของข้อมูล
│   ├── middleware/      # Authentication และ authorization
│   ├── router/          # Routes และ dependency wiring
│   ├── mail/            # ส่งอีเมลเปิดใช้งานบัญชี
│   └── pkg/             # JWT, password hashing และ activation token
├── migrations/         # SQL migrations แบบ up / down
├── docs/               # Swagger ที่สร้างจาก annotations
├── tests/              # Project-level tests
├── docker-compose.yml  # PostgreSQL สำหรับพัฒนาในเครื่อง
└── Makefile            # คำสั่งพัฒนาและดูแลโปรเจกต์
```

ลำดับการทำงานหลักคือ `router → handler → service → repository → model/database` โดย handler จัดการ HTTP, service ดูแลกฎธุรกิจ และ repository เข้าถึงฐานข้อมูลด้วย context ของคำขอ

## เริ่มใช้งานในเครื่อง

เตรียม Go ตาม [go.mod](go.mod), PostgreSQL และ Docker Compose หากต้องการใช้ฐานข้อมูลที่ให้มาในโปรเจกต์ ส่วน `make` เป็นทางเลือกสำหรับเรียกคำสั่งลัด

### 1. ดาวน์โหลด dependencies

รันคำสั่งจากโฟลเดอร์รากของ repository:

```sh
go mod download
```

### 2. ตั้งค่า environment

แอปอ่าน environment variables และโหลด `.env` ที่โฟลเดอร์รากหากมี โดยค่าที่กำหนดไว้ใน environment อยู่แล้วจะมีลำดับความสำคัญสูงกว่า `.env`

ตัวอย่างค่าขั้นต่ำสำหรับรัน server ด้านล่างเป็น **placeholder** ให้แทนด้วยค่าของเครื่องที่ใช้งานก่อนบันทึกใน `.env` หรือกำหนดผ่าน environment:

```dotenv
PORT=8080
DATABASE_URL=postgres://<db-user>:<db-password>@localhost:5432/<db-name>?sslmode=disable
JWT_SECRET=<access-signing-secret-at-least-32-bytes>
JWT_REFRESH_SECRET=<different-refresh-signing-secret-at-least-32-bytes>
JWT_ISSUER=erbm
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TTL=168h
REFRESH_COOKIE_SECURE=false
```

หากใช้ Docker Compose ให้ตั้งชื่อฐานข้อมูล ผู้ใช้ และรหัสผ่านใน `DATABASE_URL` ให้ตรงกับ service `postgres` ใน [docker-compose.yml](docker-compose.yml) โดยเชื่อมต่อผ่าน `localhost:5432` เมื่อรัน Go บนเครื่อง

JWT secrets ทั้งสองค่าต้องต่างกันและมีความยาวอย่างน้อย 32 bytes ส่วน `sslmode=disable` และ `REFRESH_COOKIE_SECURE=false` ในตัวอย่างใช้สำหรับการพัฒนาในเครื่อง เมื่อให้บริการผ่าน HTTPS ให้ใช้ `REFRESH_COOKIE_SECURE=true` และตั้งค่า SSL ของฐานข้อมูลตามสภาพแวดล้อม

ไม่ควร commit `.env` หรือค่าลับลง Git; โปรเจกต์มี ignore rules สำหรับไฟล์เหล่านี้แล้ว

### 3. เตรียมฐานข้อมูล

หากยังไม่มี PostgreSQL ให้เริ่ม service ที่ให้มา:

```sh
docker compose up -d postgres
```

Compose นี้รันเฉพาะ PostgreSQL และเก็บข้อมูลใน volume `postgres_data` ส่วน API server รันด้วย Go ตามขั้นตอนถัดไป

### 4. รัน migrations

```sh
go run ./cmd/migrate up
```

Migration runner อ่าน `DATABASE_URL`, ใช้ไฟล์ใน `migrations/` และบันทึกเวอร์ชันที่รันแล้วในตาราง `schema_migrations` แต่ละ migration ทำงานใน transaction ของตัวเอง

Migrations สร้าง schema และบทบาทเริ่มต้น แต่ **ไม่ได้สร้างบัญชี ADMIN หรือรหัสผ่านเริ่มต้น** ปัจจุบันยังไม่มีคำสั่ง bootstrap หรือ public registration endpoint จึงต้องเตรียมบัญชี ADMIN แรกผ่านกระบวนการของผู้ดูแลฐานข้อมูลก่อนใช้งาน API จัดการพนักงาน

### 5. รัน API server

```sh
go run ./cmd/server
```

ค่าเริ่มต้นของ server คือ `http://localhost:8080`:

- API prefix: `/api/v1`
- Swagger UI: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)

Server ตรวจ configuration ของ JWT และเชื่อมต่อฐานข้อมูลตอนเริ่มทำงาน แต่ไม่ได้รัน migrations ให้อัตโนมัติ

## Configuration

ค่าต่อไปนี้อ่านจาก [config/config.go](config/config.go):

| ตัวแปร                           | ค่าเริ่มต้น | หน้าที่                                                                 |
| -------------------------------- | ----------- | ----------------------------------------------------------------------- |
| `PORT`                           | `8080`      | Port ของ API server                                                     |
| `DATABASE_URL`                   | ไม่มี       | PostgreSQL connection string                                            |
| `JWT_SECRET`                     | ไม่มี       | Secret สำหรับ access token                                              |
| `JWT_REFRESH_SECRET`             | ไม่มี       | Secret สำหรับ refresh token                                             |
| `JWT_ISSUER`                     | `erbm`      | Issuer ของ JWT                                                          |
| `JWT_ACCESS_TOKEN_TTL`           | `15m`       | อายุ access token                                                       |
| `JWT_REFRESH_TTL`                | `168h`      | อายุ refresh token                                                      |
| `REFRESH_COOKIE_SECURE`          | `false`     | จำกัด refresh cookie ให้ส่งผ่าน HTTPS                                   |
| `AUTH_LOGIN_LOCK_DURATION`       | `3m`        | ระยะเวลาล็อกบัญชีเมื่อรหัสผ่านผิดถึงเกณฑ์                               |
| `AUTH_MAX_FAILED_LOGIN_ATTEMPTS` | `10`        | จำนวนครั้งที่ใส่รหัสผ่านผิดก่อนล็อกบัญชี                                |
| `SMTP_HOST`                      | ไม่มี       | SMTP hostname                                                           |
| `SMTP_PORT`                      | ไม่มี       | SMTP port                                                               |
| `SMTP_USERNAME`                  | ไม่มี       | บัญชีสำหรับส่งอีเมล                                                     |
| `SMTP_GMAIL_APP_PASSWORD`        | ไม่มี       | App Password สำหรับ SMTP sender                                         |
| `SMTP_FROM_ADDRESS`              | ไม่มี       | อีเมลผู้ส่ง                                                             |
| `FRONTEND_ACTIVATION_URL`        | ไม่มี       | URL หน้าตั้งรหัสผ่านของ frontend ซึ่งระบบจะเติม query parameter `token` |

ค่าระยะเวลาใช้รูปแบบของ Go เช่น `15m` หรือ `168h` และ JWT TTL ต้องมากกว่าศูนย์

ต้องตั้งค่า SMTP ทั้งห้าค่าและ `FRONTEND_ACTIVATION_URL` ให้ครบเพื่อให้ผู้ใช้รับและเปิดลิงก์ตั้งรหัสผ่านได้ หากไม่ได้ตั้งค่า SMTP server ยังเริ่มทำงานได้ แต่ส่งอีเมลไม่ได้ การสร้างพนักงานและบัญชีจะยังถูกบันทึกแม้ส่งอีเมลไม่สำเร็จ และสามารถส่งลิงก์ใหม่ภายหลังได้

## Authentication และสิทธิ์

บัญชีมีสถานะ `INVITED`, `ACTIVE` และ `DEACTIVATED` โดย flow เปิดใช้งานบัญชีเป็นดังนี้:

1. ADMIN สร้างพนักงานและระบุ `role_code` สำหรับบัญชี
2. พนักงานที่ยังทำงานอยู่ได้รับบัญชี `INVITED` และลิงก์ตั้งรหัสผ่านทางอีเมล
3. Frontend ส่ง `token` และ `password` ไปที่ `/api/v1/auth/activation/set-password`
4. บัญชีเปลี่ยนเป็น `ACTIVE` และเข้าสู่ระบบได้

Activation token ใช้ได้ครั้งเดียวและมีอายุ 24 ชั่วโมง การส่งลิงก์ใหม่จะยกเลิกลิงก์เก่าที่ยังไม่ได้ใช้ รหัสผ่านเริ่มต้นต้องผ่าน validation ความยาว 8–72

เมื่อ login สำเร็จ API ส่ง access token ใน JSON และ refresh token ใน cookie ชื่อ `refresh_token` ซึ่งเป็น `HttpOnly`, `SameSite=Lax` และมี path `/api/v1/auth` การเรียก refresh ต้องส่ง cookie นี้มาด้วย ส่วน protected endpoints ใช้ header:

```http
Authorization: Bearer <access_token>
```

Logout ล้าง refresh cookie โดยยังไม่มีการเพิกถอน token ฝั่ง server

| บทบาท        | สิทธิ์หลักใน API ปัจจุบัน                                                           |
| ------------ | ----------------------------------------------------------------------------------- |
| `ADMIN`      | ดู roles, จัดการพนักงาน, ดูทุกโครงการ และสร้างหรือแก้ไขโครงการ                      |
| `PM`         | สร้างโครงการ ดูโครงการที่ตนเป็นสมาชิก และแก้ไขหรือปิดโครงการที่ตนเป็น PM            |
| `PC`         | สร้างโครงการ ดูโครงการที่ตนเป็นสมาชิก และแก้ไขชื่อหรือรายละเอียดโครงการที่ตนเป็น PC |
| `ACCOUNTING` | ดูทุกโครงการ                                                                        |
| `EMPLOYEE`   | เข้าสู่ระบบและดูข้อมูลตนเอง; ไม่มีสิทธิ์จัดการพนักงานหรือโครงการ                    |

บทบาทของบัญชีแยกจาก `position` ของพนักงาน สำหรับโครงการมีข้อกำหนดเพิ่มเติม:

- ตอนสร้าง Project ไม่ต้องส่ง `project_code`; backend สร้าง `PRJ-2026-000001` โดยใช้ปี ค.ศ. ตามเวลาไทยและเลขลำดับอย่างน้อย 6 หลัก เริ่มใหม่ที่ 000001 ทุกปี รหัสอยู่ใน response และแก้ไขผ่าน PATCH ไม่ได้
- ADMIN และ PC ต้องระบุ `pm_user_id` ตอนสร้างโครงการ ส่วน PM ที่สร้างโครงการจะเป็น PM ของโครงการนั้นเอง
- PC ที่สร้างโครงการถูกเพิ่มเป็น PC ของโครงการโดยอัตโนมัติ
- หนึ่งโครงการมี PM 1 คนและ PC ได้ไม่เกิน 1 คน; PM และ PC แต่ละคนดูแลหลายโครงการได้ โดยเก็บ `pm_user_id` และ nullable `pc_user_id` ใน `projects` โดยตรง
- POST รับ `pc_user_id` เป็น ID เดียว; PATCH ใช้ field เดียวกัน โดยไม่ส่งเพื่อคงผู้รับผิดชอบเดิม ส่ง `null` เพื่อลบ PC หรือส่ง ID จำนวนเต็มบวกเพื่อเปลี่ยน PC เฉพาะ ADMIN หรือ PM ของโครงการเปลี่ยนการมอบหมายได้
- Response โครงการใช้ `pc` เป็น object หรือ `null` เช่นเดียวกับรูปแบบ `pm`; เฉพาะ PC ที่ได้รับมอบหมายเท่านั้นที่สร้าง Advance ได้ ดูรายละเอียดใน [Project coordinator](docs/api/project-coordinator.md)
- PM และ PC ที่ถูกมอบหมายต้องเป็นบัญชี `ACTIVE` และมีบทบาทตรงกับหน้าที่
- `budget` รับเป็น decimal string เช่น `"1000.00"` และวันที่ใช้รูปแบบ `YYYY-MM-DD`
- เฉพาะ PM ของโครงการปิดโครงการได้ด้วย `status: "CLOSED"` เมื่อไม่มี Advance หรือทุกใบเป็น `CLOSED` แล้ว ถ้ายังมีใบที่ไม่ `CLOSED` รวมถึง `DRAFT` จะคืน `409` โดยไม่บันทึก field อื่นใน PATCH นั้น โครงการที่ปิดแล้วแก้ไขหรือเปิดกลับไม่ได้

## API endpoints

Path ทั้งหมดในตารางมี prefix `/api/v1`:

| Method  | Path                                | การใช้งาน                         | สิทธิ์                    |
| ------- | ----------------------------------- | --------------------------------- | ------------------------- |
| `POST`  | `/auth/login`                       | เข้าสู่ระบบ                       | Public                    |
| `POST`  | `/auth/refresh`                     | ขอ access token ใหม่              | Refresh cookie            |
| `POST`  | `/auth/logout`                      | ล้าง refresh cookie               | Public                    |
| `POST`  | `/auth/activation/set-password`     | ตั้งรหัสผ่านครั้งแรก              | Activation token          |
| `POST`  | `/auth/activation/resend`           | ขอส่งลิงก์เปิดใช้งานใหม่ทางอีเมล  | Public                    |
| `GET`   | `/me`                               | ดูบัญชีและข้อมูลพนักงานของตนเอง   | Access token              |
| `GET`   | `/roles`                            | ดูบทบาทที่กำหนดให้บัญชีได้        | ADMIN                     |
| `GET`   | `/employees`                        | ดูรายการพนักงาน                   | ADMIN                     |
| `GET`   | `/employees/{id}`                   | ดูพนักงานรายคน                    | ADMIN                     |
| `POST`  | `/employees`                        | สร้างพนักงานหลายรายการ            | ADMIN                     |
| `PATCH` | `/employees/{id}`                   | แก้ไขข้อมูลพนักงาน                | ADMIN                     |
| `PATCH` | `/employees/{id}/deactivate`        | บันทึกวันพ้นสภาพและปิดใช้งานบัญชี | ADMIN                     |
| `POST`  | `/employees/{id}/resend-activation` | ส่งลิงก์เปิดใช้งานใหม่ให้พนักงาน  | ADMIN                     |
| `GET`   | `/projects`                         | ดูรายการโครงการ                   | ADMIN, PM, PC, ACCOUNTING |
| `GET`   | `/projects/{id}`                    | ดูรายละเอียดโครงการ               | ADMIN, PM, PC, ACCOUNTING |
| `POST`  | `/projects`                         | สร้างโครงการ                      | ADMIN, PM, PC             |
| `PATCH` | `/projects/{id}`                    | แก้ไขหรือปิดโครงการตามสิทธิ์      | ADMIN, PM, PC             |
| `GET`   | `/advances`                         | ดู Advance ตามสิทธิ์ พร้อมรหัสและผู้ขอ | ADMIN, PM, PC, ACCOUNTING |
| `POST`  | `/advances`                         | สร้าง Advance DRAFT ในโครงการที่ได้รับมอบหมาย | PC |

การดูหรือแก้ไขโครงการยังตรวจสิทธิ์ตามสมาชิกและ PM ของโครงการใน service ตามกฎด้านบน ดู payload และ HTTP status เพิ่มเติมได้ใน Swagger UI

### ตัวอย่างเข้าสู่ระบบ

แทนอีเมลและรหัสผ่านด้วยบัญชี `ACTIVE` ที่เตรียมไว้:

```sh
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"<your-password>"}'
```

ตัวอย่าง response body เมื่อสำเร็จ:

```json
{
  "success": true,
  "message": "OK",
  "data": {
    "access_token": "<access_token>",
    "expires_in": 900
  }
}
```

`expires_in` มีหน่วยเป็นวินาทีและเปลี่ยนตาม `JWT_ACCESS_TOKEN_TTL` จากนั้นใช้ token เพื่อดูข้อมูลตนเอง:

```sh
curl http://localhost:8080/api/v1/me \
  -H 'Authorization: Bearer <access_token>'
```

### ตัวอย่างสร้างพนักงาน

ส่ง JSON **array** ไปที่ `POST /api/v1/employees` ด้วย access token ของ ADMIN:

```json
[
  {
    "first_name": "Somchai",
    "last_name": "Jaidee",
    "position": "Project Coordinator",
    "email": "somchai@example.com",
    "date_hired": "2026-01-01",
    "role_code": "PC"
  }
]
```

### รูปแบบ response และ pagination

JSON responses ใช้ envelope `success`, `message` และอาจมี `data`, `errors` หรือ `meta` ส่วน HTTP `204` ไม่มี response body

รายการพนักงานและโครงการรองรับ `page` และ `page_size` โดย `page_size` สูงสุด 100 และ response ใช้ชื่อ `meta.per_page` รายการโครงการรองรับ `q` สำหรับค้นหารหัสหรือชื่อ และ `status` เป็น `ACTIVE` หรือ `CLOSED` เช่น:

```text
GET /api/v1/projects?page=1&page_size=20&q=ERBM&status=ACTIVE
```

## คำสั่งพัฒนา

| คำสั่ง                | หน้าที่                                            |
| --------------------- | -------------------------------------------------- |
| `go run ./cmd/server` | รัน server ด้วย Swagger ที่มีอยู่ใน repository     |
| `make dev`            | สร้าง Swagger ใหม่แล้วรัน server                   |
| `make swagger`        | สร้างเอกสารใน `docs/` จาก Swagger annotations      |
| `make build`          | สร้าง Swagger และ compile server เป็น `bin/server` |
| `make run`            | Build แล้วรัน `bin/server`                         |
| `make migrate-up`     | รัน migrations ที่ยังไม่ได้ใช้                     |
| `make migrate-down`   | ย้อนกลับ migration ล่าสุดหนึ่งรายการ               |
| `make migrate-create` | สร้างคู่ไฟล์ migration โดยถามชื่อ                  |
| `make test`           | รัน tests พร้อม coverage                           |
| `make vet`            | รัน `go vet ./...`                                 |
| `make docker-up`      | เริ่ม PostgreSQL ตาม Compose                       |
| `make docker-down`    | หยุดและลบ containers โดยเก็บ volume ไว้            |
| `make docker-logs`    | ดู logs ของ services ใน Compose                    |
| `make help`           | แสดงคำสั่งพร้อมคำอธิบาย                            |

คำสั่งที่เรียก `swagger` ใช้ `go run github.com/swaggo/swag/cmd/swag` และอาจดาวน์โหลด dependencies ในครั้งแรก

รัน checks หลักจากราก repository:

```sh
go test ./...
go vet ./...
go build ./cmd/server ./cmd/migrate
```

เมื่อแก้ไฟล์ Go ให้รัน `gofmt -w <changed-go-files>` และเพิ่ม tests สำหรับ behavior ที่เปลี่ยนใน `tests/` ตาม [AGENTS.md](AGENTS.md) หากแก้ routes ให้ปรับ Swagger annotations และสร้างเอกสารใหม่ให้ตรงกัน

## แนวทางพัฒนาต่อ

รักษา service/repository boundaries, ส่ง `context.Context` ผ่านงานฐานข้อมูล และใช้ transaction สำหรับงานการเงินหรือ workflow หลายขั้นตอน Actor ID ต้องมาจาก authentication และจำนวนเงินที่ใช้ตัดสินผลต้องคำนวณด้วยค่าทศนิยมที่แม่นยำ

เมื่อพัฒนา Advance และ Expense ให้ยึดข้อกำหนดใน [AGENTS.md](AGENTS.md) และ skills ภายใต้ `.agents/skills/`:

- Advance ต้องอยู่ภายใต้ Project และ Project มี Advance สถานะ `ACTIVE` ได้ไม่เกินหนึ่งรายการ
- PC สร้างและส่ง Advance Request; PM อนุมัติหรือส่งกลับ; Accounting ยืนยันการจ่ายและตรวจสอบการปิด
- บันทึก Expense ได้เฉพาะขณะ Advance เป็น `ACTIVE`
- OCR เป็นข้อมูลประกอบ ไม่ใช่ข้อมูลการเงินที่ใช้ตัดสินยอด
- การปิดต้องเป็นไปตาม `Advance Amount = Total Confirmed Expenses + Return Amount`
- ลายเซ็น MD อยู่นอกระบบใน phase 1

เพิ่ม migration ใหม่เมื่อต้องเปลี่ยน schema และไม่แก้ไฟล์ migration ที่ใช้ใน shared environment แล้ว
