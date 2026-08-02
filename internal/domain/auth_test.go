package domain

import (
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	now := time.Date(2026, time.August, 2, 18, 0, 0, 0, time.UTC)
	userID, err := NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	if len(userID) != 36 || userID[14] != '4' {
		t.Fatalf("invalid generated user UUID: %q", userID)
	}
	session, err := NewSession(userID, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.ID) != 64 || len(session.Token) != 64 || session.ID == session.Token || session.ID != SessionID(session.Token) || !session.ActiveAt(now) || session.ActiveAt(now.Add(time.Hour)) {
		t.Fatalf("unexpected new session: %+v", session)
	}
	session.Revoke(now.Add(time.Minute))
	if session.ActiveAt(now.Add(2 * time.Minute)) {
		t.Fatal("revoked session is active")
	}
	if _, err := NewSession("not-a-uuid", time.Hour, now); err == nil {
		t.Fatal("invalid user UUID was accepted")
	}
	if _, err := NewSession(userID, 0, now); err == nil {
		t.Fatal("zero lifetime was accepted")
	}
}

func TestUsernameValidation(t *testing.T) {
	for _, username := range []string{"a", "User_01", "user-name", "1234567890123456"} {
		if _, err := NewUser(username, "hash", string(RoleViewer)); err != nil {
			t.Fatalf("valid username %q: %v", username, err)
		}
	}
	for _, username := range []string{"", "-user", "_user", "user name", "user!", "álvaro", "12345678901234567", "admin'--"} {
		if _, err := NewUser(username, "hash", string(RoleViewer)); err == nil {
			t.Fatalf("invalid username %q was accepted", username)
		}
	}
}

func TestManagerRole(t *testing.T) {
	user, err := NewUser("manager", "hash", string(RoleManager))
	if err != nil || user.Role != RoleManager {
		t.Fatalf("manager role: user=%+v err=%v", user, err)
	}
	if role, err := ParseRole("worker"); err != nil || role != RoleViewer {
		t.Fatalf("worker alias: role=%q err=%v", role, err)
	}
}
