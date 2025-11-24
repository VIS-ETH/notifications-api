package database

import (
	"errors"
	"fmt"

	stdsql "database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/sirupsen/logrus"
)

func MigrateDB(dsnFlag, migrationsDir *string) error {
	// Run database migrations using sql-migrate before starting services.
	migDB, err := stdsql.Open("pgx", *dsnFlag)
	if err != nil {
		return fmt.Errorf("failed to open db for migrations: %v", err)
	}

	// Before function returns, add potentially failed DB closing to err
	defer func() { err = errors.Join(err, migDB.Close()) }()

	migrations := &migrate.FileMigrationSource{Dir: *migrationsDir}
	applied, err := migrate.Exec(migDB, "postgres", migrations, migrate.Up)
	if err != nil {
		return fmt.Errorf("database migration failed: %v", err)
	}
	if applied > 0 {
		logrus.Infof("applied %d migration(s)", applied)
	}

	return err
}
