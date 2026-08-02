package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"gorm.io/gorm"
)

type TeamRepository struct {
	db *gorm.DB
}

type teamRecord struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	Name      string
	ManagerID string `gorm:"type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (teamRecord) TableName() string {
	return "teams"
}

func NewTeamRepository(db *gorm.DB) *TeamRepository {
	return &TeamRepository{db: db}
}

func (r *TeamRepository) Create(ctx context.Context, team domain.Team) (domain.Team, error) {
	record := teamRecord{ID: team.ID, Name: team.Name, ManagerID: team.ManagerID}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return domain.Team{}, application.ErrTeamNameTaken
		}
		return domain.Team{}, err
	}
	return toTeam(record), nil
}

func (r *TeamRepository) List(ctx context.Context, managerID *string) ([]domain.Team, error) {
	query := r.db.WithContext(ctx)
	if managerID != nil {
		query = query.Where("manager_id = ?", *managerID)
	}
	records := make([]teamRecord, 0)
	if err := query.Order("lower(name), id").Find(&records).Error; err != nil {
		return nil, err
	}
	teams := make([]domain.Team, len(records))
	for index, record := range records {
		teams[index] = toTeam(record)
	}
	return teams, nil
}

func (r *TeamRepository) FindByID(ctx context.Context, id string) (domain.Team, error) {
	var record teamRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Team{}, domain.ErrTeamNotFound
		}
		return domain.Team{}, err
	}
	return toTeam(record), nil
}

func (r *TeamRepository) UpdateManager(ctx context.Context, id, managerID string) (domain.Team, error) {
	result := r.db.WithContext(ctx).Model(&teamRecord{}).Where("id = ?", id).Update("manager_id", managerID)
	if result.Error != nil {
		return domain.Team{}, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.Team{}, domain.ErrTeamNotFound
	}
	return r.FindByID(ctx, id)
}

func toTeam(record teamRecord) domain.Team {
	return domain.Team{ID: record.ID, Name: record.Name, ManagerID: record.ManagerID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

var _ application.TeamRepository = (*TeamRepository)(nil)
