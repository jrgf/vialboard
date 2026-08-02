package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jrgf/vialboard/internal/domain"
)

var (
	ErrTeamNameTaken      = errors.New("team name is already taken")
	ErrTeamForbidden      = errors.New("team action is forbidden")
	ErrUserHasTeam        = errors.New("user already belongs to a team")
	ErrTeamMemberNotFound = errors.New("team member was not found")
)

type TeamRepository interface {
	Create(context.Context, domain.Team) (domain.Team, error)
	List(context.Context, *string) ([]domain.Team, error)
	FindByID(context.Context, string) (domain.Team, error)
	UpdateManager(context.Context, string, string) (domain.Team, error)
}

type TeamService struct {
	teams         TeamRepository
	users         UserRepository
	notifications *NotificationService
}

func NewTeamService(teams TeamRepository, users UserRepository, notifications *NotificationService) *TeamService {
	return &TeamService{teams: teams, users: users, notifications: notifications}
}

func (s *TeamService) Create(ctx context.Context, actor domain.User, name, managerID string) (domain.Team, error) {
	switch actor.Role {
	case domain.RoleManager:
		if managerID != "" && managerID != actor.ID {
			return domain.Team{}, ErrTeamForbidden
		}
		managerID = actor.ID
	case domain.RoleAdmin:
		if err := s.requireActiveManager(ctx, managerID); err != nil {
			return domain.Team{}, err
		}
	default:
		return domain.Team{}, ErrTeamForbidden
	}
	team, err := domain.NewTeam(name, managerID)
	if err != nil {
		return domain.Team{}, err
	}
	return s.teams.Create(ctx, team)
}

func (s *TeamService) AssignManager(ctx context.Context, actor domain.User, teamID, managerID string) (domain.Team, error) {
	if actor.Role != domain.RoleAdmin {
		return domain.Team{}, ErrTeamForbidden
	}
	team, err := s.Find(ctx, teamID)
	if err != nil {
		return domain.Team{}, err
	}
	if err := s.requireActiveManager(ctx, managerID); err != nil {
		return domain.Team{}, err
	}
	if team.ManagerID == managerID {
		return team, nil
	}
	updated, err := s.teams.UpdateManager(ctx, team.ID, managerID)
	if err == nil {
		s.notifyMember(ctx, actor.ID, team.ManagerID, domain.NotificationTeamRemoved, fmt.Sprintf("You no longer manage %s", team.Name), team.ID)
		s.notifyMember(ctx, actor.ID, managerID, domain.NotificationTeamAdded, fmt.Sprintf("You now manage %s", team.Name), team.ID)
	}
	return updated, err
}

func (s *TeamService) AvailableManagers(ctx context.Context, actor domain.User) ([]domain.User, error) {
	if actor.Role != domain.RoleAdmin {
		return nil, ErrTeamForbidden
	}
	return s.users.ListActive(ctx, MemberListOptions{Role: domain.RoleManager})
}

func (s *TeamService) List(ctx context.Context, actor domain.User) ([]domain.Team, error) {
	switch actor.Role {
	case domain.RoleAdmin:
		return s.teams.List(ctx, nil)
	case domain.RoleManager:
		return s.teams.List(ctx, &actor.ID)
	default:
		if actor.TeamID == nil {
			return []domain.Team{}, nil
		}
		team, err := s.teams.FindByID(ctx, *actor.TeamID)
		if err != nil {
			return nil, err
		}
		return []domain.Team{team}, nil
	}
}

func (s *TeamService) Find(ctx context.Context, teamID string) (domain.Team, error) {
	if !domain.ValidTeamID(teamID) {
		return domain.Team{}, domain.NewValidationError("invalidTeamId", "Team ID must be a UUID")
	}
	return s.teams.FindByID(ctx, teamID)
}

func (s *TeamService) RequireManaged(ctx context.Context, actor domain.User, teamID string) (domain.Team, error) {
	team, err := s.Find(ctx, teamID)
	if err != nil {
		return domain.Team{}, err
	}
	if actor.Role != domain.RoleAdmin && (actor.Role != domain.RoleManager || team.ManagerID != actor.ID) {
		return domain.Team{}, ErrTeamForbidden
	}
	return team, nil
}

