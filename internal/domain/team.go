package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrTeamNotFound = errors.New("team not found")

type Team struct {
	ID        string
	Name      string
	ManagerID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewTeam(name, managerID string) (Team, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		return Team{}, NewValidationError("invalidTeamName", "Team name must be between 1 and 64 characters")
	}
	if !ValidUserID(managerID) {
		return Team{}, NewValidationError("invalidManagerId", "Team manager ID must be a UUID")
	}
	id, err := NewUserID()
	if err != nil {
		return Team{}, err
	}
	return Team{ID: id, Name: name, ManagerID: managerID}, nil
}

func ValidTeamID(value string) bool {
	return validUUID(value)
}
