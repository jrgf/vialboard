package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRepository struct {
	db *gorm.DB
}

type userRecord struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	Username     string
	PasswordHash string
	Role         domain.Role
	TeamID       *string `gorm:"type:uuid"`
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type sessionRecord struct {
	ID        string `gorm:"type:char(64);primaryKey"`
	UserID    string `gorm:"type:uuid"`
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (sessionRecord) TableName() string {
	return "sessions"
}

func (userRecord) TableName() string {
	return "users"
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	record := userRecord{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
		TeamID:       user.TeamID,
		Active:       user.Active,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return domain.User{}, application.ErrUsernameTaken
		}
		return domain.User{}, err
	}
	return toUser(record), nil
}

func (r *UserRepository) List(ctx context.Context, options application.UserListOptions) ([]domain.User, int64, error) {
	query := r.db.WithContext(ctx).Model(&userRecord{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := make([]userRecord, 0)
	if err := query.Order("created_at DESC, id DESC").Limit(options.Limit).Offset(options.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	users := make([]domain.User, len(records))
	for index, record := range records {
		users[index] = toUser(record)
	}
	return users, total, nil
}

func (r *UserRepository) ListActive(ctx context.Context, options application.MemberListOptions) ([]domain.User, error) {
	records := make([]userRecord, 0)
	role := options.Role
	if role == "" {
		role = domain.RoleViewer
	}
	query := r.db.WithContext(ctx).Where("active AND role = ?", role)
	if options.TeamID != nil {
		query = query.Where("team_id = ?", *options.TeamID)
	}
	if options.ManagedBy != nil {
		query = query.Where("team_id IN (SELECT id FROM teams WHERE manager_id = ?)", *options.ManagedBy)
	}
	if options.Unassigned {
		query = query.Where("team_id IS NULL")
	}
	if err := query.Order("lower(username), id").Find(&records).Error; err != nil {
		return nil, err
	}
	users := make([]domain.User, len(records))
	for index, record := range records {
		users[index] = toUser(record)
	}
	return users, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (domain.User, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, application.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return toUser(record), nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).Where("lower(username) = lower(?)", strings.TrimSpace(username)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, application.ErrInvalidCredentials
		}
		return domain.User{}, err
	}
	return toUser(record), nil
}

func (r *UserRepository) Update(ctx context.Context, user domain.User) (domain.User, error) {
	record := userRecord{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
		TeamID:       user.TeamID,
		Active:       user.Active,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current userRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "team_id").First(&current, "id = ?", record.ID).Error; err != nil {
			return err
		}
		if current.TeamID != nil && !sameNullableString(current.TeamID, record.TeamID) {
			if err := tx.Model(&issueRecord{}).
				Where("team_id = ? AND assignee_id = ?", *current.TeamID, record.ID).
				Update("assignee_id", nil).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		if !record.Active {
			return tx.Model(&sessionRecord{}).
				Where("user_id = ? AND revoked_at IS NULL", record.ID).
				Update("revoked_at", gorm.Expr("GREATEST(created_at, CURRENT_TIMESTAMP)")).Error
		}
		return nil
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.ConstraintName == "users_managed_teams_check" {
			return domain.User{}, application.ErrUserManagesTeams
		}
		return domain.User{}, err
	}
	return toUser(record), nil
}

func (r *UserRepository) SetTeam(ctx context.Context, userID string, expectedTeamID, teamID *string) (domain.User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current userRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "role", "team_id").First(&current, "id = ? AND role = ?", userID, domain.RoleViewer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrUserNotFound
			}
			return err
		}
		if !sameNullableString(current.TeamID, expectedTeamID) {
			return application.ErrUserHasTeam
		}
		if current.TeamID != nil && (teamID == nil || *current.TeamID != *teamID) {
			if err := tx.Model(&issueRecord{}).
				Where("team_id = ? AND assignee_id = ?", *current.TeamID, userID).
				Update("assignee_id", nil).Error; err != nil {
				return err
			}
		}
		return tx.Model(&userRecord{}).Where("id = ?", userID).Update("team_id", teamID).Error
	})
	if err != nil {
		return domain.User{}, err
	}
	return r.FindByID(ctx, userID)
}

func sameNullableString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&userRecord{}).Where("id = ?", userID).Updates(map[string]any{
			"password_hash": passwordHash,
			"updated_at":    now.UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return application.ErrUserNotFound
		}
		return tx.Model(&sessionRecord{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Update("revoked_at", gorm.Expr("GREATEST(created_at, ?)", now.UTC())).Error
	})
}

func (r *UserRepository) ReplaceSession(ctx context.Context, session domain.Session) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "active").First(&user, "id = ?", session.UserID).Error; err != nil {
			return err
		}
		if !user.Active {
			return application.ErrInvalidCredentials
		}
		if err := tx.Model(&sessionRecord{}).
			Where("user_id = ? AND revoked_at IS NULL", session.UserID).
			Update("revoked_at", gorm.Expr("GREATEST(created_at, ?)", session.CreatedAt.UTC())).Error; err != nil {
			return err
		}
		return tx.Create(&sessionRecord{
			ID:        session.ID,
			UserID:    session.UserID,
			ExpiresAt: session.ExpiresAt,
			RevokedAt: session.RevokedAt,
			CreatedAt: session.CreatedAt,
		}).Error
	})
}

func (r *UserRepository) FindActiveUserBySession(ctx context.Context, sessionID string, now time.Time) (domain.User, error) {
	var record userRecord
	err := r.db.WithContext(ctx).
		Table("users").
		Select("users.*").
		Joins("JOIN sessions ON sessions.user_id = users.id").
		Where("sessions.id = ? AND sessions.revoked_at IS NULL AND sessions.expires_at > ? AND users.active", sessionID, now.UTC()).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, application.ErrInvalidSession
	}
	if err != nil {
		return domain.User{}, err
	}
	return toUser(record), nil
}

func (r *UserRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&sessionRecord{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Update("revoked_at", now.UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrInvalidSession
	}
	return nil
}

func toUser(record userRecord) domain.User {
	return domain.User{
		ID:           record.ID,
		Username:     record.Username,
		PasswordHash: record.PasswordHash,
		Role:         record.Role,
		TeamID:       record.TeamID,
		Active:       record.Active,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

var _ application.UserRepository = (*UserRepository)(nil)
