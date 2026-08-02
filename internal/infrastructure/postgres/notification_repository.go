package postgres

import (
	"context"
	"time"

	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"gorm.io/gorm"
)

type NotificationRepository struct {
	db *gorm.DB
}

type notificationRecord struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:uuid"`
	Kind      domain.NotificationKind
	Message   string
	IssueID   *uint64
	TeamID    *string `gorm:"type:uuid"`
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (notificationRecord) TableName() string {
	return "notifications"
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) CreateMany(ctx context.Context, notifications []domain.Notification) error {
	records := make([]notificationRecord, len(notifications))
	for index, notification := range notifications {
		records[index] = fromNotification(notification)
	}
	return r.db.WithContext(ctx).Create(&records).Error
}

func (r *NotificationRepository) List(ctx context.Context, userID string, limit int) ([]domain.Notification, int64, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	records := make([]notificationRecord, 0)
	if err := query.Order("id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	var unread int64
	if err := query.Model(&notificationRecord{}).Where("read_at IS NULL").Count(&unread).Error; err != nil {
		return nil, 0, err
	}
	return toNotifications(records), unread, nil
}

func (r *NotificationRepository) MarkRead(ctx context.Context, userID string, id uint64, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&notificationRecord{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", id, userID).
		Update("read_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&notificationRecord{}).Where("id = ? AND user_id = ?", id, userID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotificationNotFound
	}
	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&notificationRecord{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", now).Error
}

func (r *NotificationRepository) LatestID(ctx context.Context) (uint64, error) {
	var id uint64
	err := r.db.WithContext(ctx).Model(&notificationRecord{}).Select("COALESCE(MAX(id), 0)").Scan(&id).Error
	return id, err
}

func (r *NotificationRepository) ListAfter(ctx context.Context, id uint64, limit int) ([]domain.Notification, error) {
	records := make([]notificationRecord, 0)
	if err := r.db.WithContext(ctx).Where("id > ?", id).Order("id").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return toNotifications(records), nil
}

func toNotifications(records []notificationRecord) []domain.Notification {
	notifications := make([]domain.Notification, len(records))
	for index, record := range records {
		notifications[index] = domain.Notification{
			ID: record.ID, UserID: record.UserID, Kind: record.Kind, Message: record.Message,
			IssueID: record.IssueID, TeamID: record.TeamID, ReadAt: record.ReadAt, CreatedAt: record.CreatedAt,
		}
	}
	return notifications
}

func fromNotification(notification domain.Notification) notificationRecord {
	return notificationRecord{
		ID: notification.ID, UserID: notification.UserID, Kind: notification.Kind, Message: notification.Message,
		IssueID: notification.IssueID, TeamID: notification.TeamID, ReadAt: notification.ReadAt, CreatedAt: notification.CreatedAt,
	}
}

var _ application.NotificationRepository = (*NotificationRepository)(nil)
