package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	vial "github.com/jrgf/go-vial"
)

type healthAPI struct {
	database *sql.DB
}

func (api healthAPI) register(app *vial.App) {
	app.Get("/health/live", api.live)
	app.Get("/health/ready", api.ready)
}

func (healthAPI) live(context *vial.Context) error {
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (api healthAPI) ready(contextValue *vial.Context) error {
	ctx, cancel := context.WithTimeout(contextValue.Request().Context(), 2*time.Second)
	defer cancel()
	if err := api.database.PingContext(ctx); err != nil {
		return contextValue.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
	}
	return contextValue.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
