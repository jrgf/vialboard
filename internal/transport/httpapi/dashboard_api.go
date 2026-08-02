package httpapi

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/go-vial/render"
)

//go:embed dashboard/templates/*.html dashboard/static/*
var dashboardFiles embed.FS

var (
	dashboardViews  = render.New(template.Must(template.New("dashboard").ParseFS(dashboardFiles, "dashboard/templates/*.html")))
	dashboardAssets = mustSub(dashboardFiles, "dashboard/static")
)

type dashboardAPI struct {
	views *render.Renderer
}

type dashboardPage struct {
	Title string
}

func (api dashboardAPI) register(app *vial.App) {
	app.Get("/", api.index, vial.RouteName("dashboard"))

	assets := http.StripPrefix("/dashboard/static/", http.FileServerFS(dashboardAssets))
	app.HandleHTTP("GET /dashboard/static/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		assets.ServeHTTP(writer, request)
	}), vial.RouteName("dashboard.static"))
}

func (api dashboardAPI) index(context *vial.Context) error {
	headers := context.Response().Header()
	headers.Set("Cache-Control", "no-store")
	headers.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("X-Content-Type-Options", "nosniff")
	return api.views.HTML(context, http.StatusOK, "dashboard", dashboardPage{Title: "Vialboard"})
}

func mustSub(files fs.FS, directory string) fs.FS {
	assets, err := fs.Sub(files, directory)
	if err != nil {
		panic(err)
	}
	return assets
}
