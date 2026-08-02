package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jrgf/vialboard/internal/application"
	passwordhash "github.com/jrgf/vialboard/internal/infrastructure/password"
	postgresstore "github.com/jrgf/vialboard/internal/infrastructure/postgres"
	"github.com/jrgf/vialboard/internal/transport/httpapi"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			log.Fatal(err)
		}
		return
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, sqlDB, err := postgresstore.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	ctx := context.Background()
	userRepository := postgresstore.NewUserRepository(db)
	teamRepository := postgresstore.NewTeamRepository(db)
	notifications := application.NewNotificationService(postgresstore.NewNotificationRepository(db))
	teams := application.NewTeamService(teamRepository, userRepository, notifications)
	issues := application.NewIssueService(postgresstore.NewIssueRepository(db), userRepository, teams, notifications)
	users := application.NewUserService(userRepository, passwordhash.Hasher{})

	if len(os.Args) > 1 {
		switch {
		case len(os.Args) == 2 && os.Args[1] == "seed":
			if err := postgresstore.MigrateUp(ctx, sqlDB); err != nil {
				log.Fatal(err)
			}
			ownerUsername := os.Getenv("VIAL_SEED_OWNER_USERNAME")
			if ownerUsername == "" {
				ownerUsername = os.Getenv("VIAL_USER_USERNAME")
			}
			owner, err := users.GetByUsername(ctx, ownerUsername)
			if err != nil {
				log.Fatal(err)
			}
			created, err := issues.Seed(ctx, owner)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("seeded %d issues", created)
		case len(os.Args) == 3 && os.Args[1] == "user" && os.Args[2] == "create":
			if err := postgresstore.MigrateUp(ctx, sqlDB); err != nil {
				log.Fatal(err)
			}
			user, err := users.Create(ctx, os.Getenv("VIAL_USER_USERNAME"), os.Getenv("VIAL_USER_PASSWORD"), os.Getenv("VIAL_USER_ROLE"))
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("created user id=%s username=%s role=%s", user.ID, user.Username, user.Role)
		case len(os.Args) == 3 && os.Args[1] == "migrate":
			var err error
			switch os.Args[2] {
			case "up":
				err = postgresstore.MigrateUp(ctx, sqlDB)
			case "down":
				err = postgresstore.MigrateDown(ctx, sqlDB)
			default:
				log.Fatal("usage: vialboard [healthcheck | seed | user create | migrate up | migrate down]")
			}
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("migration %s complete", os.Args[2])
		default:
			log.Fatal("usage: vialboard [healthcheck | seed | user create | migrate up | migrate down]")
		}
		return
	}

	if err := postgresstore.MigrateUp(ctx, sqlDB); err != nil {
		log.Fatal(err)
	}
	app := httpapi.New(issues, users, teams, notifications, sqlDB)
	if err := app.Run(ctx, httpAddress()); err != nil {
		log.Fatal(err)
	}
}

func healthcheck() error {
	port := os.Getenv("VIAL_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/health/ready")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness status %d", response.StatusCode)
	}
	return nil
}

func httpAddress() string {
	host := os.Getenv("VIAL_HTTP_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("VIAL_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	return net.JoinHostPort(host, port)
}
