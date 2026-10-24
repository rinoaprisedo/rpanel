package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"rpanel/backend/modules/users"
	"rpanel/backend/secretbox"

	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

type memGuard struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (m *memGuard) MarkUsed(_ context.Context, key string, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen[key] {
		return false, nil
	}
	m.seen[key] = true
	return true, nil
}

func setup(t *testing.T, secret string) (*TwoFactor, *users.User, *time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE users (id integer primary key autoincrement, uuid text, name text, email text, password text, avatar text, status text, last_login_at datetime, totp_secret_enc text, totp_enabled boolean default false, recovery_codes text, role_id integer, created_by integer, updated_by integer, deleted_by integer, created_at datetime, updated_at datetime, deleted_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	u := &users.User{Name: "A", Email: "a@example.com", Password: "x", Status: users.StatusActive}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.NewCipher(secret)
	now := time.Date(2024, 5, 1, 12, 0, 10, 0, time.UTC)
	tf := &TwoFactor{db: db, box: box, guard: &memGuard{seen: map[string]bool{}}, issuer: "RPanel", now: func() time.Time { return now }}
	if secret == "" {
		tf.box = nil
	}
	return tf, u, &now
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRequiresSecretKey(t *testing.T) {
	tf, u, _ := setup(t, "")
	if _, err := tf.Begin(u); !errors.Is(err, ErrTwoFactorNoSecret) {
		t.Fatalf("want ErrTwoFactorNoSecret, got %v", err)
	}
}

func TestEnrolmentFlow(t *testing.T) {
	tf, u, now := setup(t, "k")
	ctx := context.Background()
	en, err := tf.Begin(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(en.URL, "otpauth://totp/") || !strings.Contains(en.URL, "RPanel") || !strings.HasPrefix(en.QRDataURL, "data:image/png;base64,") || en.Secret == "" {
		t.Fatalf("enrolment %+v", en)
	}
	if strings.Contains(u.TOTPSecretEnc, en.Secret) || u.TOTPSecretEnc == "" {
		t.Fatal("secret must be stored encrypted")
	}
	// not active before confirmation
	if u.TOTPEnabled {
		t.Fatal("must not be enabled before confirming a code")
	}
	if _, err := tf.Enable(ctx, u, "000000"); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("wrong code: %v", err)
	}
	if _, err := tf.Enable(ctx, u, "12ab56"); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("malformed code: %v", err)
	}
	codes, err := tf.Enable(ctx, u, codeAt(t, en.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != recoveryCount || !u.TOTPEnabled {
		t.Fatalf("codes=%v enabled=%v", codes, u.TOTPEnabled)
	}
	if strings.Contains(u.RecoveryCodes, strings.ReplaceAll(codes[0], "-", "")) {
		t.Fatal("recovery codes must be stored hashed")
	}
	if _, err := tf.Begin(u); !errors.Is(err, ErrTwoFactorEnabled) {
		t.Fatalf("re-begin while enabled: %v", err)
	}
	var fromDB users.User
	tf.db.First(&fromDB, u.ID)
	if !fromDB.TOTPEnabled {
		t.Fatal("not persisted")
	}
}

func enrolled(t *testing.T) (*TwoFactor, *users.User, string, *time.Time, []string) {
	t.Helper()
	tf, u, now := setup(t, "k")
	en, _ := tf.Begin(u)
	codes, err := tf.Enable(context.Background(), u, codeAt(t, en.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	// move to the next window so the enrolment code does not count as used
	n := now.Add(2 * time.Minute)
	*now = n
	return tf, u, en.Secret, now, codes
}

func TestVerifyWindowAndReplay(t *testing.T) {
	tf, u, secret, now, _ := enrolled(t)
	ctx := context.Background()
	cur := codeAt(t, secret, *now)
	if ok, err := tf.Verify(ctx, u, cur); err != nil || !ok {
		t.Fatalf("current code: %v %v", ok, err)
	}
	if ok, _ := tf.Verify(ctx, u, cur); ok {
		t.Fatal("replayed code must be rejected")
	}
	if ok, _ := tf.Verify(ctx, u, codeAt(t, secret, now.Add(-30*time.Second))); !ok {
		t.Fatal("previous step must be accepted (clock skew)")
	}
	if ok, _ := tf.Verify(ctx, u, codeAt(t, secret, now.Add(30*time.Second))); !ok {
		t.Fatal("next step must be accepted (clock skew)")
	}
	if ok, _ := tf.Verify(ctx, u, codeAt(t, secret, now.Add(-5*time.Minute))); ok {
		t.Fatal("old code must be rejected")
	}
	if ok, _ := tf.Verify(ctx, u, "000000"); ok {
		t.Fatal("garbage must be rejected")
	}
	if ok, _ := tf.Verify(ctx, u, " "+cur[:3]+" "+cur[3:]+" "); ok {
		t.Fatal("(already used) spaced code must still be a replay")
	}
}

func TestRecoveryCodesAreSingleUse(t *testing.T) {
	tf, u, _, _, codes := enrolled(t)
	ctx := context.Background()
	rc := strings.ToUpper(codes[0]) // case/dash insensitive
	if ok, err := tf.Verify(ctx, u, rc); err != nil || !ok {
		t.Fatalf("recovery code: %v %v", ok, err)
	}
	if ok, _ := tf.Verify(ctx, u, codes[0]); ok {
		t.Fatal("recovery code must be single use")
	}
	var fromDB users.User
	tf.db.First(&fromDB, u.ID)
	if ok, _ := tf.Verify(ctx, &fromDB, codes[0]); ok {
		t.Fatal("consumption must be persisted")
	}
	if ok, _ := tf.Verify(ctx, u, codes[1]); !ok {
		t.Fatal("other codes must keep working")
	}
	if ok, _ := tf.Verify(ctx, u, "zzzzz-zzzzz"); ok {
		t.Fatal("unknown code must fail")
	}
}

func TestDisableAndRegenerate(t *testing.T) {
	tf, u, secret, now, codes := enrolled(t)
	ctx := context.Background()
	if err := tf.Disable(ctx, u, "000000"); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("disable with bad code: %v", err)
	}
	if !u.TOTPEnabled {
		t.Fatal("must stay enabled")
	}
	fresh, err := tf.RegenerateRecovery(ctx, u, codeAt(t, secret, *now))
	if err != nil || len(fresh) != recoveryCount {
		t.Fatalf("regenerate: %v %v", fresh, err)
	}
	if ok, _ := tf.Verify(ctx, u, codes[0]); ok {
		t.Fatal("old recovery codes must be invalidated")
	}
	if err := tf.Disable(ctx, u, fresh[0]); err != nil {
		t.Fatalf("disable with recovery code: %v", err)
	}
	if u.TOTPEnabled || u.TOTPSecretEnc != "" || u.RecoveryCodes != "" {
		t.Fatalf("not cleared: %+v", u)
	}
}
