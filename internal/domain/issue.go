package domain

import (
	"errors"
	"strings"
	"time"
)

type Status string
type Priority string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"

	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

var (
	ErrIssueNotFound  = errors.New("issue not found")
	ErrIssueForbidden = errors.New("issue action is forbidden")
)

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type Issue struct {
	ID          uint64
	Title       string
	Description string
	Status      Status
	Priority    Priority
	DueDate     *time.Time
	CreatedBy   string
	TeamID      *string
	AssigneeID  *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewIssue(title, description, status, priority, dueDate, createdBy, teamID, assigneeID string) (Issue, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Issue{}, NewValidationError("invalidTitle", "Title is required")
	}

	if status == "" {
		status = string(StatusOpen)
	}
	parsedStatus, err := ParseStatus(status)
	if err != nil {
		return Issue{}, err
	}
	parsedPriority, err := ParsePriority(priority)
	if err != nil {
		return Issue{}, err
	}
	parsedDueDate, err := ParseDueDate(dueDate)
	if err != nil {
		return Issue{}, err
	}
	if !ValidUserID(createdBy) {
		return Issue{}, NewValidationError("invalidCreatedBy", "Issue creator must be a user UUID")
	}
	var assignee *string
	var team *string
	if teamID != "" {
		if !ValidTeamID(teamID) {
			return Issue{}, NewValidationError("invalidTeamId", "Team ID must be a UUID")
		}
		team = &teamID
	}
	if assigneeID != "" {
		if !ValidUserID(assigneeID) {
			return Issue{}, NewValidationError("invalidAssigneeId", "Assignee ID must be a UUID")
		}
		assignee = &assigneeID
	}
	return Issue{
		Title:       title,
		Description: description,
		Status:      parsedStatus,
		Priority:    parsedPriority,
		DueDate:     parsedDueDate,
		CreatedBy:   createdBy,
		TeamID:      team,
		AssigneeID:  assignee,
	}, nil
}

func (i *Issue) ApplyUpdate(title, description, status, priority, dueDate, teamID, assigneeID *string) error {
	if title == nil && description == nil && status == nil && priority == nil && dueDate == nil && teamID == nil && assigneeID == nil {
		return NewValidationError("emptyUpdate", "At least one field is required")
	}
	if title != nil {
		value := strings.TrimSpace(*title)
		if value == "" {
			return NewValidationError("invalidTitle", "Title is required")
		}
		i.Title = value
	}
	if description != nil {
		i.Description = *description
	}
	if status != nil {
		value, err := ParseStatus(*status)
		if err != nil {
			return err
		}
		i.Status = value
	}
	if priority != nil {
		value, err := ParsePriority(*priority)
		if err != nil {
			return err
		}
		i.Priority = value
	}
	if dueDate != nil {
		value, err := ParseDueDate(*dueDate)
		if err != nil {
			return err
		}
		i.DueDate = value
	}
	if teamID != nil {
		if *teamID == "" {
			i.TeamID = nil
		} else {
			if !ValidTeamID(*teamID) {
				return NewValidationError("invalidTeamId", "Team ID must be a UUID")
			}
			i.TeamID = teamID
		}
	}
	if assigneeID != nil {
		if *assigneeID == "" {
			i.AssigneeID = nil
		} else {
			if !ValidUserID(*assigneeID) {
				return NewValidationError("invalidAssigneeId", "Assignee ID must be a UUID")
			}
			i.AssigneeID = assigneeID
		}
	}
	return nil
}

func ParseStatus(status string) (Status, error) {
	value := Status(status)
	if value != StatusOpen && value != StatusClosed {
		return "", NewValidationError("invalidStatus", "Status must be open or closed")
	}
	return value, nil
}

func ParsePriority(priority string) (Priority, error) {
	if priority == "" {
		return PriorityMedium, nil
	}
	value := Priority(priority)
	if value != PriorityLow && value != PriorityMedium && value != PriorityHigh && value != PriorityCritical {
		return "", NewValidationError("invalidPriority", "Priority must be low, medium, high, or critical")
	}
	return value, nil
}

func ParseDueDate(dueDate string) (*time.Time, error) {
	if dueDate == "" {
		return nil, nil
	}
	value, err := time.Parse(time.DateOnly, dueDate)
	if err != nil {
		return nil, NewValidationError("invalidDueDate", "Due date must use YYYY-MM-DD")
	}
	return &value, nil
}

func NewValidationError(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}
