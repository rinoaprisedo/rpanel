# RPanel — Laporan Penetration Test & Security Review

- **Tanggal:** 2026-10-26
- **Versi diuji:** 1.3.0 (temuan) → **1.3.1** (setelah perbaikan)
- **Lingkup:** seluruh backend Go (`backend/`), frontend React (`webadmin/`), skrip deploy & konfigurasi nginx (`deploy/`)
- **Metode:** review kode manual, analisa statis (`gosec`, `govulncheck`, `npm audit`), dan uji dinamis black-box pada VM Ubuntu 24.04 bersih
- **Model ancaman:** panel berjalan sebagai **root** dan mengelola nginx, ufw, docker, cron, database, dan filesystem. Akun panel = akses root ke server. Aktor yang dipertimbangkan: (a) penyerang tak terautentikasi dari internet, (b) user panel non-superadmin dengan izin terbatas, (c) kebocoran dump database.

## Ringkasan Eksekutif

Review menemukan **4 isu kritis/tinggi**, **6 sedang**, dan **4 rendah/informasi**. Semuanya sudah diperbaiki dan diverifikasi di versi 1.3.1. Temuan paling serius adalah **eskalasi hak akses** (user manager non-superadmin bisa mengangkat dirinya jadi superadmin) dan **beberapa CVE dependensi** pada toolchain Go serta library frontend.

Setelah perbaikan, uji black-box pada server berjalan mengonfirmasi: secret entrance, isolasi backend ke loopback, CSRF, rate-limit/lockout login, header keamanan + CSP, serta proteksi autentikasi/otorisasi semuanya aktif.

**Status: siap go-public** dengan catatan operasional di bagian akhir.

---

## Temuan & Perbaikan

### KRITIS / TINGGI

#### H-1 — Eskalasi hak akses: non-superadmin bisa membuat/menjadi superadmin
- **Lokasi:** `modules/users/service.go`, `modules/roles/service.go`
- **Masalah:** User dengan izin `users:create/edit` atau `roles:edit` bisa (a) menugaskan role superadmin ke akun mana pun termasuk dirinya, (b) membuat/mengangkat role jadi superadmin, (c) mengubah password/email akun superadmin (pengambilalihan akun), (d) mengedit permission role yang ia pegang sendiri untuk memperluas aksesnya. Role superadmin melewati semua pengecekan izin, sehingga ini setara akses root penuh.
- **Dampak:** Privilege escalation → root pada host.
- **Perbaikan:** Service kini memerlukan `ActorIsSuperadmin` untuk menyentuh apa pun yang bersifat superadmin: menugaskan role superadmin, membuat/mengangkat role superadmin, dan mengedit akun superadmin. `UpdatePermissions` menolak mengubah role superadmin dan role milik aktor sendiri (`ErrOwnRole`).
- **Verifikasi:** `modules/users/escalation_test.go`, `modules/roles/escalation_test.go`.

#### H-2 — Upload gambar memakai ekstensi dari nama file klien (stored XSS)
- **Lokasi:** `storage/local.go`
- **Masalah:** Avatar/logo/favicon disimpan dengan ekstensi dari `header.Filename`. Byte yang lolos sniff sebagai gambar namun dinamai `x.html`/`x.js` akan tersimpan dan disajikan `/uploads` sebagai HTML/skrip. Directory `/uploads` juga disajikan tanpa header pembatas.
- **Dampak:** Stored XSS / eksekusi konten di origin panel.
- **Perbaikan:** Ekstensi file ditentukan dari MIME hasil sniff (`extForMime`), bukan nama klien. `/uploads` kini mengirim `Content-Security-Policy: default-src 'none'; … sandbox` + `X-Content-Type-Options: nosniff`.
- **Verifikasi:** `storage/local_test.go` (`TestUploadIgnoresClientExtension`).

