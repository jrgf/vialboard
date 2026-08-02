package domain

import (
	"strings"
	"time"
)

type ActivityKind string

const (
	ActivityCreated           ActivityKind = "created"
	ActivityUpdated           ActivityKind = "updated"
	ActivityStatusChanged     ActivityKind = "statusChanged"
	ActivityPriorityChanged   ActivityKind = "priorityChanged"
	ActivityDueDateChanged    ActivityKind = "dueDateChanged"
	ActivityAssignmentChanged ActivityKind = "assignmentChanged"
	ActivityTeamChanged       ActivityKind = "teamChanged"
	ActivityComment           ActivityKind = "comment"
)

type IssueActivity struct {
	ID            uint64
	IssueID       uint64
	ActorID       string
	ActorUsername string
	Kind          ActivityKind
	Body          string
	CreatedAt     time.Time
}

func NewComment(issueID uint64, actorID, body string) (IssueActivity, error) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 2000 {
		return IssueActivity{}, NewValidationError("invalidComment", "Comment must be between 1 and 2000 characters")
	}
	return IssueActivity{IssueID: issueID, ActorID: actorID, Kind: ActivityComment, Body: body}, nil
}
