package httpapi_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jrgf/go-vial/testkit"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	passwordhash "github.com/jrgf/vialboard/internal/infrastructure/password"
	postgresstore "github.com/jrgf/vialboard/internal/infrastructure/postgres"
	"github.com/jrgf/vialboard/internal/transport/httpapi"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type issueResponse struct {
	ID          uint64          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      domain.Status   `json:"status"`
	Priority    domain.Priority `json:"priority"`
	DueDate     *string         `json:"dueDate"`
	CreatedBy   string          `json:"createdBy"`
	TeamID      *string         `json:"teamId"`
	AssigneeID  *string         `json:"assigneeId"`
}

type issueListResponse struct {
	Items      []issueResponse `json:"items"`
	Pagination struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"pageSize"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
	} `json:"pagination"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      struct {
		ID       string      `json:"id"`
		Username string      `json:"username"`
		Role     domain.Role `json:"role"`
		TeamID   *string     `json:"teamId"`
	} `json:"user"`
}

type operationResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	StatusURL   string `json:"status_url"`
	Progress    int    `json:"progress"`
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	Result      *struct {
		DownloadURL string `json:"download_url"`
		Filename    string `json:"filename"`
		Rows        int    `json:"rows"`
	} `json:"result"`
}

const (
	testUsername = "test-user"
	testPassword = "test-password"
)

func TestIssueLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	adminDB, adminSQL, err := postgresstore.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })

	schema := "vialboard_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema)) })

	testURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := testURL.Query()
	query.Set("search_path", schema)
	testURL.RawQuery = query.Encode()
	db, sqlDB, err := postgresstore.Open(testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx := context.Background()
	if err := postgresstore.MigrateUp(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("users") || !db.Migrator().HasTable("sessions") {
		t.Fatal("auth tables were not created")
	}
	var userIDType string
	if err := db.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'id'").Scan(&userIDType).Error; err != nil {
		t.Fatal(err)
	}
	if userIDType != "uuid" {
		t.Fatalf("user ID type = %q, want uuid", userIDType)
	}
	if !db.Migrator().HasColumn("users", "role") {
		t.Fatal("user role column was not created")
	}
	if !db.Migrator().HasIndex("sessions", "sessions_one_current_per_user_key") {
		t.Fatal("single-session index was not created")
	}
	if !db.Migrator().HasConstraint("users", "users_username_format_check") {
		t.Fatal("username format constraint was not created")
	}
	if !db.Migrator().HasColumn("issues", "created_by") || !db.Migrator().HasConstraint("issues", "issues_created_by_fkey") {
		t.Fatal("issue ownership schema was not created")
	}
	if !db.Migrator().HasColumn("issues", "priority") || !db.Migrator().HasColumn("issues", "due_date") || !db.Migrator().HasColumn("issues", "assignee_id") {
		t.Fatal("issue planning schema was not created")
	}
	if !db.Migrator().HasTable("issue_activities") {
		t.Fatal("issue activity table was not created")
	}
	if !db.Migrator().HasTable("teams") || !db.Migrator().HasColumn("users", "team_id") || !db.Migrator().HasColumn("issues", "team_id") {
		t.Fatal("team schema was not created")
	}
	if !db.Migrator().HasTable("notifications") || !db.Migrator().HasIndex("notifications", "notifications_unread_idx") {
		t.Fatal("notification schema was not created")
	}
	if !db.Migrator().HasTable("issue_exports") {
		t.Fatal("issue export schema was not created")
	}
	var managerTriggerCount int64
	if err := db.Raw("SELECT COUNT(*) FROM pg_trigger AS trigger JOIN pg_class AS relation ON relation.oid = trigger.tgrelid JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace WHERE namespace.nspname = current_schema() AND trigger.tgname IN ('teams_require_active_manager', 'users_preserve_managed_team_manager') AND NOT trigger.tgisinternal").Scan(&managerTriggerCount).Error; err != nil {
		t.Fatal(err)
	}
	if managerTriggerCount != 2 {
		t.Fatalf("manager constraint triggers = %d, want 2", managerTriggerCount)
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("issue_exports") {
		t.Fatal("issue export table still exists after rollback")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT COUNT(*) FROM pg_trigger AS trigger JOIN pg_class AS relation ON relation.oid = trigger.tgrelid JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace WHERE namespace.nspname = current_schema() AND trigger.tgname IN ('teams_require_active_manager', 'users_preserve_managed_team_manager') AND NOT trigger.tgisinternal").Scan(&managerTriggerCount).Error; err != nil {
		t.Fatal(err)
	}
	if managerTriggerCount != 0 || !db.Migrator().HasTable("notifications") {
		t.Fatal("manager constraint rollback affected notifications")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("notifications") {
		t.Fatal("notification schema still exists after rollback")
	}
	if !db.Migrator().HasTable("teams") {
		t.Fatal("notification rollback removed teams")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("teams") || db.Migrator().HasColumn("users", "team_id") || db.Migrator().HasColumn("issues", "team_id") {
		t.Fatal("team schema still exists after rollback")
	}
	if !db.Migrator().HasTable("issue_activities") {
		t.Fatal("team rollback removed issue activity")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("issue_activities") {
		t.Fatal("issue activity table still exists after rollback")
	}
	if !db.Migrator().HasColumn("issues", "priority") {
		t.Fatal("activity rollback removed issue planning")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("issues", "priority") || db.Migrator().HasColumn("issues", "due_date") || db.Migrator().HasColumn("issues", "assignee_id") {
		t.Fatal("issue planning schema still exists after rollback")
	}
	if !db.Migrator().HasColumn("issues", "created_by") {
		t.Fatal("issue planning rollback removed issue ownership")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("issues", "created_by") || db.Migrator().HasConstraint("issues", "issues_created_by_fkey") {
		t.Fatal("issue ownership schema still exists after rollback")
	}
	if !db.Migrator().HasConstraint("users", "users_username_format_check") {
		t.Fatal("issue ownership rollback removed username validation")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasConstraint("users", "users_username_format_check") {
		t.Fatal("username format constraint still exists after rollback")
	}
	if !db.Migrator().HasIndex("sessions", "sessions_one_current_per_user_key") {
		t.Fatal("username rollback removed the single-session index")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasIndex("sessions", "sessions_one_current_per_user_key") {
		t.Fatal("single-session index still exists after rollback")
	}
	if !db.Migrator().HasColumn("users", "role") {
		t.Fatal("single-session rollback removed the user role")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("users", "role") {
		t.Fatal("user role column still exists after rollback")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'id'").Scan(&userIDType).Error; err != nil {
		t.Fatal(err)
	}
	if userIDType != "bigint" {
		t.Fatalf("rolled-back user ID type = %q, want bigint", userIDType)
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("users") || db.Migrator().HasTable("sessions") {
		t.Fatal("auth tables still exist after migration down")
	}
	if !db.Migrator().HasTable("issues") {
		t.Fatal("auth rollback removed the issues table")
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := postgresstore.MigrateDown(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("issues") {
		t.Fatal("issues table still exists after migration down")
	}
	if err := postgresstore.MigrateUp(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	hasher := passwordhash.Hasher{}
	userRepository := postgresstore.NewUserRepository(db)
	users := application.NewUserService(userRepository, hasher)
	user, err := users.Create(ctx, testUsername, testPassword, string(domain.RoleAdmin))
	if err != nil {
		t.Fatal(err)
	}
	if len(user.ID) != 36 || user.PasswordHash == testPassword || !hasher.Verify(user.PasswordHash, testPassword) || user.Role != domain.RoleAdmin {
		t.Fatalf("invalid created user: %+v", user)
	}
	if _, err := users.Create(ctx, "TEST-USER", "another-password", string(domain.RoleAdmin)); !errors.Is(err, application.ErrUsernameTaken) {
		t.Fatalf("duplicate username error = %v", err)
	}
	if _, err := users.Create(ctx, "invalid-role", "another-password", "owner"); err == nil {
		t.Fatal("invalid role was accepted")
	}
	viewer, err := users.Create(ctx, "viewer", "viewer-password", string(domain.RoleViewer))
	if err != nil || viewer.Role != domain.RoleViewer {
		t.Fatalf("create viewer: user=%+v err=%v", viewer, err)
	}
	manager, err := users.Create(ctx, "manager", "manager-password", string(domain.RoleManager))
	if err != nil || manager.Role != domain.RoleManager {
		t.Fatalf("create manager: user=%+v err=%v", manager, err)
	}
	replacementManager, err := users.Create(ctx, "manager-two", "manager-two-password", string(domain.RoleManager))
	if err != nil || replacementManager.Role != domain.RoleManager {
		t.Fatalf("create replacement manager: user=%+v err=%v", replacementManager, err)
	}

	teamRepository := postgresstore.NewTeamRepository(db)
	notifications := application.NewNotificationService(postgresstore.NewNotificationRepository(db))
	teams := application.NewTeamService(teamRepository, userRepository, notifications)
	issues := application.NewIssueService(postgresstore.NewIssueRepository(db), userRepository, teams, notifications)
	app := httpapi.New(issues, users, teams, notifications, sqlDB)
	server := testkit.Start(t, app)
	for _, path := range []string{"/health/live", "/health/ready"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("%s status = %d", path, response.StatusCode)
		}
	}

	dashboardResponse, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	dashboardBody, err := io.ReadAll(dashboardResponse.Body)
	_ = dashboardResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dashboardResponse.StatusCode != http.StatusOK ||
		!bytes.Contains(dashboardBody, []byte(`<title>Vialboard</title>`)) ||
		dashboardResponse.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("dashboard status = %d, csp = %q", dashboardResponse.StatusCode, dashboardResponse.Header.Get("Content-Security-Policy"))
	}
	dashboardScript, err := http.Get(server.URL + "/dashboard/static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	_ = dashboardScript.Body.Close()
	if dashboardScript.StatusCode != http.StatusOK || !strings.Contains(dashboardScript.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("dashboard script status = %d, content type = %q", dashboardScript.StatusCode, dashboardScript.Header.Get("Content-Type"))
	}

	for _, credentials := range []struct {
		name     string
		username string
		password string
	}{
		{name: "unknown", username: "missing-user", password: "wrong-password"},
		{name: "wrong", username: testUsername, password: "wrong-password"},
	} {
		status, body := requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
			"username": credentials.username,
			"password": credentials.password,
		})
		if status != http.StatusUnauthorized || !bytes.Contains(body, []byte(`"code":"invalidCredentials"`)) {
			t.Fatalf("%s login status = %d, body = %s", credentials.name, status, body)
		}
	}

	status, body := requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": testUsername,
		"password": testPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", status, body)
	}
	var login loginResponse
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	if len(login.Token) != 64 || login.User.ID != user.ID || login.User.Role != domain.RoleAdmin || !login.ExpiresAt.After(time.Now()) {
		t.Fatalf("unexpected login response: %+v", login)
	}
	var rawTokenCount, digestCount int64
	if err := db.Table("sessions").Where("id = ?", login.Token).Count(&rawTokenCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("sessions").Where("id = ?", domain.SessionID(login.Token)).Count(&digestCount).Error; err != nil {
		t.Fatal(err)
	}
	if rawTokenCount != 0 || digestCount != 1 {
		t.Fatalf("session token storage raw=%d digest=%d", rawTokenCount, digestCount)
	}
	previousToken := login.Token
	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": testUsername,
		"password": testPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("replacement login status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	if login.Token == previousToken {
		t.Fatal("replacement login reused the previous token")
	}
	status, _ = requestJSONWithToken(t, server.URL+"/logout", http.MethodPost, nil, previousToken)
	if status != http.StatusUnauthorized {
		t.Fatalf("previous session status = %d", status)
	}
	var currentSessionCount int64
	if err := db.Table("sessions").Where("user_id = ? AND revoked_at IS NULL", user.ID).Count(&currentSessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if currentSessionCount != 1 {
		t.Fatalf("current sessions = %d, want 1", currentSessionCount)
	}

	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]any{
		"username":             "registered-user",
		"password":             "registered-password",
		"passwordConfirmation": "registered-password",
		"role":                 "admin",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("register role status = %d, body = %s", status, body)
	}
	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]string{
		"username":             "admin'--",
		"password":             "registered-password",
		"passwordConfirmation": "registered-password",
	})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"invalidUsername"`)) {
		t.Fatalf("unsafe username status = %d, body = %s", status, body)
	}
	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]string{
		"username":             "long-password",
		"password":             strings.Repeat("a", 251),
		"passwordConfirmation": strings.Repeat("a", 251),
	})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"invalidPassword"`)) {
		t.Fatalf("long password status = %d, body = %s", status, body)
	}
	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]string{
		"username":             "password-mismatch",
		"password":             "registered-password",
		"passwordConfirmation": "different-password",
	})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"passwordMismatch"`)) {
		t.Fatalf("registration password mismatch status = %d, body = %s", status, body)
	}
	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]string{
		"username":             "registered-user",
		"password":             "registered-password",
		"passwordConfirmation": "registered-password",
	})
	if status != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", status, body)
	}
	var registered loginResponse
	if err := json.Unmarshal(body, &registered); err != nil {
		t.Fatal(err)
	}
	if registered.Token == "" || registered.User.ID == "" || registered.User.Role != domain.RoleViewer {
		t.Fatalf("unexpected registered user: %+v", registered)
	}
	status, body = requestJSON(t, server.URL+"/register", http.MethodPost, map[string]string{
		"username":             "REGISTERED-USER",
		"password":             "registered-password",
		"passwordConfirmation": "registered-password",
	})
	if status != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"usernameTaken"`)) {
		t.Fatalf("duplicate registration status = %d, body = %s", status, body)
	}

	for _, header := range []string{"", "Bearer invalid"} {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/issues", bytes.NewBufferString(`{"title":"Unauthorized"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", header)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("authorization %q status = %d, challenge = %q", header, response.StatusCode, response.Header.Get("WWW-Authenticate"))
		}
	}
	status, _ = requestJSON(t, server.URL+"/issues", http.MethodGet, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated issue list status = %d", status)
	}
	status, _ = requestJSON(t, server.URL+"/members", http.MethodGet, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated member list status = %d", status)
	}

	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "viewer",
		"password": "viewer-password",
	})
	if status != http.StatusOK {
		t.Fatalf("viewer login status = %d, body = %s", status, body)
	}
	var viewerLogin loginResponse
	if err := json.Unmarshal(body, &viewerLogin); err != nil {
		t.Fatal(err)
	}
	status, body = requestJSONWithToken(t, server.URL+"/members", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusOK || string(body) != "[]\n" && string(body) != "[]" {
		t.Fatalf("member list status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":    "Viewer issue",
		"priority": string(domain.PriorityHigh),
		"dueDate":  "2026-08-15",
	}, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":      "Assigned issue",
		"priority":   string(domain.PriorityHigh),
		"dueDate":    "2026-08-15",
		"assigneeId": viewer.ID,
	}, login.Token)
	if status != http.StatusCreated {
		t.Fatalf("admin create assigned issue status = %d, body = %s", status, body)
	}
	var viewerIssue issueResponse
	if err := json.Unmarshal(body, &viewerIssue); err != nil {
		t.Fatal(err)
	}
	if viewerIssue.CreatedBy != user.ID || viewerIssue.AssigneeID == nil || *viewerIssue.AssigneeID != viewer.ID || viewerIssue.Priority != domain.PriorityHigh || viewerIssue.DueDate == nil || *viewerIssue.DueDate != "2026-08-15" {
		t.Fatalf("unexpected viewer issue: %+v", viewerIssue)
	}
	viewerIssueURL := server.URL + "/issues/" + strconv.FormatUint(viewerIssue.ID, 10)

	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodGet, nil, registered.Token)
	if status != http.StatusOK {
		t.Fatalf("registered user list status = %d, body = %s", status, body)
	}
	var registeredIssues issueListResponse
	if err := json.Unmarshal(body, &registeredIssues); err != nil {
		t.Fatal(err)
	}
	if len(registeredIssues.Items) != 0 || registeredIssues.Pagination.Total != 0 {
		t.Fatalf("registered user saw lateral issues: %+v", registeredIssues)
	}
	for _, attempt := range []struct {
		method  string
		payload any
	}{
		{method: http.MethodGet},
		{method: http.MethodPatch, payload: map[string]string{"title": "Stolen"}},
		{method: http.MethodDelete},
	} {
		status, body = requestJSONWithToken(t, viewerIssueURL, attempt.method, attempt.payload, registered.Token)
		if status != http.StatusNotFound {
			t.Fatalf("lateral %s status = %d, body = %s", attempt.method, status, body)
		}
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("admin lateral read status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodPatch, map[string]string{"assigneeId": registered.User.ID}, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer assignment status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodPatch, map[string]string{"title": "Changed by worker"}, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer field update status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodPatch, map[string]string{"status": string(domain.StatusClosed)}, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("viewer status update status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL+"/comments", http.MethodPost, map[string]string{"body": "Ready for review"}, viewerLogin.Token)
	if status != http.StatusCreated {
		t.Fatalf("owner comment status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL+"/activity?page=1&pageSize=50", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("activity list status = %d, body = %s", status, body)
	}
	var activity dto.ActivityListResponse
	if err := json.Unmarshal(body, &activity); err != nil {
		t.Fatal(err)
	}
	if activity.Pagination.Total < 3 || len(activity.Items) < 3 || activity.Items[0].Kind != domain.ActivityComment || activity.Items[0].Body != "Ready for review" {
		t.Fatalf("unexpected activity: %+v", activity)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodDelete, nil, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer delete status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, viewerIssueURL, http.MethodDelete, nil, login.Token)
	if status != http.StatusNoContent {
		t.Fatalf("admin delete assigned issue status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/users", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer user-list status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/account/password", http.MethodPatch, map[string]string{
		"currentPassword":      "wrong-password",
		"newPassword":          "viewer-new-password",
		"passwordConfirmation": "viewer-new-password",
	}, viewerLogin.Token)
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"invalidCurrentPassword"`)) {
		t.Fatalf("wrong current password status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/account/password", http.MethodPatch, map[string]string{
		"currentPassword":      "viewer-password",
		"newPassword":          "viewer-new-password",
		"passwordConfirmation": "viewer-new-password",
	}, viewerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("change password status = %d, body = %s", status, body)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/members", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusUnauthorized {
		t.Fatalf("password change left old session active: status = %d", status)
	}
	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "viewer",
		"password": "viewer-new-password",
	})
	if status != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &viewerLogin); err != nil {
		t.Fatal(err)
	}

	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "manager",
		"password": "manager-password",
	})
	if status != http.StatusOK {
		t.Fatalf("manager login status = %d, body = %s", status, body)
	}
	var managerLogin loginResponse
	if err := json.Unmarshal(body, &managerLogin); err != nil {
		t.Fatal(err)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams", http.MethodPost, map[string]string{"name": "Platform"}, managerLogin.Token)
	if status != http.StatusCreated {
		t.Fatalf("manager create team status = %d, body = %s", status, body)
	}
	var team dto.TeamResponse
	if err := json.Unmarshal(body, &team); err != nil {
		t.Fatal(err)
	}
	if team.ManagerID != manager.ID || team.Name != "Platform" {
		t.Fatalf("unexpected team: %+v", team)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/availableManagers", http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("available managers status = %d, body = %s", status, body)
	}
	var availableManagers []dto.MemberResponse
	if err := json.Unmarshal(body, &availableManagers); err != nil || len(availableManagers) != 2 || availableManagers[0].ID != manager.ID || availableManagers[1].ID != replacementManager.ID {
		t.Fatalf("available managers = %+v, err = %v", availableManagers, err)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/availableManagers", http.MethodGet, nil, managerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("manager available-managers status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams", http.MethodPost, map[string]string{"name": "Missing manager"}, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("admin team without manager status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams", http.MethodPost, map[string]string{"name": "Operations", "managerId": manager.ID}, login.Token)
	if status != http.StatusCreated {
		t.Fatalf("admin create managed team status = %d, body = %s", status, body)
	}
	var adminTeam dto.TeamResponse
	if err := json.Unmarshal(body, &adminTeam); err != nil || adminTeam.ManagerID != manager.ID {
		t.Fatalf("admin-created team = %+v, err = %v", adminTeam, err)
	}
	adminTeamURL := server.URL + "/teams/" + adminTeam.ID
	status, body = requestJSONWithToken(t, adminTeamURL, http.MethodPatch, map[string]string{"managerId": replacementManager.ID}, managerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("manager reassign team status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, adminTeamURL, http.MethodPatch, map[string]string{"managerId": viewer.ID}, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("assign viewer as manager status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, adminTeamURL, http.MethodPatch, map[string]string{"managerId": replacementManager.ID}, login.Token)
	if status != http.StatusOK {
		t.Fatalf("admin reassign team status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &adminTeam); err != nil || adminTeam.ManagerID != replacementManager.ID {
		t.Fatalf("reassigned team = %+v, err = %v", adminTeam, err)
	}
	var managerAssignmentNotifications int64
	if err := db.Table("notifications").Where("team_id = ? AND user_id IN ?", adminTeam.ID, []string{manager.ID, replacementManager.ID}).Count(&managerAssignmentNotifications).Error; err != nil {
		t.Fatal(err)
	}
	if managerAssignmentNotifications != 2 {
		t.Fatalf("manager reassignment notifications = %d, want 2", managerAssignmentNotifications)
	}
	status, body = requestJSONWithToken(t, server.URL+"/users/"+manager.ID, http.MethodPatch, map[string]string{"role": string(domain.RoleViewer)}, login.Token)
	if status != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"userManagesTeams"`)) {
		t.Fatalf("managed manager role update status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams", http.MethodPost, map[string]string{"name": "No access"}, viewerLogin.Token)
	if status != http.StatusForbidden {
		t.Fatalf("viewer create team status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/"+team.ID+"/members/"+viewer.ID, http.MethodPut, nil, managerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("manager add member status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/"+team.ID+"/users", http.MethodPost, map[string]string{
		"username": "team-worker",
		"password": "team-worker-password",
	}, managerLogin.Token)
	if status != http.StatusCreated {
		t.Fatalf("manager create team user status = %d, body = %s", status, body)
	}
	var teamWorker dto.UserResponse
	if err := json.Unmarshal(body, &teamWorker); err != nil {
		t.Fatal(err)
	}
	if teamWorker.TeamID == nil || *teamWorker.TeamID != team.ID || teamWorker.Role != domain.RoleViewer {
		t.Fatalf("unexpected team worker: %+v", teamWorker)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/"+team.ID+"/members", http.MethodGet, nil, managerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("team members status = %d, body = %s", status, body)
	}
	var teamMembers []dto.MemberResponse
	if err := json.Unmarshal(body, &teamMembers); err != nil {
		t.Fatal(err)
	}
	if len(teamMembers) != 3 || teamMembers[0].ID != manager.ID || teamMembers[0].Role != domain.RoleManager {
		t.Fatalf("team members = %+v", teamMembers)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/"+team.ID+"/members", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("worker team members status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &teamMembers); err != nil || len(teamMembers) != 3 || teamMembers[0].ID != manager.ID {
		t.Fatalf("worker team members = %+v, err = %v", teamMembers, err)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":  "Unassigned team issue",
		"teamId": team.ID,
	}, login.Token)
	if status != http.StatusCreated {
		t.Fatalf("admin create unassigned team issue status = %d, body = %s", status, body)
	}
	var unassignedTeamIssue issueResponse
	if err := json.Unmarshal(body, &unassignedTeamIssue); err != nil {
		t.Fatal(err)
	}
	unassignedTeamIssueURL := server.URL + "/issues/" + strconv.FormatUint(unassignedTeamIssue.ID, 10)
	status, body = requestJSONWithToken(t, unassignedTeamIssueURL, http.MethodGet, nil, managerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("manager team issue read status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, unassignedTeamIssueURL, http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusNotFound {
		t.Fatalf("unassigned team member issue status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":      "Team issue",
		"teamId":     team.ID,
		"assigneeId": viewer.ID,
		"priority":   string(domain.PriorityHigh),
	}, managerLogin.Token)
	if status != http.StatusCreated {
		t.Fatalf("manager create team issue status = %d, body = %s", status, body)
	}
	var teamIssue issueResponse
	if err := json.Unmarshal(body, &teamIssue); err != nil {
		t.Fatal(err)
	}
	if teamIssue.TeamID == nil || *teamIssue.TeamID != team.ID || teamIssue.AssigneeID == nil || *teamIssue.AssigneeID != viewer.ID {
		t.Fatalf("unexpected team issue: %+v", teamIssue)
	}
	teamIssueURL := server.URL + "/issues/" + strconv.FormatUint(teamIssue.ID, 10)
	status, body = requestJSONWithToken(t, teamIssueURL, http.MethodPatch, map[string]string{"status": string(domain.StatusClosed)}, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("assigned worker status update = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/notifications", http.MethodGet, nil, managerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("manager notification inbox status = %d, body = %s", status, body)
	}
	var managerInbox dto.NotificationListResponse
	if err := json.Unmarshal(body, &managerInbox); err != nil {
		t.Fatal(err)
	}
	managerNotified := false
	for _, notification := range managerInbox.Items {
		if notification.Kind == domain.NotificationIssueUpdated && notification.IssueID != nil && *notification.IssueID == teamIssue.ID {
			managerNotified = true
		}
	}
	if !managerNotified {
		t.Fatalf("manager did not receive assigned issue update: %+v", managerInbox)
	}
	var unassignedWorkerNotifications int64
	if err := db.Table("notifications").Where("user_id = ? AND issue_id = ?", teamWorker.ID, teamIssue.ID).Count(&unassignedWorkerNotifications).Error; err != nil {
		t.Fatal(err)
	}
	if unassignedWorkerNotifications != 0 {
		t.Fatalf("unassigned worker received %d issue notifications", unassignedWorkerNotifications)
	}
	status, body = requestJSONWithToken(t, server.URL+"/notifications", http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("notification inbox status = %d, body = %s", status, body)
	}
	var inbox dto.NotificationListResponse
	if err := json.Unmarshal(body, &inbox); err != nil {
		t.Fatal(err)
	}
	var issueNotificationID uint64
	for _, notification := range inbox.Items {
		if notification.Kind == domain.NotificationIssueCreated && notification.IssueID != nil && *notification.IssueID == teamIssue.ID {
			issueNotificationID = notification.ID
		}
	}
	if inbox.Unread < 2 || issueNotificationID == 0 {
		t.Fatalf("unexpected notification inbox: %+v", inbox)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/notifications/"+strconv.FormatUint(issueNotificationID, 10)+"/read", http.MethodPatch, nil, registered.Token)
	if status != http.StatusNotFound {
		t.Fatalf("lateral notification read status = %d", status)
	}
	status, body = requestJSONWithToken(t, server.URL+"/notifications/"+strconv.FormatUint(issueNotificationID, 10)+"/read", http.MethodPatch, nil, viewerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("mark notification read status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/notifications/readAll", http.MethodPost, nil, viewerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("mark all notifications read status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/notifications", http.MethodGet, nil, viewerLogin.Token)
	if err := json.Unmarshal(body, &inbox); err != nil || status != http.StatusOK || inbox.Unread != 0 {
		t.Fatalf("read notification inbox status = %d, inbox = %+v, err = %v", status, inbox, err)
	}
	status, body = requestJSONWithToken(t, teamIssueURL, http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusOK {
		t.Fatalf("team member issue read status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, teamIssueURL, http.MethodGet, nil, registered.Token)
	if status != http.StatusNotFound {
		t.Fatalf("unassigned viewer team issue status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":      "Invalid team assignment",
		"teamId":     team.ID,
		"assigneeId": registered.User.ID,
	}, managerLogin.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("cross-team assignment status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, server.URL+"/teams/"+team.ID+"/members/"+viewer.ID, http.MethodDelete, nil, managerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("manager remove member status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, teamIssueURL, http.MethodGet, nil, viewerLogin.Token)
	if status != http.StatusNotFound {
		t.Fatalf("removed member issue status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, teamIssueURL, http.MethodDelete, nil, managerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("manager delete team issue status = %d, body = %s", status, body)
	}
	status, body = requestJSONWithToken(t, unassignedTeamIssueURL, http.MethodDelete, nil, managerLogin.Token)
	if status != http.StatusNoContent {
		t.Fatalf("manager delete unassigned team issue status = %d, body = %s", status, body)
	}

	status, body = requestJSONWithToken(t, server.URL+"/users", http.MethodPost, map[string]string{
		"username": "managed-user",
		"password": "managed-password",
		"role":     string(domain.RoleViewer),
	}, login.Token)
	if status != http.StatusCreated {
		t.Fatalf("admin create user status = %d, body = %s", status, body)
	}
	var managed dto.UserResponse
	if err := json.Unmarshal(body, &managed); err != nil {
		t.Fatal(err)
	}
	if managed.Role != domain.RoleViewer || !managed.Active {
		t.Fatalf("unexpected managed user: %+v", managed)
	}
	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "managed-user",
		"password": "managed-password",
	})
	if status != http.StatusOK {
		t.Fatalf("managed user login status = %d, body = %s", status, body)
	}
	var managedLogin loginResponse
	if err := json.Unmarshal(body, &managedLogin); err != nil {
		t.Fatal(err)
	}

	status, body = requestJSONWithToken(t, server.URL+"/users?page=1&pageSize=2", http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("user list status = %d, body = %s", status, body)
	}
	var listedUsers dto.UserListResponse
	if err := json.Unmarshal(body, &listedUsers); err != nil {
		t.Fatal(err)
	}
	if len(listedUsers.Items) != 2 || listedUsers.Pagination.Total != 7 || listedUsers.Pagination.TotalPages != 4 {
		t.Fatalf("unexpected user page: %+v", listedUsers)
	}
	status, body = requestJSONWithToken(t, server.URL+"/users?page=5&pageSize=2", http.MethodGet, nil, login.Token)
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"pageOutOfRange"`)) {
		t.Fatalf("user page range status = %d, body = %s", status, body)
	}

	status, body = requestJSONWithToken(t, server.URL+"/users/"+managed.ID, http.MethodPatch, map[string]any{
		"role":   string(domain.RoleAdmin),
		"active": false,
	}, login.Token)
	if status != http.StatusOK {
		t.Fatalf("update user access status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &managed); err != nil {
		t.Fatal(err)
	}
	if managed.Role != domain.RoleAdmin || managed.Active {
		t.Fatalf("unexpected updated user: %+v", managed)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/logout", http.MethodPost, nil, managedLogin.Token)
	if status != http.StatusUnauthorized {
		t.Fatalf("deactivated user session status = %d", status)
	}
	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "managed-user",
		"password": "managed-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("inactive user login status = %d, body = %s", status, body)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/users/not-a-uuid", http.MethodPatch, map[string]bool{"active": true}, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid user ID status = %d", status)
	}
	missingUserID, err := domain.NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/users/"+missingUserID, http.MethodPatch, map[string]bool{"active": true}, login.Token)
	if status != http.StatusNotFound {
		t.Fatalf("missing user status = %d", status)
	}

	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]any{
		"title":      "Rejected issue",
		"unexpected": true,
	}, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown JSON field status = %d, body = %s", status, body)
	}

	injectionTitle := `'); DROP TABLE users; --`
	injectionDescription := `"; DELETE FROM sessions; --`
	status, body = requestJSONWithToken(t, server.URL+"/issues", http.MethodPost, map[string]string{
		"title":       injectionTitle,
		"description": injectionDescription,
	}, login.Token)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", status, body)
	}
	var issue issueResponse
	if err := json.Unmarshal(body, &issue); err != nil {
		t.Fatal(err)
	}
	if issue.ID == 0 || issue.Title != injectionTitle || issue.Description != injectionDescription || issue.Status != domain.StatusOpen || issue.CreatedBy != user.ID {
		t.Fatalf("unexpected created issue: %+v", issue)
	}
	if !db.Migrator().HasTable("users") || !db.Migrator().HasTable("sessions") {
		t.Fatal("SQL-shaped issue text affected auth tables")
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/issues", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+login.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
	if response.Header.Get("X-Request-ID") == "" {
		t.Fatal("request ID middleware did not set X-Request-ID")
	}
	var listed issueListResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(listed.Items) != 1 || listed.Items[0].ID != issue.ID {
		t.Fatalf("unexpected issue list: %+v", listed.Items)
	}
	if listed.Pagination.Page != 1 || listed.Pagination.PageSize != 20 || listed.Pagination.Total != 1 {
		t.Fatalf("unexpected pagination: %+v", listed.Pagination)
	}
	operationID := testIssueExport(t, server.URL, login.Token, registered.Token, issue)
	for _, table := range []string{"vial_async_operations", "issue_exports"} {
		var count int64
		column := "operation_id"
		if table == "vial_async_operations" {
			column = "id"
		}
		if err := db.Table(table).Where(column+" = ?", operationID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s operation rows = %d, want 1", table, count)
		}
	}

	issueURL := server.URL + "/issues/" + strconv.FormatUint(issue.ID, 10)
	status, body = requestJSONWithToken(t, issueURL, http.MethodPatch, map[string]string{"status": string(domain.StatusClosed)}, login.Token)
	if status != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &issue); err != nil {
		t.Fatal(err)
	}
	if issue.Status != domain.StatusClosed {
		t.Fatalf("updated status = %q", issue.Status)
	}

	status, body = requestJSONWithToken(t, issueURL, http.MethodDelete, nil, login.Token)
	if status != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", status, body)
	}
	status, _ = requestJSONWithToken(t, issueURL, http.MethodGet, nil, login.Token)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted issue status = %d", status)
	}

	created, err := issues.Seed(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if created != 7 {
		t.Fatalf("seeded issues = %d, want 7", created)
	}
	created, err = issues.Seed(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("second seed created %d duplicate issues", created)
	}

	status, body = requestJSONWithToken(t, server.URL+"/issues?status=open&page=1&pageSize=2", http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("filtered list status = %d, body = %s", status, body)
	}
	var filtered issueListResponse
	if err := json.Unmarshal(body, &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 2 || filtered.Pagination.Total != 3 || filtered.Pagination.TotalPages != 2 {
		t.Fatalf("unexpected filtered page: %+v", filtered)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues?status=open&page=2&pageSize=2", http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("second filtered page status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Pagination.Page != 2 {
		t.Fatalf("unexpected second filtered page: %+v", filtered)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues?status=open&page=20&pageSize=2", http.MethodGet, nil, login.Token)
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"pageOutOfRange"`)) {
		t.Fatalf("out-of-range page status = %d, body = %s", status, body)
	}

	status, _ = requestJSONWithToken(t, server.URL+"/issues?page=invalid", http.MethodGet, nil, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid bound page status = %d", status)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/issues?status=invalid", http.MethodGet, nil, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid status filter status = %d", status)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/issues?pageSize=101", http.MethodGet, nil, login.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("oversized page status = %d", status)
	}
	status, body = requestJSONWithToken(t, server.URL+"/issues?search=authentication&priority=medium&sort=title&order=asc&page=1&pageSize=10", http.MethodGet, nil, login.Token)
	if status != http.StatusOK {
		t.Fatalf("searched issue list status = %d, body = %s", status, body)
	}
	if err := json.Unmarshal(body, &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].Title != "Add authentication" {
		t.Fatalf("unexpected searched issues: %+v", filtered.Items)
	}
	for _, query := range []string{"priority=urgent", "sort=created_at", "order=sideways"} {
		status, _ = requestJSONWithToken(t, server.URL+"/issues?"+query, http.MethodGet, nil, login.Token)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid filter %q status = %d", query, status)
		}
	}

	status, body = requestJSONWithToken(t, server.URL+"/users/"+registered.User.ID+"/password", http.MethodPatch, map[string]string{
		"newPassword":          "registered-new-password",
		"passwordConfirmation": "registered-new-password",
	}, login.Token)
	if status != http.StatusNoContent {
		t.Fatalf("admin password reset status = %d, body = %s", status, body)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/members", http.MethodGet, nil, registered.Token)
	if status != http.StatusUnauthorized {
		t.Fatalf("password reset left old session active: status = %d", status)
	}
	status, body = requestJSON(t, server.URL+"/login", http.MethodPost, map[string]string{
		"username": "registered-user",
		"password": "registered-new-password",
	})
	if status != http.StatusOK {
		t.Fatalf("reset password login status = %d, body = %s", status, body)
	}

	status, body = requestJSONWithToken(t, server.URL+"/logout", http.MethodPost, nil, login.Token)
	if status != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", status, body)
	}
	status, _ = requestJSONWithToken(t, server.URL+"/logout", http.MethodPost, nil, login.Token)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", status)
	}
	var revokedAt *time.Time
	if err := db.Table("sessions").Select("revoked_at").Where("id = ?", domain.SessionID(login.Token)).Scan(&revokedAt).Error; err != nil {
		t.Fatal(err)
	}
	if revokedAt == nil {
		t.Fatal("logout did not revoke the session")
	}
}

func testIssueExport(t *testing.T, serverURL, token, otherToken string, issue issueResponse) string {
	t.Helper()
	submit := func() operationResponse {
		req, err := http.NewRequest(http.MethodPost, serverURL+"/exports/issues", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "issue-export-integration")
		req.Header.Set("Prefer", "respond-async")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusAccepted || response.Header.Get("Location") == "" {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("export submission status = %d, body = %s", response.StatusCode, body)
		}
		var operation operationResponse
		if err := json.NewDecoder(response.Body).Decode(&operation); err != nil {
			t.Fatal(err)
		}
		return operation
	}

	operation := submit()
	duplicate := submit()
	if operation.ID == "" || operation.StatusURL == "" || duplicate.ID != operation.ID {
		t.Fatalf("idempotent export operations = %+v, %+v", operation, duplicate)
	}
	statusURL := serverURL + operation.StatusURL
	if status, _ := requestJSON(t, statusURL, http.MethodGet, nil); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated operation status = %d", status)
	}
	if status, _ := requestJSONWithToken(t, statusURL, http.MethodGet, nil, otherToken); status != http.StatusNotFound {
		t.Fatalf("other owner operation status = %d", status)
	}

	eventRequest, err := http.NewRequest(http.MethodGet, statusURL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	eventRequest.Header.Set("Authorization", "Bearer "+token)
	eventResponse, err := http.DefaultClient.Do(eventRequest)
	if err != nil {
		t.Fatal(err)
	}
	if eventResponse.StatusCode != http.StatusOK || !strings.Contains(eventResponse.Header.Get("Content-Type"), "text/event-stream") {
		_ = eventResponse.Body.Close()
		t.Fatalf("operation events status = %d, type = %q", eventResponse.StatusCode, eventResponse.Header.Get("Content-Type"))
	}
	events := make(chan []byte, 1)
	go func() {
		body, _ := io.ReadAll(eventResponse.Body)
		_ = eventResponse.Body.Close()
		events <- body
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains([]string{"succeeded", "failed", "cancelled"}, operation.Status) && time.Now().Before(deadline) {
		status, body := requestJSONWithToken(t, statusURL, http.MethodGet, nil, token)
		if status != http.StatusOK {
			t.Fatalf("operation poll status = %d, body = %s", status, body)
		}
		if err := json.Unmarshal(body, &operation); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains([]string{"succeeded", "failed", "cancelled"}, operation.Status) {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if operation.Status != "succeeded" || operation.Progress != 100 || operation.MaxAttempts != 3 || operation.Result == nil || operation.Result.Rows < 1 {
		t.Fatalf("completed export operation = %+v", operation)
	}
	select {
	case body := <-events:
		if !bytes.Contains(body, []byte("event: completion")) || !bytes.Contains(body, []byte(operation.ID)) {
			t.Fatalf("unexpected operation events: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation completion event timed out")
	}

	status, body := requestJSONWithToken(t, serverURL+operation.Result.DownloadURL, http.MethodGet, nil, token)
	if status != http.StatusOK {
		t.Fatalf("export download status = %d, body = %s", status, body)
	}
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 || rows[0][0] != "id" || rows[1][0] != strconv.FormatUint(issue.ID, 10) || rows[1][1] != issue.Title {
		t.Fatalf("unexpected export CSV: %#v", rows)
	}
	if status, _ := requestJSONWithToken(t, statusURL, http.MethodDelete, nil, token); status != http.StatusNoContent {
		t.Fatalf("completed operation cancellation status = %d", status)
	}
	return operation.ID
}

func requestJSON(t *testing.T, url, method string, payload any) (int, []byte) {
	return requestJSONWithToken(t, url, method, payload, "")
}

func requestJSONWithToken(t *testing.T, url, method string, payload any, token string) (int, []byte) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}