#### H-3 — Kerentanan dependensi (CVE) backend & frontend
- **Backend:** toolchain Go 1.26/1.27.1 (CVE di `net/http`, `crypto/tls`, `crypto/x509`, `html/template`, `mime/multipart`), `golang.org/x/net`, `golang.org/x/text`, `excelize` (panic parsing), dan paket **`x/crypto/openpgp` yang sudah tidak dirawat**.
- **Frontend:** `axios` (12 advisory termasuk SSRF/prototype-pollution), `follow-redirects`, `zod` (DoS memori), `react-router` (open-redirect).
- **Perbaikan:** Go di-bump ke 1.27.3 (`go.mod` + `Dockerfile` ke `golang:1.27-alpine`); `x/net`, `x/text`, `x/crypto`, `excelize` di-upgrade; OpenPGP dipindah ke `github.com/ProtonMail/go-crypto`. Frontend: `axios`, `follow-redirects`, `zod@4`, `@hookform/resolvers@5` di-upgrade.
- **Verifikasi:** `govulncheck ./...` → **0 kerentanan terjangkau**; `npm audit --omit=dev` → sisa hanya `react-router` moderate (lihat M-6). Tes regresi resolver: `webadmin/src/utils/validation.resolver.test.js`.

#### H-4 — File Manager menjangkau direktori sensitif & default root terlalu luas
- **Lokasi:** `modules/filemanager/`, `config/config.go`
- **Masalah:** Default `FM_ROOTS=/www,/home` memberi user non-superadmin akses ke home seluruh user (termasuk `~/.ssh/authorized_keys`). Direktori milik modul lain (skrip cron yang jalan sebagai root, file compose, dump database) berada di dalam root dan bisa dibaca/ditulis.
- **Dampak:** User terbatas bisa menanam authorized_keys atau skrip cron → eskalasi ke root.
- **Perbaikan:** Default root diperketat ke `/www/wwwroot`. `Access.Resolve` menolak (untuk non-superadmin) direktori `.ssh`/`.gnupg` dan direktori terlarang (cron, compose, backup, private storage), termasuk via symlink (`system.RealPath`).
- **Verifikasi:** `modules/filemanager/handler_test.go` (`TestRestrictedUsersCannotReachSensitiveDirs`).

### SEDANG

#### M-1 — Database & role panel bisa diambil alih via modul Databases
- **Lokasi:** `modules/databases/`
- **Masalah:** Database panel sendiri (berisi hash password, secret 2FA, mirror sesi) bisa di-browse/di-query/di-backup/di-drop jika server lokal terdaftar.
- **Perbaikan:** `ProtectPanelDatabase(cfg.DBName, cfg.DBUser)` menandai DB & role panel sebagai objek sistem (disembunyikan dari listing, ditolak untuk browse/SQL/backup/drop/grant).
- **Verifikasi:** `modules/databases/browser_test.go` (`TestPanelDatabaseIsProtected`).

#### M-2 — Token sesi & reset password disimpan plaintext di DB
- **Lokasi:** `modules/auth/`, `session/session.go`, migrasi
- **Masalah:** Mirror sesi (tabel `sessions`) dan token reset menyimpan token mentah. Kebocoran dump DB = pengambilalihan sesi langsung.
- **Perbaikan:** Token di-hash SHA-256 sebelum disimpan (`hashToken`). Migrasi sekali jalan membersihkan baris lama. Redis tetap sumber kebenaran, jadi tidak ada user yang ter-logout.

#### M-3 — Ganti password tidak mencabut sesi lain + timing oracle pada login
- **Lokasi:** `modules/auth/service.go`
- **Masalah:** `ChangePassword` tidak mencabut sesi lain (sesi curian tetap hidup setelah korban ganti password). Login juga membalas lebih cepat untuk email tak dikenal (user enumeration via timing).
- **Perbaikan:** `ChangePassword` mencabut semua sesi lain kecuali milik pemanggil (`DeleteOthersForUser`). Login menjalankan `bcrypt` dummy saat email tak ada agar waktu respons seragam.

#### M-4 — Endpoint lupa/reset password aktif di production tanpa pengiriman email
- **Lokasi:** `modules/auth/routes.go`
- **Masalah:** Email tidak pernah dikirim (token hanya dicetak ke log), jadi endpoint ini hanya menambah attack surface.
- **Perbaikan:** Route `forgot-password`/`reset-password` hanya didaftarkan di non-production; reset juga diberi rate-limit. Admin yang terkunci memakai `rpanel set-admin` di server.

