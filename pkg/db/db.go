package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

// Open opens a database connection based on the driver name and DSN.
// driver: "sqlite3", "postgres", "mysql", etc.
// dsn: database-specific connection string
func Open(driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// OpenSQLite opens a SQLite database
func OpenSQLite(dsn string) (*sql.DB, error) {
	return Open("sqlite3", dsn)
}

// OpenPostgres opens a PostgreSQL database
func OpenPostgres(dsn string) (*sql.DB, error) {
	return Open("postgres", dsn)
}

// OpenMySQL opens a MySQL database
func OpenMySQL(dsn string) (*sql.DB, error) {
	return Open("mysql", dsn)
}

// OpenFromConfig opens a database based on driver and DSN from config.
// If dsn is empty, uses default path for SQLite.
func OpenFromConfig(driver, dsn string) (*sql.DB, error) {
	switch driver {
	case "postgres":
		return OpenPostgres(dsn)
	case "mysql":
		return OpenMySQL(dsn)
	default:
		if dsn == "" {
			dsn = "tongstock.db"
		}
		return OpenSQLite(dsn + "?cache=shared")
	}
}
