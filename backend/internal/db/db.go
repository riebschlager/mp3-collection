package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB opens a connection to the SQLite database and runs the schema file.
func InitDB(dbPath string, schemaPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Read schema file
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	// Execute schema
	_, err = db.Exec(string(schemaBytes))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	migrations := []struct {
		table      string
		column     string
		definition string
	}{
		{"artists", "image_url", "TEXT"},
		{"albums", "image_url", "TEXT"},
	}

	for _, migration := range migrations {
		exists, err := columnExists(db, migration.table, migration.column)
		if err != nil {
			return fmt.Errorf("check column %s.%s: %w", migration.table, migration.column, err)
		}
		if exists {
			continue
		}

		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", migration.table, migration.column, migration.definition)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("migrate column %s.%s: %w", migration.table, migration.column, err)
		}
	}

	return nil
}

func columnExists(db *sql.DB, table string, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	return false, nil
}
