package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"rpanel/backend/modules/users"
	"rpanel/backend/secretbox"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

var (
	ErrTwoFactorRequired   = errors.New("two-factor code required")
	ErrInvalidOTP          = errors.New("invalid two-factor code")
	ErrTwoFactorEnabled    = errors.New("two-factor authentication is already enabled; disable it first")
	ErrTwoFactorNotStarted = errors.New("start the setup first")
	ErrTwoFactorNoSecret   = errors.New("two-factor authentication needs SECRET_KEY to be configured on the server")

	totpRe     = regexp.MustCompile(`^[0-9]{6}$`)
	recoveryRe = regexp.MustCompile(`^[a-z0-9]{10}$`)
)

const (
	totpPeriod    = 30
	recoveryCount = 8
)

// ReplayGuard remembers used one-time codes so a captured code cannot be reused.
type ReplayGuard interface {
	// MarkUsed returns true if key was not seen before (and records it).
	MarkUsed(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type redisGuard struct{ rdb *redis.Client }

func (g redisGuard) MarkUsed(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return g.rdb.SetNX(ctx, key, 1, ttl).Result()
}

// TwoFactor implements TOTP enrolment and verification.
type TwoFactor struct {
	db     *gorm.DB
	box    *secretbox.Cipher // nil when SECRET_KEY is unset
	guard  ReplayGuard
	issuer string
	now    func() time.Time
}

func NewTwoFactor(db *gorm.DB, rdb *redis.Client, secret, issuer string) *TwoFactor {
	box, _ := secretbox.NewCipher(secret)
	return &TwoFactor{db: db, box: box, guard: redisGuard{rdb}, issuer: issuer, now: time.Now}
}

func (t *TwoFactor) need() error {
	if t.box == nil {
		return ErrTwoFactorNoSecret
	}
	return nil
}

type Enrolment struct {
	Secret    string `json:"secret"`
	URL       string `json:"otpauth_url"`
	QRDataURL string `json:"qr"`
}

// Begin generates a fresh secret and stores it (not yet active).
func (t *TwoFactor) Begin(u *users.User) (*Enrolment, error) {
	if err := t.need(); err != nil {
		return nil, err
	}
	if u.TOTPEnabled {
		return nil, ErrTwoFactorEnabled
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: t.issuer, AccountName: u.Email, Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return nil, err
	}
	enc, err := t.box.Encrypt(key.Secret())
	if err != nil {
		return nil, err
	}
	if err := t.db.Model(u).Updates(map[string]any{"totp_secret_enc": enc, "totp_enabled": false, "recovery_codes": ""}).Error; err != nil {
		return nil, err
	}
	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, err
	}
	return &Enrolment{Secret: key.Secret(), URL: key.URL(), QRDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, nil
}

func hashCode(c string) string {
	h := sha256.Sum256([]byte(c))
	return hex.EncodeToString(h[:])
}

func normalizeRecovery(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func newRecoveryCodes() (plain []string, hashed []string, err error) {
	for i := 0; i < recoveryCount; i++ {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		raw := hex.EncodeToString(b) // 10 chars
		plain = append(plain, raw[:5]+"-"+raw[5:])
		hashed = append(hashed, hashCode(raw))
	}
	return plain, hashed, nil
}

// checkTOTP validates a 6-digit code within ±1 step and rejects replays.
func (t *TwoFactor) checkTOTP(ctx context.Context, u *users.User, code string) (bool, error) {
	secret, err := t.box.Decrypt(u.TOTPSecretEnc)
	if err != nil {
		return false, err
	}
	now := t.now()
	for off := int64(-1); off <= 1; off++ {
		step := now.Unix()/totpPeriod + off
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*totpPeriod, 0), totp.ValidateOpts{Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err != nil {
			return false, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			fresh, err := t.guard.MarkUsed(ctx, fmt.Sprintf("totp:used:%d:%d", u.ID, step), 3*time.Minute)
			if err != nil {
				return false, err
			}
			return fresh, nil
		}
	}
	return false, nil
}

// checkRecovery consumes a recovery code if it matches.
func (t *TwoFactor) checkRecovery(u *users.User, code string) (bool, error) {
	code = normalizeRecovery(code)
	if !recoveryRe.MatchString(code) || u.RecoveryCodes == "" {
		return false, nil
	}
	var hashes []string
	if err := json.Unmarshal([]byte(u.RecoveryCodes), &hashes); err != nil {
		return false, err
	}
	want := hashCode(code)
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			rest := append(append([]string{}, hashes[:i]...), hashes[i+1:]...)
			b, _ := json.Marshal(rest)
			u.RecoveryCodes = string(b)
			return true, t.db.Model(u).Update("recovery_codes", u.RecoveryCodes).Error
		}
	}
	return false, nil
}

// Verify accepts a TOTP code or an unused recovery code for an enrolled user.
func (t *TwoFactor) Verify(ctx context.Context, u *users.User, code string) (bool, error) {
	if err := t.need(); err != nil {
		return false, err
	}
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if totpRe.MatchString(code) {
		return t.checkTOTP(ctx, u, code)
	}
	return t.checkRecovery(u, code)
}

// Enable activates 2FA after the user proves they can generate a code, and
// returns the one-time recovery codes (shown only once).
func (t *TwoFactor) Enable(ctx context.Context, u *users.User, code string) ([]string, error) {
	if err := t.need(); err != nil {
		return nil, err
	}
	if u.TOTPEnabled {
		return nil, ErrTwoFactorEnabled
	}
	if u.TOTPSecretEnc == "" {
		return nil, ErrTwoFactorNotStarted
	}
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if !totpRe.MatchString(code) {
		return nil, ErrInvalidOTP
	}
	if ok, err := t.checkTOTP(ctx, u, code); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, ErrInvalidOTP
	}
	plain, hashed, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(hashed)
	if err := t.db.Model(u).Updates(map[string]any{"totp_enabled": true, "recovery_codes": string(b)}).Error; err != nil {
		return nil, err
	}
	u.TOTPEnabled, u.RecoveryCodes = true, string(b)
	return plain, nil
}

// Disable turns 2FA off (caller has already verified the password).
func (t *TwoFactor) Disable(ctx context.Context, u *users.User, code string) error {
	if !u.TOTPEnabled {
		return nil
	}
	ok, err := t.Verify(ctx, u, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidOTP
	}
	return t.Reset(u)
}

// Reset clears all 2FA data (also used by the disable-2fa CLI).
func (t *TwoFactor) Reset(u *users.User) error {
	u.TOTPEnabled, u.TOTPSecretEnc, u.RecoveryCodes = false, "", ""
	return t.db.Model(u).Updates(map[string]any{"totp_enabled": false, "totp_secret_enc": "", "recovery_codes": ""}).Error
}

// RegenerateRecovery issues a fresh set of recovery codes after a valid code.
func (t *TwoFactor) RegenerateRecovery(ctx context.Context, u *users.User, code string) ([]string, error) {
	if !u.TOTPEnabled {
		return nil, ErrTwoFactorNotStarted
	}
	ok, err := t.Verify(ctx, u, code)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidOTP
	}
	plain, hashed, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(hashed)
	u.RecoveryCodes = string(b)
	return plain, t.db.Model(u).Update("recovery_codes", u.RecoveryCodes).Error
}