#### M-5 — Endpoint Settings menerima key/nilai sembarang
- **Lokasi:** `modules/settings/service.go`
- **Masalah:** `BulkUpdate` menerima key apa pun; `app_logo`/`app_favicon` bisa diarahkan ke URL eksternal, `primary_color` bisa diisi nilai sembarang (injeksi ke CSS variable).
- **Perbaikan:** Whitelist key + validasi nilai (`primary_color` wajib hex `#rrggbb`, `app_name` dibatasi panjang & tanpa `<>`/newline, path file hanya boleh dikosongkan — diset hanya lewat Upload).

#### M-6 — SPA frontend tanpa Content-Security-Policy
- **Lokasi:** `deploy/nginx-panel.conf.tmpl`, `webadmin/nginx.conf`, `deploy/install.sh`
- **Masalah:** HTML SPA disajikan tanpa CSP; defence-in-depth terhadap XSS tidak ada.
- **Perbaikan:** CSP ketat ditambahkan (`default-src 'self'`, `object-src 'none'`, `frame-ancestors 'none'`, hanya mengizinkan Google Fonts). Installer menambal config nginx instalasi lama saat upgrade.
- **Catatan tersisa:** `npm audit` masih melaporkan `react-router` (moderate, open-redirect via `<Link>`/`useNavigate`). RPanel tidak memakai SSR dan tidak membangun link dari input tak-tepercaya; risiko praktis rendah. Upgrade mayor React Router dijadwalkan terpisah.

### RENDAH / INFORMASI

- **L-1 — Validasi status user:** `UpdateStatus`/`Create`/`Update` kini memvalidasi status ke `active/inactive/suspended` dan menolak password < 8 karakter di sisi server (sebelumnya hanya divalidasi di frontend).
- **L-2 — Permission hardcoded distable:** `/uploads` kini pakai grup middleware, menjaga file statis tidak mewarisi perilaku SPA.
- **L-3 — `gosec` G304/G301/G306:** Path file berasal dari input tervalidasi (`ResolvePath`/`ValidName`) dan permission file wajar untuk panel yang mengelola web server; dianggap acceptable, bukan celah.
- **L-4 — Redis tanpa password:** Installer sudah mewajibkan `--requirepass` (dicek di `install.sh`); tidak ada regresi.

---

## Verifikasi Dinamis (black-box, VM Ubuntu 24.04 bersih)

Dipasang via `install-online.sh` lalu di-upgrade ke 1.3.1. Hasil yang dikonfirmasi langsung terhadap server berjalan:

| Kontrol | Hasil |
|---|---|
| Secret entrance (tanpa cookie) | semua route → **404** |
| Backend bind | hanya `127.0.0.1:8800` (tidak terekspos) |
| Endpoint sensitif tanpa sesi | `auth/me`, `users`, `file-manager`, `databases`, `settings` → **401** |
| Rate-limit/lockout login | 5× gagal → 401, lalu → **429** |
| CSRF (POST tanpa token) | **403** |
| forgot-password di production | **404** (dinonaktifkan) |
| Header SPA | CSP + X-Frame-Options DENY + nosniff + HSTS + Referrer-Policy |
| `/uploads` | CSP `default-src 'none'; sandbox` + nosniff |
| Cookie sesi | `HttpOnly`, `Secure`, `SameSite=Strict` |
| Login superadmin + otorisasi | sukses; superadmin unrestricted sesuai desain |

Analisa statis akhir: `govulncheck` → **0 terjangkau**; seluruh tes Go & 18 tes frontend **lolos**.

---

## Rekomendasi Operasional (saat go-public)

1. **Aktifkan 2FA** untuk setiap akun administrator segera setelah login pertama.
2. **Batasi `PANEL_ALLOWED_IPS`** ke IP tetap bila memungkinkan.
3. **Pasang sertifikat TLS asli** (default self-signed) untuk domain panel.
4. **Backup `SECRET_KEY`** (`/opt/rpanel/.env`) — kehilangannya menginvalidasi password DB tersimpan & secret 2FA.
5. **Beri izin Terminal / File Manager / Docker / Cron / Databases (edit)** hanya kepada orang yang Anda percayai setara root.
6. **Jangan memasang di server yang sudah memakai panel lain** (aaPanel/BT): keduanya mengelola nginx, firewall, dan cron.
7. **Jalankan `rpanel-update` secara berkala** untuk menerima perbaikan keamanan berikutnya.

---

<p align="center">Security review by Claude Code · Develop by <a href="https://rinoaprisedo.com">rinoaprisedo.com</a></p>
