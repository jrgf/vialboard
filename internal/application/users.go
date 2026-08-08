package application

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/jrgf/vialboard/internal/domain"
)

var (
	ErrUsernameTaken      = errors.New("username is already taken")
	ErrUserNotFound       = errors.New("user was not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("current password is incorrect")
	ErrInvalidSession     = errors.New("invalid session")
	ErrUserManagesTeams   = errors.New("user manages one or more teams")
)

const (
	sessionLifetime   = 24 * time.Hour
	dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

type UserRepository interface {
	Create(context.Context, domain.User) (domain.User, error)
	List(context.Context, UserListOptions) ([]domain.User, int64, error)
	ListActive(context.Context, MemberListOptions) ([]domain.User, error)
	FindByID(context.Context, string) (domain.User, error)
	FindByUsername(context.Context, string) (domain.User, error)
	Update(context.Context, domain.User) (domain.User, error)
	SetTeam(context.Context, string, *string, *string) (domain.User, error)
	UpdatePassword(context.Context, string, string, time.Time) error
	ReplaceSession(context.Context, domain.Session) error
	FindActiveUserBySession(context.Context, string, time.Time) (domain.User, error)
	RevokeSession(context.Context, string, time.Time) error
}

type UserListOptions struct {
	Limit  int
	Offset int
}

type MemberListOptions struct {
	TeamID     *string
	ManagedBy  *string
	Unassigned bool
	Role       domain.Role
}

type UserPage struct {
	Users      []domain.User
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) bool
}

type UserService struct {
	users     UserRepository
	passwords PasswordHasher
}

func NewUserService(users UserRepository, passwords PasswordHasher) *UserService {
	return &UserService{users: users, passwords: passwords}
}

func (s *UserService) Create(ctx context.Context, username, password, role string) (domain.User, error) {
	return s.create(ctx, username, password, role, nil)
}

func (s *UserService) CreateInTeam(ctx context.Context, username, password, teamID string) (domain.User, error) {
	if !domain.ValidTeamID(teamID) {
		return domain.User{}, domain.NewValidationError("invalidTeamId", "Team ID must be a UUID")
	}
	return s.create(ctx, username, password, string(domain.RoleViewer), &teamID)
}

func (s *UserService) create(ctx context.Context, username, password, role string, teamID *string) (domain.User, error) {
	if _, err := domain.ValidateUsername(username); err != nil {
		return domain.User{}, err
	}
	if err := validatePassword(password); err != nil {
		return domain.User{}, err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return domain.User{}, err
	}
	user, err := domain.NewUser(username, hash, role)
	if err != nil {
		return domain.User{}, err
	}
	user.TeamID = teamID
	return s.users.Create(ctx, user)
}

func (s *UserService) ChangePassword(ctx context.Context, actor domain.User, currentPassword, newPassword string) error {
	if !s.passwords.Verify(actor.PasswordHash, currentPassword) {
		return ErrInvalidPassword
	}
	return s.setPassword(ctx, actor.ID, newPassword)
}

func (s *UserService) ResetPassword(ctx context.Context, userID, newPassword string) error {
	if _, err := s.users.FindByID(ctx, userID); err != nil {
		return err
	}
	return s.setPassword(ctx, userID, newPassword)
}

func (s *UserService) setPassword(ctx context.Context, userID, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, hash, time.Now())
}

func validatePassword(password string) error {
	passwordLength := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || passwordLength < 8 || passwordLength > 250 {
		return domain.NewValidationError("invalidPassword", "Password must be between 8 and 250 characters")
	}
	return nil
}

func (s *UserService) Register(ctx context.Context, username, password string) (domain.User, domain.Session, error) {
	user, err := s.Create(ctx, username, password, string(domain.RoleViewer))
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	session, err := s.startSession(ctx, user)
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	return user, session, nil
}

func (s *UserService) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, ErrInvalidCredentials) {
		return domain.User{}, ErrUserNotFound
	}
	return user, err
}

func (s *UserService) GetByID(ctx context.Context, id string) (domain.User, error) {
	return s.users.FindByID(ctx, id)
}

func (s *UserService) List(ctx context.Context, page, pageSize int) (UserPage, error) {
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return UserPage{}, domain.NewValidationError("invalidPage", "Page must be a positive integer")
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize < 1 || pageSize > 100 {
		return UserPage{}, domain.NewValidationError("invalidPageSize", "Page size must be between 1 and 100")
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/pageSize {
		return UserPage{}, domain.NewValidationError("invalidPage", "Page is too large")
	}
	users, total, err := s.users.List(ctx, UserListOptions{Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		return UserPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	lastPage := totalPages
	if lastPage == 0 {
		lastPage = 1
	}
	if page > lastPage {
		return UserPage{}, domain.NewValidationError("pageOutOfRange", "Page cannot exceed total pages")
	}
	return UserPage{Users: users, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *UserService) Members(ctx context.Context, actor domain.User) ([]domain.User, error) {
	switch actor.Role {
	case domain.RoleAdmin:
		return s.users.ListActive(ctx, MemberListOptions{})
	case domain.RoleManager:
		return s.users.ListActive(ctx, MemberListOptions{ManagedBy: &actor.ID})
	default:
		if actor.TeamID == nil {
			return []domain.User{}, nil
		}
		return s.users.ListActive(ctx, MemberListOptions{TeamID: actor.TeamID})
	}
}

func (s *UserService) UpdateAccess(ctx context.Context, id string, role *string, active *bool) (domain.User, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if err := user.ApplyAccessUpdate(role, active); err != nil {
		return domain.User{}, err
	}
	return s.users.Update(ctx, user)
}

func (s *UserService) Login(ctx context.Context, username, password string) (domain.User, domain.Session, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			s.passwords.Verify(dummyPasswordHash, password)
			return domain.User{}, domain.Session{}, ErrInvalidCredentials
		}
		return domain.User{}, domain.Session{}, err
	}
	if !s.passwords.Verify(user.PasswordHash, password) || !user.Active {
		return domain.User{}, domain.Session{}, ErrInvalidCredentials
	}
	session, err := s.startSession(ctx, user)
	if err != nil {
		return domain.User{}, domain.Session{}, err
	}
	return user, session, nil
}

func (s *UserService) startSession(ctx context.Context, user domain.User) (domain.Session, error) {
	session, err := domain.NewSession(user.ID, sessionLifetime, time.Now())
	if err != nil {
		return domain.Session{}, err
	}
	if err := s.users.ReplaceSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *UserService) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, ErrInvalidSession
	}
	return s.users.FindActiveUserBySession(ctx, domain.SessionID(token), time.Now())
}

func (s *UserService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSession
	}
	return s.users.RevokeSession(ctx, sessionID, time.Now())
}
