package httpapi

import (
	"database/sql"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/go-vial/middleware"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

func New(issues *application.IssueService, users *application.UserService, teams *application.TeamService, notifications *application.NotificationService, database *sql.DB) *vial.App {
	app := vial.New(vial.WithDisallowUnknownJSONFields(true))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recover(),
	)

	authenticated := authenticate(users)
	limited := newRequestLimiter(10, time.Minute).middleware
	// ponytail: live fan-out is per process; replace the hub with Redis pub/sub before running multiple replicas.
	notificationHub := newNotificationHub()
	dashboardAPI{views: dashboardViews}.register(app)
	healthAPI{database: database}.register(app)
	authAPI{users: users}.register(app, authenticated, limited)
	usersAPI{users: users}.register(app, authenticated, requireRole(domain.RoleAdmin), limited)
	teamsAPI{teams: teams, users: users}.register(app, authenticated)
	issuesAPI{issues: issues}.register(app, authenticated)
	notificationsAPI{notifications: notifications, users: users, hub: notificationHub}.register(app, authenticated)
	app.Go("notifications", notificationBroadcaster(notifications, notificationHub))
	return app
}
