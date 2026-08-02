package httpapi

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestDashboardModulesAreEmbedded(t *testing.T) {
	assets := []string{
		"app.js", "core.js", "auth.js", "issues.js", "users.js", "teams.js", "notifications.js",
		"styles.css", "base.css", "auth.css", "shell.css", "issues.css", "users.css", "teams.css", "notifications.css", "dialogs.css", "responsive.css",
	}
	for _, asset := range assets {
		if _, err := fs.Stat(dashboardAssets, asset); err != nil {
			t.Fatalf("dashboard asset %q: %v", asset, err)
		}
	}

	styles, err := fs.ReadFile(dashboardAssets, "styles.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, stylesheet := range assets[8:] {
		if !strings.Contains(string(styles), `"./`+stylesheet+`"`) {
			t.Fatalf("styles.css does not import %s", stylesheet)
		}
	}

	page, err := dashboardFiles.ReadFile("dashboard/templates/dashboard.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `type="module"`) {
		t.Fatal("dashboard entry script is not an ES module")
	}
	for _, control := range []string{`id="available-user-search"`, `id="team-manager-search"`, `id="manager-options"`, `id="password-confirmation"`, `id="replacement-password-confirmation"`} {
		if !strings.Contains(string(page), control) {
			t.Fatalf("dashboard is missing searchable team control %s", control)
		}
	}

	core, err := fs.ReadFile(dashboardAssets, "core.js")
	if err != nil {
		t.Fatal(err)
	}
	registry := regexp.MustCompile(`(?s)Object\.fromEntries\(\[(.*?)\]\.map`).FindSubmatch(core)
	if len(registry) != 2 {
		t.Fatal("dashboard element registry was not found")
	}
	registered := make(map[string]bool)
	for _, match := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(registry[1], -1) {
		registered[string(match[1])] = true
	}
	usage := regexp.MustCompile(`elements\["([^"]+)"\]`)
	for _, asset := range assets[:7] {
		script, err := fs.ReadFile(dashboardAssets, asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range usage.FindAllSubmatch(script, -1) {
			if !registered[string(match[1])] {
				t.Fatalf("%s uses unregistered element %q", asset, match[1])
			}
		}
	}

	notifications, err := fs.ReadFile(dashboardAssets, "notifications.js")
	if err != nil {
		t.Fatal(err)
	}
	app, err := fs.ReadFile(dashboardAssets, "app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notifications), `new CustomEvent("notification-navigation"`) ||
		!strings.Contains(string(app), `openIssueByID(event.detail.issueId)`) ||
		!strings.Contains(string(app), `state.selectedTeamID = event.detail.teamId`) {
		t.Fatal("notification destinations are not wired")
	}
}
