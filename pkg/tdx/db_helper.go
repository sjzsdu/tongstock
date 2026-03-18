package tdx

import (
	"database/sql"

	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/db"
)

func openDatabase(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = config.Get().Database.DSN
	}
	return db.OpenFromConfig(config.Get().Database.Driver, dbPath)
}
