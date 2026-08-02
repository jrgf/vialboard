package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"gorm.io/gorm"
)

type IssueRepository struct {
	db *gorm.DB
}

type issueRecord struct {
	ID          uint64 `gorm:"primaryKey"`
	Title       string
	Description string
	Status      string
	Priority    string
	DueDate     *time.Time
	CreatedBy   string  `gorm:"type:uuid"`
	TeamID      *string `gorm:"type:uuid"`
	AssigneeID  *string `gorm:"type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type activityRecord struct {
	ID            uint64 `gorm:"primaryKey"`
	IssueID       uint64
	ActorID       string `gorm:"type:uuid"`
	ActorUsername string `gorm:"->"`
	Kind          string
	Body          string
	CreatedAt     time.Time
}

func (activityRecord) TableName() string {
	return "issue_activities"
}

func (issueRecord) TableName() string {
	return "issues"
}

func NewIssueRepository(db *gorm.DB) *IssueRepository {
	return &IssueRepository{db: db}
}

func (r *IssueRepository) List(ctx context.Context, options application.ListOptions) ([]domain.Issue, int64, error) {
	query := r.db.WithContext(ctx).Model(&issueRecord{})
	query = scopeIssues(query, options.Scope)
	if options.Status != nil {
		query = query.Where("status = ?", *options.Status)
	}
	if options.Priority != nil {
		query = query.Where("priority = ?", *options.Priority)
	}
	if options.TeamID != nil {
		query = query.Where("team_id = ?", *options.TeamID)
	}
	if options.Search != "" {
		search := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(options.Search)
		query = query.Where(`(title ILIKE ? ESCAPE '\' OR description ILIKE ? ESCAPE '\')`, "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	records := make([]issueRecord, 0)
	if err := query.Order(options.Order).Limit(options.Limit).Offset(options.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	issues := make([]domain.Issue, len(records))
	for index, record := range records {
		issues[index] = toDomain(record)
	}
	return issues, total, nil
}

func (r *IssueRepository) FindByID(ctx context.Context, id uint64, scope application.IssueScope) (domain.Issue, error) {
	var record issueRecord
	err := scopeIssues(r.db.WithContext(ctx), scope).First(&record, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Issue{}, domain.ErrIssueNotFound
	}
	return toDomain(record), err
}

func (r *IssueRepository) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	record := fromDomain(issue)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Create(&activityRecord{IssueID: record.ID, ActorID: issue.CreatedBy, Kind: string(domain.ActivityCreated)}).Error
	})
	if err != nil {
		return domain.Issue{}, err
	}
	return toDomain(record), nil
}

