package httpapi

import (
	"context"
	"database/sql"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/go-vial/contrib/asyncpostgres"
	"github.com/jrgf/go-vial/middleware"
	"github.com/jrgf/go-vial/sse"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

func New(issues *application.IssueService, users *application.UserService, teams *application.TeamService, notifications *application.NotificationService, database *sql.DB, options ...vial.Option) *vial.App {
	options = append([]vial.Option{vial.WithDisallowUnknownJSONFields(true)}, options...)
	app := vial.New(options...)
	executor := asyncpostgres.New(database)
	exports := issueExportAPI{issues: issues, users: users, store: issueExportStore{database: database}, executor: executor}
	executor.Handle(issueExportOperation, exports.generate)
	app.Async(executor)
	if err := app.Register(apiModule{issues: issues, users: users, teams: teams, notifications: notifications, database: database, exports: exports}); err != nil {
		panic(err)
	}
	return app
}

type apiModule struct {
	issues        *application.IssueService
	users         *application.UserService
	teams         *application.TeamService
	notifications *application.NotificationService
	database      *sql.DB
	exports       issueExportAPI
}

func (apiModule) Name() string { return "api" }

func (module apiModule) Register(registrar *vial.Registrar) error {
	registrar.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recover(),
	)

	authenticated := authenticate(module.users)
	limited, err := middleware.RateLimit(middleware.RateLimitConfig{
		Requests: 10,
		Window:   time.Minute,
		Key: func(context *vial.Context) (string, error) {
			address, err := context.ClientIP()
			if err != nil {
				return "", vial.BadRequest("invalidClientIP", "Invalid client address")
			}
			return address.String() + " " + context.Request().URL.Path, nil
		},
	})
	if err != nil {
		return err
	}
	// ponytail: live fan-out is per process; replace the hub with Redis pub/sub before running multiple replicas.
	notificationHub, err := sse.NewHub(sse.HubConfig{SlowConsumer: sse.DropEvent})
	if err != nil {
		return err
	}
	registrar.OnStop(func(context.Context) error {
		notificationHub.Close()
		return nil
	})
	public := registrar.Group("")
	protected := registrar.Group("")
	protected.Use(authenticated)
	admin := protected.Group("")
	admin.Use(requireRole(domain.RoleAdmin))

	dashboardAPI{views: dashboardViews}.register(public)
	registrar.Health("/health/live")
	registrar.Readiness("/health/ready", func(ctx context.Context) error {
		if err := module.database.PingContext(ctx); err != nil {
			return err
		}
		return module.exports.executor.Ready(ctx)
	})
	authAPI{users: module.users}.register(public, protected, limited)
	usersAPI{users: module.users}.register(public, protected, admin, limited)
	teamsAPI{teams: module.teams, users: module.users}.register(protected)
	issuesAPI{issues: module.issues}.register(protected)
	module.exports.register(protected)
	notificationsAPI{notifications: module.notifications, users: module.users, hub: notificationHub}.register(protected)
	registrar.Go("notifications", notificationBroadcaster(module.notifications, notificationHub))
	return nil
}
