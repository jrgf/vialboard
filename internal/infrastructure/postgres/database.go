package postgres

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Open(databaseURL string) (*gorm.DB, *sql.DB, error) {
	db, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	return db, sqlDB, nil
}

func MigrateUp(ctx context.Context, db *sql.DB) error {
	if err := configureMigrations(); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, migrationsDir)
}

func MigrateDown(ctx context.Context, db *sql.DB) error {
	if err := configureMigrations(); err != nil {
		return err
	}
	return goose.DownContext(ctx, db, migrationsDir)
}

func configureMigrations() error {
	goose.SetBaseFS(migrationFiles)
	return goose.SetDialect("postgres")
}
