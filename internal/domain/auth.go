package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

type Role string

const (
	RoleViewer  Role = "viewer"
	RoleManager Role = "manager"
	RoleAdmin   Role = "admin"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,15}$`)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	TeamID       *string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	ID        string
	Token     string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func NewUser(username, passwordHash, role string) (User, error) {
	username, err := ValidateUsername(username)
	if err != nil {
		return User{}, err
	}
	if passwordHash == "" {
		return User{}, NewValidationError("invalidPasswordHash", "Password hash is required")
	}
	if strings.TrimSpace(role) == "" {
		role = string(RoleViewer)
	}
	userRole, err := ParseRole(role)
	if err != nil {
		return User{}, err
	}
	id, err := NewUserID()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, PasswordHash: passwordHash, Role: userRole, Active: true}, nil
}

func ValidateUsername(value string) (string, error) {
	username := strings.TrimSpace(value)
	if !usernamePattern.MatchString(username) {
		return "", NewValidationError("invalidUsername", "Username must be 1-16 characters, start with a letter or number, and contain only letters, numbers, underscores, or hyphens")
	}
	return username, nil
}

func ParseRole(value string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(value)))
	if role == "worker" {
		role = RoleViewer
	}
	if role != RoleViewer && role != RoleManager && role != RoleAdmin {
		return "", NewValidationError("invalidRole", "Role must be viewer (worker), manager, or admin")
	}
	return role, nil
}

func (u *User) ApplyAccessUpdate(role *string, active *bool) error {
	if role == nil && active == nil {
		return NewValidationError("emptyUpdate", "At least one field is required")
	}
	if role != nil {
		parsed, err := ParseRole(*role)
		if err != nil {
			return err
		}
		u.Role = parsed
		if parsed != RoleViewer {
			u.TeamID = nil
		}
	}
	if active != nil {
		u.Active = *active
	}
	return nil
}

func ValidUserID(value string) bool {
	return validUUID(value)
}

func NewUserID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func NewSession(userID string, lifetime time.Duration, now time.Time) (Session, error) {
	if !validUUID(userID) {
		return Session{}, NewValidationError("invalidUserId", "User ID must be a UUID")
	}
	if lifetime <= 0 {
		return Session{}, NewValidationError("invalidSessionLifetime", "Session lifetime must be positive")
	}

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return Session{}, err
	}
	now = now.UTC()
	token := hex.EncodeToString(random)
	return Session{
		ID:        SessionID(token),
		Token:     token,
		UserID:    userID,
		ExpiresAt: now.Add(lifetime),
		CreatedAt: now,
	}, nil
}

func SessionID(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

func (s *Session) Revoke(now time.Time) {
	now = now.UTC()
	s.RevokedAt = &now
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}