func (s *TeamService) Members(ctx context.Context, actor domain.User, teamID string) ([]domain.User, error) {
	team, err := s.RequireVisible(ctx, actor, teamID)
	if err != nil {
		return nil, err
	}
	members, err := s.users.ListActive(ctx, MemberListOptions{TeamID: &teamID})
	if err != nil {
		return nil, err
	}
	manager, err := s.users.FindByID(ctx, team.ManagerID)
	if err != nil {
		return nil, err
	}
	return append([]domain.User{manager}, members...), nil
}

func (s *TeamService) AvailableUsers(ctx context.Context, actor domain.User) ([]domain.User, error) {
	if actor.Role != domain.RoleAdmin && actor.Role != domain.RoleManager {
		return nil, ErrTeamForbidden
	}
	return s.users.ListActive(ctx, MemberListOptions{Unassigned: true})
}

func (s *TeamService) AddMember(ctx context.Context, actor domain.User, teamID, userID string) (domain.User, error) {
	team, err := s.RequireManaged(ctx, actor, teamID)
	if err != nil {
		return domain.User{}, err
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if !user.Active || user.Role != domain.RoleViewer {
		return domain.User{}, domain.NewValidationError("invalidTeamMember", "Only active viewers can belong to a team")
	}
	if user.TeamID != nil {
		if *user.TeamID == teamID {
			return user, nil
		}
		if actor.Role != domain.RoleAdmin {
			return domain.User{}, ErrUserHasTeam
		}
	}
	updated, err := s.users.SetTeam(ctx, userID, user.TeamID, &teamID)
	if err == nil {
		s.notifyMember(ctx, actor.ID, updated.ID, domain.NotificationTeamAdded, fmt.Sprintf("You were added to %s", team.Name), team.ID)
	}
	return updated, err
}

func (s *TeamService) RemoveMember(ctx context.Context, actor domain.User, teamID, userID string) error {
	team, err := s.RequireManaged(ctx, actor, teamID)
	if err != nil {
		return err
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.TeamID == nil || *user.TeamID != teamID {
		return ErrTeamMemberNotFound
	}
	_, err = s.users.SetTeam(ctx, userID, &teamID, nil)
	if err == nil {
		s.notifyMember(ctx, actor.ID, user.ID, domain.NotificationTeamRemoved, fmt.Sprintf("You were removed from %s", team.Name), team.ID)
	}
	return err
}

func (s *TeamService) RequireVisible(ctx context.Context, actor domain.User, teamID string) (domain.Team, error) {
	team, err := s.Find(ctx, teamID)
	if err != nil {
		return domain.Team{}, err
	}
	if actor.Role == domain.RoleAdmin || actor.Role == domain.RoleManager && team.ManagerID == actor.ID || actor.TeamID != nil && *actor.TeamID == team.ID {
		return team, nil
	}
	return domain.Team{}, ErrTeamForbidden
}

func (s *TeamService) notifyMember(ctx context.Context, actorID, userID string, kind domain.NotificationKind, message, teamID string) {
	// ponytail: notifications are best-effort after the membership commit; use an outbox if guaranteed delivery becomes contractual.
	if err := s.notifications.Notify(ctx, []string{userID}, actorID, kind, message, nil, &teamID); err != nil {
		slog.ErrorContext(ctx, "create team notification", "error", err, "team_id", teamID, "user_id", userID)
	}
}

func (s *TeamService) requireActiveManager(ctx context.Context, managerID string) error {
	if !domain.ValidUserID(managerID) {
		return domain.NewValidationError("invalidManagerId", "Manager ID must be a UUID")
	}
	manager, err := s.users.FindByID(ctx, managerID)
	if errors.Is(err, ErrUserNotFound) || err == nil && (!manager.Active || manager.Role != domain.RoleManager) {
		return domain.NewValidationError("invalidManagerId", "Team manager must be an active manager")
	}
	return err
}
