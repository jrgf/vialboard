package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jrgf/vialboard/internal/domain"
)

type IssueRepository interface {
	List(context.Context, ListOptions) ([]domain.Issue, int64, error)
	FindByID(context.Context, uint64, IssueScope) (domain.Issue, error)
	Create(context.Context, domain.Issue) (domain.Issue, error)
	Update(context.Context, domain.Issue, IssueScope, string, []domain.IssueActivity) (domain.Issue, error)
	Delete(context.Context, uint64, IssueScope) error
	Seed(context.Context, []domain.Issue) (int, error)
	ListActivity(context.Context, uint64, IssueScope, int, int) ([]domain.IssueActivity, int64, error)
	AddComment(context.Context, uint64, IssueScope, domain.IssueActivity) (domain.IssueActivity, error)
}

type IssueScope struct {
	UserID string
	Role   domain.Role
	TeamID *string
}

type ListOptions struct {
	Status   *domain.Status
	Priority *domain.Priority
	TeamID   *string
	Search   string
	Scope    IssueScope
	Order    string
	Limit    int
	Offset   int
}

type IssuePage struct {
	Issues     []domain.Issue
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

type ActivityPage struct {
	Items      []domain.IssueActivity
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

type IssueService struct {
	repository    IssueRepository
	users         UserRepository
	teams         *TeamService
	notifications *NotificationService
}

func NewIssueService(repository IssueRepository, users UserRepository, teams *TeamService, notifications *NotificationService) *IssueService {
	return &IssueService{repository: repository, users: users, teams: teams, notifications: notifications}
}

func (s *IssueService) List(ctx context.Context, actor domain.User, status, priority, teamID, search, sort, order string, page, pageSize int) (IssuePage, error) {
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return IssuePage{}, domain.NewValidationError("invalidPage", "Page must be a positive integer")
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize < 1 || pageSize > 100 {
		return IssuePage{}, domain.NewValidationError("invalidPageSize", "Page size must be between 1 and 100")
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/pageSize {
		return IssuePage{}, domain.NewValidationError("invalidPage", "Page is too large")
	}

	var statusFilter *domain.Status
	if status != "" {
		parsed, err := domain.ParseStatus(status)
		if err != nil {
			return IssuePage{}, err
		}
		statusFilter = &parsed
	}
	var priorityFilter *domain.Priority
	if priority != "" {
		parsed, err := domain.ParsePriority(priority)
		if err != nil {
			return IssuePage{}, err
		}
		priorityFilter = &parsed
	}
	var teamFilter *string
	if teamID != "" {
		if !domain.ValidTeamID(teamID) {
			return IssuePage{}, domain.NewValidationError("invalidTeamId", "Team ID must be a UUID")
		}
		teamFilter = &teamID
	}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > 100 {
		return IssuePage{}, domain.NewValidationError("invalidSearch", "Search must not exceed 100 characters")
	}
	issueOrder, err := parseIssueOrder(sort, order)
	if err != nil {
		return IssuePage{}, err
	}

	issues, total, err := s.repository.List(ctx, ListOptions{
		Status:   statusFilter,
		Priority: priorityFilter,
		TeamID:   teamFilter,
		Search:   search,
		Scope:    issueVisibilityScope(actor),
		Order:    issueOrder,
		Limit:    pageSize,
		Offset:   (page - 1) * pageSize,
	})
	if err != nil {
		return IssuePage{}, err
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
		return IssuePage{}, domain.NewValidationError("pageOutOfRange", "Page cannot exceed total pages")
	}
	return IssuePage{Issues: issues, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *IssueService) Create(ctx context.Context, actor domain.User, title, description, status, priority, dueDate, teamID, assigneeID string) (domain.Issue, error) {
	if actor.Role == domain.RoleViewer {
		return domain.Issue{}, domain.ErrIssueForbidden
	}
	if actor.Role == domain.RoleManager && teamID == "" {
		return domain.Issue{}, domain.NewValidationError("teamRequired", "Managers must create issues for a team")
	}
	if err := s.validatePlacement(ctx, actor, teamID, assigneeID); err != nil {
		return domain.Issue{}, err
	}
	issue, err := domain.NewIssue(title, description, status, priority, dueDate, actor.ID, teamID, assigneeID)
	if err != nil {
		return domain.Issue{}, err
	}
	created, err := s.repository.Create(ctx, issue)
	if err == nil {
		s.notifyIssue(ctx, actor, created, domain.NotificationIssueCreated, fmt.Sprintf("%s created %q", actor.Username, created.Title))
	}
	return created, err
}

func (s *IssueService) Get(ctx context.Context, actor domain.User, id uint64) (domain.Issue, error) {
	return s.repository.FindByID(ctx, id, issueVisibilityScope(actor))
}

func (s *IssueService) Update(ctx context.Context, actor domain.User, id uint64, title, description, status, priority, dueDate, teamID, assigneeID *string) (domain.Issue, error) {
	visibility := issueVisibilityScope(actor)
	issue, err := s.repository.FindByID(ctx, id, visibility)
	if err != nil {
		return domain.Issue{}, err
	}
	if actor.Role == domain.RoleViewer && (status == nil || title != nil || description != nil || priority != nil || dueDate != nil || teamID != nil || assigneeID != nil) {
		return domain.Issue{}, domain.ErrIssueForbidden
	}
	if teamID != nil || assigneeID != nil {
		if actor.Role == domain.RoleViewer {
			return domain.Issue{}, domain.ErrIssueForbidden
		}
		nextTeamID := stringValue(issue.TeamID)
		nextAssigneeID := stringValue(issue.AssigneeID)
		if teamID != nil {
			nextTeamID = *teamID
		}
		if assigneeID != nil {
			nextAssigneeID = *assigneeID
		}
		if err := s.validatePlacement(ctx, actor, nextTeamID, nextAssigneeID); err != nil {
			return domain.Issue{}, err
		}
	}
	before := issue
	if err := issue.ApplyUpdate(title, description, status, priority, dueDate, teamID, assigneeID); err != nil {
		return domain.Issue{}, err
	}
	updated, err := s.repository.Update(ctx, issue, visibility, actor.ID, issueActivities(before, issue, actor.ID))
	if err == nil {
		s.notifyIssue(ctx, actor, updated, domain.NotificationIssueUpdated, fmt.Sprintf("%s updated %q", actor.Username, updated.Title))
	}
	return updated, err
}

func (s *IssueService) Delete(ctx context.Context, actor domain.User, id uint64) error {
	visibility := issueVisibilityScope(actor)
	issue, err := s.repository.FindByID(ctx, id, visibility)
	if err != nil {
		return err
	}
	if actor.Role == domain.RoleViewer {
		return domain.ErrIssueForbidden
	}
	if actor.Role == domain.RoleManager {
		if issue.TeamID == nil {
			return domain.ErrIssueForbidden
		}
		if _, err := s.teams.RequireManaged(ctx, actor, *issue.TeamID); err != nil {
			return domain.ErrIssueForbidden
		}
	} else if actor.Role != domain.RoleAdmin && issue.CreatedBy != actor.ID {
		return domain.ErrIssueForbidden
	}
	return s.repository.Delete(ctx, id, visibility)
}

func (s *IssueService) ListActivity(ctx context.Context, actor domain.User, issueID uint64, page, pageSize int) (ActivityPage, error) {
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return ActivityPage{}, domain.NewValidationError("invalidPage", "Page must be a positive integer")
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize < 1 || pageSize > 100 {
		return ActivityPage{}, domain.NewValidationError("invalidPageSize", "Page size must be between 1 and 100")
	}
	visibility := issueVisibilityScope(actor)
	if _, err := s.repository.FindByID(ctx, issueID, visibility); err != nil {
		return ActivityPage{}, err
	}
	items, total, err := s.repository.ListActivity(ctx, issueID, visibility, pageSize, (page-1)*pageSize)
	if err != nil {
		return ActivityPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	if page > max(1, totalPages) {
		return ActivityPage{}, domain.NewValidationError("pageOutOfRange", "Page cannot exceed total pages")
	}
	return ActivityPage{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *IssueService) AddComment(ctx context.Context, actor domain.User, issueID uint64, body string) (domain.IssueActivity, error) {
	visibility := issueVisibilityScope(actor)
	issue, err := s.repository.FindByID(ctx, issueID, visibility)
	if err != nil {
		return domain.IssueActivity{}, err
	}
	comment, err := domain.NewComment(issueID, actor.ID, body)
	if err != nil {
		return domain.IssueActivity{}, err
	}
	created, err := s.repository.AddComment(ctx, issueID, visibility, comment)
	if err == nil {
		s.notifyIssue(ctx, actor, issue, domain.NotificationIssueCommented, fmt.Sprintf("%s commented on %q", actor.Username, issue.Title))
	}
	return created, err
}

func (s *IssueService) Seed(ctx context.Context, owner domain.User) (int, error) {
	issues := make([]domain.Issue, 0, len(initialIssues))
	for _, initial := range initialIssues {
		issue, err := domain.NewIssue(initial.title, initial.description, initial.status, string(domain.PriorityMedium), "", owner.ID, "", "")
		if err != nil {
			return 0, err
		}
		issues = append(issues, issue)
	}
	return s.repository.Seed(ctx, issues)
}

func issueVisibilityScope(actor domain.User) IssueScope {
	return IssueScope{UserID: actor.ID, Role: actor.Role, TeamID: actor.TeamID}
}

func (s *IssueService) validatePlacement(ctx context.Context, actor domain.User, teamID, assigneeID string) error {
	if teamID != "" {
		team, err := s.teams.Find(ctx, teamID)
		if err != nil {
			return err
		}
		switch actor.Role {
		case domain.RoleManager:
			if team.ManagerID != actor.ID {
				return domain.ErrIssueForbidden
			}
		case domain.RoleViewer:
			if actor.TeamID == nil || *actor.TeamID != team.ID {
				return domain.ErrIssueForbidden
			}
		}
	} else if actor.Role == domain.RoleManager || actor.Role == domain.RoleViewer && actor.TeamID != nil {
		return domain.NewValidationError("teamRequired", "Team members and managers must create issues for a team")
	}

	if assigneeID == "" {
		return nil
	}
	if !domain.ValidUserID(assigneeID) {
		return domain.NewValidationError("invalidAssigneeId", "Assignee ID must be a UUID")
	}
	user, err := s.users.FindByID(ctx, assigneeID)
	if errors.Is(err, ErrUserNotFound) || err == nil && (!user.Active || user.Role != domain.RoleViewer) {
		return domain.NewValidationError("invalidAssigneeId", "Assignee must be an active viewer")
	}
	if err != nil {
		return err
	}
	if actor.Role == domain.RoleViewer && user.ID != actor.ID {
		return domain.ErrIssueForbidden
	}
	if teamID == "" {
		if user.TeamID != nil || actor.Role == domain.RoleViewer && user.ID != actor.ID {
			return domain.NewValidationError("invalidAssigneeId", "Assignee must belong to the issue team")
		}
		return nil
	}
	if user.TeamID == nil || *user.TeamID != teamID {
		return domain.NewValidationError("invalidAssigneeId", "Assignee must belong to the issue team")
	}
	return nil
}

func parseIssueOrder(sort, order string) (string, error) {
	direction := "DESC"
	switch order {
	case "", "desc":
	case "asc":
		direction = "ASC"
	default:
		return "", domain.NewValidationError("invalidOrder", "Order must be asc or desc")
	}
	switch sort {
	case "", "createdAt":
		return "created_at " + direction + ", id " + direction, nil
	case "updatedAt":
		return "updated_at " + direction + ", id DESC", nil
	case "dueDate":
		return "due_date " + direction + " NULLS LAST, id DESC", nil
	case "priority":
		return "CASE priority WHEN 'low' THEN 1 WHEN 'medium' THEN 2 WHEN 'high' THEN 3 WHEN 'critical' THEN 4 END " + direction + ", id DESC", nil
	case "title":
		return "lower(title) " + direction + ", id DESC", nil
	default:
		return "", domain.NewValidationError("invalidSort", "Sort must be createdAt, updatedAt, dueDate, priority, or title")
	}
}

func issueActivities(before, after domain.Issue, actorID string) []domain.IssueActivity {
	activities := make([]domain.IssueActivity, 0, 5)
	add := func(kind domain.ActivityKind) {
		activities = append(activities, domain.IssueActivity{IssueID: after.ID, ActorID: actorID, Kind: kind})
	}
	if before.Title != after.Title || before.Description != after.Description {
		add(domain.ActivityUpdated)
	}
	if before.Status != after.Status {
		add(domain.ActivityStatusChanged)
	}
	if before.Priority != after.Priority {
		add(domain.ActivityPriorityChanged)
	}
	if !sameDate(before.DueDate, after.DueDate) {
		add(domain.ActivityDueDateChanged)
	}
	if !sameString(before.AssigneeID, after.AssigneeID) {
		add(domain.ActivityAssignmentChanged)
	}
	if !sameString(before.TeamID, after.TeamID) {
		add(domain.ActivityTeamChanged)
	}
	return activities
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sameDate(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func sameString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func (s *IssueService) notifyIssue(ctx context.Context, actor domain.User, issue domain.Issue, kind domain.NotificationKind, message string, extraRecipients ...string) {
	recipients := append([]string{issue.CreatedBy}, extraRecipients...)
	if issue.AssigneeID != nil {
		recipients = append(recipients, *issue.AssigneeID)
	}
	if issue.TeamID != nil {
		team, err := s.teams.Find(ctx, *issue.TeamID)
		if err != nil {
			slog.ErrorContext(ctx, "find notification team", "error", err, "issue_id", issue.ID)
			return
		}
		recipients = append(recipients, team.ManagerID)
	}
	issueID := issue.ID
	// ponytail: notifications are best-effort after the issue commit; use an outbox if guaranteed delivery becomes contractual.
	if err := s.notifications.Notify(ctx, recipients, actor.ID, kind, message, &issueID, issue.TeamID); err != nil {
		slog.ErrorContext(ctx, "create issue notifications", "error", err, "issue_id", issue.ID)
	}
}

var initialIssues = []struct {
	title       string
	description string
	status      string
}{
	{"Define the issue schema", "Agree on the fields and statuses exposed by the JSON API.", string(domain.StatusClosed)},
	{"Add PostgreSQL persistence", "Store issues with GORM instead of process memory.", string(domain.StatusClosed)},
	{"Implement CRUD routes", "Create, list, read, update, and delete issues through go-vial.", string(domain.StatusClosed)},
	{"Add request validation", "Reject invalid titles, statuses, IDs, and empty updates.", string(domain.StatusClosed)},
	{"Add authentication", "Protect write operations once the API has users.", string(domain.StatusOpen)},
	{"Add filtering and pagination", "Keep issue listing bounded as the backlog grows.", string(domain.StatusOpen)},
	{"Add versioned migrations", "Replace AutoMigrate before production schema changes.", string(domain.StatusOpen)},
}