func (r *IssueRepository) Update(ctx context.Context, issue domain.Issue, scope application.IssueScope, actorID string, activities []domain.IssueActivity) (domain.Issue, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := scopeIssues(tx.Model(&issueRecord{}), scope)
		result := query.Where("id = ?", issue.ID).Updates(map[string]any{
			"title":       issue.Title,
			"description": issue.Description,
			"status":      issue.Status,
			"priority":    issue.Priority,
			"due_date":    issue.DueDate,
			"team_id":     issue.TeamID,
			"assignee_id": issue.AssigneeID,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrIssueNotFound
		}
		for _, activity := range activities {
			if err := tx.Create(&activityRecord{IssueID: issue.ID, ActorID: actorID, Kind: string(activity.Kind), Body: activity.Body}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Issue{}, err
	}
	return r.FindByID(ctx, issue.ID, scope)
}

func (r *IssueRepository) Delete(ctx context.Context, id uint64, scope application.IssueScope) error {
	result := scopeIssues(r.db.WithContext(ctx), scope).Delete(&issueRecord{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrIssueNotFound
	}
	return nil
}

func scopeIssues(query *gorm.DB, scope application.IssueScope) *gorm.DB {
	switch scope.Role {
	case domain.RoleAdmin:
		return query
	case domain.RoleManager:
		return query.Where("team_id IN (SELECT id FROM teams WHERE manager_id = ?)", scope.UserID)
	default:
		return query.Where("assignee_id = ?", scope.UserID)
	}
}

func (r *IssueRepository) Seed(ctx context.Context, issues []domain.Issue) (int, error) {
	created := 0
	// ponytail: titles are stable seed keys; add explicit seed keys if titles must become editable fixtures.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, issue := range issues {
			record := fromDomain(issue)
			result := tx.Where("title = ?", record.Title).FirstOrCreate(&record)
			if result.Error != nil {
				return result.Error
			}
			created += int(result.RowsAffected)
			if result.RowsAffected > 0 {
				if err := tx.Create(&activityRecord{IssueID: record.ID, ActorID: issue.CreatedBy, Kind: string(domain.ActivityCreated)}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	return created, err
}

func (r *IssueRepository) ListActivity(ctx context.Context, issueID uint64, scope application.IssueScope, limit, offset int) ([]domain.IssueActivity, int64, error) {
	visibleIssues := scopeIssues(r.db.WithContext(ctx).Model(&issueRecord{}).Select("id"), scope)
	query := r.db.WithContext(ctx).Model(&activityRecord{}).Where("issue_id = ? AND issue_id IN (?)", issueID, visibleIssues)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := make([]activityRecord, 0)
	if err := query.Select("issue_activities.*, users.username AS actor_username").
		Joins("JOIN users ON users.id = issue_activities.actor_id").
		Order("issue_activities.id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.IssueActivity, len(records))
	for index, record := range records {
		items[index] = toActivity(record)
	}
	return items, total, nil
}

func (r *IssueRepository) AddComment(ctx context.Context, issueID uint64, scope application.IssueScope, comment domain.IssueActivity) (domain.IssueActivity, error) {
	record := activityRecord{IssueID: issueID, ActorID: comment.ActorID, Kind: string(comment.Kind), Body: comment.Body}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := scopeIssues(tx.Model(&issueRecord{}), scope).Where("id = ?", issueID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return domain.ErrIssueNotFound
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return domain.IssueActivity{}, err
	}
	var created activityRecord
	if err := r.db.WithContext(ctx).Model(&activityRecord{}).
		Select("issue_activities.*, users.username AS actor_username").
		Joins("JOIN users ON users.id = issue_activities.actor_id").
		First(&created, "issue_activities.id = ?", record.ID).Error; err != nil {
		return domain.IssueActivity{}, err
	}
	return toActivity(created), nil
}

func toActivity(record activityRecord) domain.IssueActivity {
	return domain.IssueActivity{
		ID:            record.ID,
		IssueID:       record.IssueID,
		ActorID:       record.ActorID,
		ActorUsername: record.ActorUsername,
		Kind:          domain.ActivityKind(record.Kind),
		Body:          record.Body,
		CreatedAt:     record.CreatedAt,
	}
}

func toDomain(record issueRecord) domain.Issue {
	return domain.Issue{
		ID:          record.ID,
		Title:       record.Title,
		Description: record.Description,
		Status:      domain.Status(record.Status),
		Priority:    domain.Priority(record.Priority),
		DueDate:     record.DueDate,
		CreatedBy:   record.CreatedBy,
		TeamID:      record.TeamID,
		AssigneeID:  record.AssigneeID,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func fromDomain(issue domain.Issue) issueRecord {
	return issueRecord{
		ID:          issue.ID,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      string(issue.Status),
		Priority:    string(issue.Priority),
		DueDate:     issue.DueDate,
		CreatedBy:   issue.CreatedBy,
		TeamID:      issue.TeamID,
		AssigneeID:  issue.AssigneeID,
		CreatedAt:   issue.CreatedAt,
		UpdatedAt:   issue.UpdatedAt,
	}
}

var _ application.IssueRepository = (*IssueRepository)(nil)
