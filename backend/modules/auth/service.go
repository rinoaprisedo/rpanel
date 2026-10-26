package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"rpanel/backend/config"
	"rpanel/backend/modules/users"
	"rpanel/backend/session"

	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountInactive    = errors.New("account is not active")
	ErrSessionFailed      = errors.New("failed to create session")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidResetToken  = errors.New("invalid or already used token")
	ErrResetTokenExpired  = errors.New("token has expired")
	ErrWrongPassword      = errors.New("current password is incorrect")
	ErrPasswordReused     = errors.New("cannot reuse any of your last 5 passwords")
)

type PermissionDTO struct {
	Path      string `json:"path"`
	CanView   bool   `json:"can_view"`
	CanCreate bool   `json:"can_create"`
	CanEdit   bool   `json:"can_edit"`
	CanDelete bool   `json:"can_delete"`
}

// Service holds business rules for authentication: credential checks,
// session lifecycle, password policy, and reset-token handling.
type Service struct {
	repo  *Repository
	redis *redis.Client
	cfg   *config.Config
	TwoFA *TwoFactor
}

func NewService(repo *Repository, rdb *redis.Client, cfg *config.Config) *Service {
	return &Service{repo: repo, redis: rdb, cfg: cfg, TwoFA: NewTwoFactor(repo.DB, rdb, cfg.DBSecretKey, "RPanel")}
}

type LoginResult struct {
	User  *users.User
	Token string
}

func (s *Service) Login(email, password, otp, ip, userAgent string) (*LoginResult, error) {
	user, err := s.repo.FindUserByEmail(email)
	if err != nil {
		// Same bcrypt cost as a real check, so response time does not reveal
		// whether the e-mail address has an account.
		CheckPassword(dummyHash, password)
		return nil, ErrInvalidCredentials
	}
	if !CheckPassword(user.Password, password) {
		return nil, ErrInvalidCredentials
	}
	if user.Status != users.StatusActive {
		return nil, ErrAccountInactive
	}
	if user.TOTPEnabled {
		if otp == "" {
			return nil, ErrTwoFactorRequired
		}
		ok, err := s.TwoFA.Verify(context.Background(), user, otp)
		if err != nil {
			log.Printf("2FA verification error for user %d: %v", user.ID, err)
			return nil, ErrInvalidOTP
		}
		if !ok {
			return nil, ErrInvalidOTP
		}
	}

	token, err := session.GenerateToken()
	if err != nil {
		return nil, ErrSessionFailed
	}

	data := session.Data{
		UserID:    user.ID,
		UserUUID:  user.UUID.String(),
		IPAddress: ip,
		UserAgent: userAgent,
	}
	ttl := time.Duration(s.cfg.SessionMaxAge) * time.Second
	if err := session.Create(context.Background(), s.redis, token, data, ttl, s.cfg.SessionMaxConcurrent); err != nil {
		return nil, ErrSessionFailed
	}

	// Mirror the Redis session as a DB row so /users/:uuid/sessions can list
	// and revoke active sessions; Redis remains the source of truth for auth.
	if err := s.repo.CreateSession(&users.Session{
		UserID:    user.ID,
		Token:     hashToken(token),
		IPAddress: ip,
		UserAgent: userAgent,
		ExpiredAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}); err != nil {
		log.Printf("CreateSession DB mirror failed for user %d: %v", user.ID, err)
	}

	s.repo.UpdateLastLogin(user, time.Now())

	return &LoginResult{User: user, Token: token}, nil
}

func (s *Service) Logout(token string, userID *uint64) {
	if token == "" || userID == nil {
		return
	}
	_ = session.Delete(context.Background(), s.redis, token, *userID)
	s.repo.DeleteSessionByToken(hashToken(token))
}

func (s *Service) Me(userID uint64) (*users.User, error) {
	user, err := s.repo.FindUserByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// ProfileDTO builds the role/permissions view returned by Login and Me.
func (s *Service) ProfileDTO(u *users.User) (roleNames []string, isSuperadmin bool, perms []PermissionDTO) {
	roleNames = []string{}
	if u.Role == nil {
		return roleNames, false, []PermissionDTO{}
	}
	roleNames = append(roleNames, u.Role.Name)
	isSuperadmin = u.Role.IsSuperadmin
	if isSuperadmin {
		return roleNames, isSuperadmin, []PermissionDTO{}
	}
	return roleNames, isSuperadmin, s.repo.RolePermissionsFor([]uint64{u.Role.ID})
}

func (s *Service) ForgotPassword(email string) {
	if _, err := s.repo.FindUserByEmail(email); err != nil {
		// Do not leak whether the email exists; caller always returns a generic success message.
		return
	}

	token, _ := session.GenerateToken()
	reset := users.PasswordResetToken{
		Email:     email,
		Token:     hashToken(token),
		ExpiredAt: time.Now().Add(1 * time.Hour),
	}
	_ = s.repo.CreatePasswordResetToken(&reset)

	// SMTP is not configured for the base project; log instead of sending.
	logResetStub(email, token)
}

func (s *Service) ResetPassword(token, newPassword string) error {
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	reset, err := s.repo.FindValidResetToken(hashToken(token))
	if err != nil {
		return ErrInvalidResetToken
	}
	if time.Now().After(reset.ExpiredAt) {
		return ErrResetTokenExpired
	}

	user, err := s.repo.FindUserByEmail(reset.Email)
	if err != nil {
		return ErrUserNotFound
	}

	hashed, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	s.repo.UpdatePassword(user, hashed)
	s.repo.CreatePasswordHistory(&users.PasswordHistory{UserID: user.ID, Password: hashed, CreatedAt: time.Now()})
	s.repo.MarkResetTokenUsed(reset, time.Now())

	_ = session.DeleteAllForUser(context.Background(), s.redis, user.ID)
	return nil
}

// hashToken is what the database keeps of session and reset tokens: a leaked
// table (backup, SQL access) must not hand out live credentials.
func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// dummyHash is a bcrypt hash (cost 12) of a random string, compared against
// when the e-mail is unknown.
var dummyHash = func() string {
	h, _ := HashPassword(fmt.Sprintf("unused-%d", time.Now().UnixNano()))
	return h
}()

// ChangePassword also signs out every other session of the user (keepToken
// stays valid), so a stolen session does not survive a password change.
func (s *Service) ChangePassword(userID uint64, currentPassword, newPassword, keepToken string) error {
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	user, err := s.repo.FindUserByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if !CheckPassword(user.Password, currentPassword) {
		return ErrWrongPassword
	}

	for _, hist := range s.repo.RecentPasswordHistory(user.ID, 5) {
		if CheckPassword(hist.Password, newPassword) {
			return ErrPasswordReused
		}
	}
	if CheckPassword(user.Password, newPassword) {
		return ErrPasswordReused
	}

	hashed, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	s.repo.UpdatePassword(user, hashed)
	s.repo.CreatePasswordHistory(&users.PasswordHistory{UserID: user.ID, Password: hashed, CreatedAt: time.Now()})
	_ = session.DeleteOthersForUser(context.Background(), s.redis, user.ID, keepToken)
	s.repo.DB.Where("user_id = ? AND token <> ?", user.ID, hashToken(keepToken)).Delete(&users.Session{})
	return nil
}

func logResetStub(email, token string) {
	println("password reset requested for", email, "token:", token)
}
