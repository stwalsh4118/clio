package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFile represents a migration file
type migrationFile struct {
	version int
	name    string
	upSQL   string
	downSQL string
}

// RunMigrations runs all pending migrations using the provided database connection
// Reads migration files directly from embed.FS and executes them using the database connection
// This works with any database/sql driver (including pure Go drivers like modernc.org/sqlite)
func RunMigrations(db *sql.DB) error {
	// Get current migration version
	currentVersion, dirty, err := getMigrationVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in a dirty migration state (version %d), manual intervention required", currentVersion)
	}

	// Load all migration files
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Run pending migrations
	for _, migration := range migrations {
		if migration.version <= currentVersion {
			continue // Skip already applied migrations
		}

		// Execute migration in a transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.version, err)
		}

		// Execute migration SQL
		if _, err := tx.Exec(migration.upSQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d (%s): %w", migration.version, migration.name, err)
		}

		// Record migration version
		if err := setMigrationVersion(tx, migration.version, false); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.version, err)
		}
	}

	return nil
}

// loadMigrations loads all migration files from embed.FS
// Loads both .up.sql and .down.sql files
func loadMigrations() ([]migrationFile, error) {
	// Patterns to match migration files
	upPattern := regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)
	downPattern := regexp.MustCompile(`^(\d+)_(.+)\.down\.sql$`)

	// Map to collect migrations by version
	migrationMap := make(map[int]*migrationFile)

	// Walk migration files
	err := fs.WalkDir(migrationsFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		var version int
		var name string
		var isUp bool

		// Check if it's an up migration
		if matches := upPattern.FindStringSubmatch(d.Name()); len(matches) == 3 {
			version, err = strconv.Atoi(matches[1])
			if err != nil {
				return fmt.Errorf("invalid migration version in %s: %w", d.Name(), err)
			}
			name = matches[2]
			isUp = true
		} else if matches := downPattern.FindStringSubmatch(d.Name()); len(matches) == 3 {
			// Check if it's a down migration
			version, err = strconv.Atoi(matches[1])
			if err != nil {
				return fmt.Errorf("invalid migration version in %s: %w", d.Name(), err)
			}
			name = matches[2]
			isUp = false
		} else {
			// Skip files that don't match either pattern
			return nil
		}

		// Get or create migration entry
		migration, exists := migrationMap[version]
		if !exists {
			migration = &migrationFile{
				version: version,
				name:    name,
			}
			migrationMap[version] = migration
		}

		// Read migration SQL
		file, err := migrationsFS.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open migration file %s: %w", path, err)
		}
		defer file.Close()

		sqlBytes, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", path, err)
		}

		// Store SQL in appropriate field
		if isUp {
			migration.upSQL = string(sqlBytes)
		} else {
			migration.downSQL = string(sqlBytes)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert map to slice and sort by version
	migrations := make([]migrationFile, 0, len(migrationMap))
	for _, migration := range migrationMap {
		migrations = append(migrations, *migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// RollbackMigrations rolls back the specified number of migrations (default: 1)
// If count is 0 or negative, rolls back 1 migration
// Returns the version after rollback, or error if rollback fails
func RollbackMigrations(db *sql.DB, count int) (int, error) {
	if count <= 0 {
		count = 1 // Default to rolling back 1 migration
	}

	// Get current migration version
	currentVersion, dirty, err := getMigrationVersion(db)
	if err != nil {
		return 0, fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		return 0, fmt.Errorf("database is in a dirty migration state (version %d), manual intervention required", currentVersion)
	}

	if currentVersion == 0 {
		return 0, fmt.Errorf("no migrations to rollback")
	}

	// Load all migration files
	migrations, err := loadMigrations()
	if err != nil {
		return 0, fmt.Errorf("failed to load migrations: %w", err)
	}

	// Find migrations to rollback (in reverse order)
	migrationsToRollback := make([]migrationFile, 0, count)
	for i := len(migrations) - 1; i >= 0 && len(migrationsToRollback) < count; i-- {
		migration := migrations[i]
		if migration.version <= currentVersion && migration.downSQL != "" {
			migrationsToRollback = append(migrationsToRollback, migration)
		}
	}

	if len(migrationsToRollback) == 0 {
		return currentVersion, fmt.Errorf("no migrations found to rollback")
	}

	// Rollback migrations in reverse order (newest first)
	for i := len(migrationsToRollback) - 1; i >= 0; i-- {
		migration := migrationsToRollback[i]

		// Execute rollback in a transaction
		tx, err := db.Begin()
		if err != nil {
			return currentVersion, fmt.Errorf("failed to begin transaction for rollback %d: %w", migration.version, err)
		}

		// Execute down migration SQL
		if _, err := tx.Exec(migration.downSQL); err != nil {
			tx.Rollback()
			return currentVersion, fmt.Errorf("failed to execute rollback %d (%s): %w", migration.version, migration.name, err)
		}

		// Remove migration version record
		if err := removeMigrationVersion(tx, migration.version); err != nil {
			tx.Rollback()
			return currentVersion, fmt.Errorf("failed to remove migration version %d: %w", migration.version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return currentVersion, fmt.Errorf("failed to commit rollback %d: %w", migration.version, err)
		}

		currentVersion = migration.version - 1
	}

	return currentVersion, nil
}

// removeMigrationVersion removes a migration version from the database
func removeMigrationVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", version)
	return err
}

// getMigrationVersion gets the current migration version from the database
func getMigrationVersion(db *sql.DB) (version int, dirty bool, err error) {
	// Ensure schema_migrations table exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER NOT NULL PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get current version
	var v sql.NullInt64
	var d sql.NullBool
	err = db.QueryRow("SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&v, &d)
	if err != nil {
		if err == sql.ErrNoRows {
			// No migrations applied yet
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to query migration version: %w", err)
	}

	version = int(v.Int64)
	if d.Valid {
		dirty = d.Bool
	}

	return version, dirty, nil
}

// setMigrationVersion records a migration version in the database
func setMigrationVersion(tx *sql.Tx, version int, dirty bool) error {
	// Use INSERT OR REPLACE to handle both new and existing versions
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO schema_migrations (version, dirty)
		VALUES (?, ?)
	`, version, dirty)
	return err
}
