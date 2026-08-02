package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type NotificationKind string

const (
	NotificationIssueCreated   NotificationKind = "issueCreated"
	NotificationIssueUpdated   NotificationKind = "issueUpdated"
	NotificationIssueCommented NotificationKind = "issueCommented"
	NotificationTeamAdded      NotificationKind = "teamAdded"
	NotificationTeamRemoved    NotificationKind = "teamRemoved"
)

var ErrNotificationNotFound = errors.New("notification not found")

type Notification struct {
	ID        uint64
	UserID    string
	Kind      NotificationKind
	Message   string
	IssueID   *uint64
	TeamID    *string
	ReadAt    *time.Time
	CreatedAt time.Time
}

func NewNotification(userID string, kind NotificationKind, message string, issueID *uint64, teamID *string) (Notification, error) {
	message = strings.TrimSpace(message)
	if !ValidUserID(userID) {
		return Notification{}, NewValidationError("invalidUserId", "Notification user ID must be a UUID")
	}
	if !kind.valid() {
		return Notification{}, NewValidationError("invalidNotificationKind", "Notification kind is invalid")
	}
	if !utf8.ValidString(message) || utf8.RuneCountInString(message) < 1 || utf8.RuneCountInString(message) > 300 {
		return Notification{}, NewValidationError("invalidNotificationMessage", "Notification message must be between 1 and 300 characters")
	}
	return Notification{UserID: userID, Kind: kind, Message: message, IssueID: issueID, TeamID: teamID}, nil
}

func (kind NotificationKind) valid() bool {
	switch kind {
	case NotificationIssueCreated, NotificationIssueUpdated, NotificationIssueCommented, NotificationTeamAdded, NotificationTeamRemoved:
		return true
	default:
		return false
	}
}
