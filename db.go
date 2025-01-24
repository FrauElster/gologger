package gologger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

type DbConfig struct {
	TableName  string
	DB         *sql.DB
	TimeFormat string
	LabelsMap  map[string]string
}

type dialectQueries struct {
	createTableSQL string
	insertLogSQL   string
}

var db *sql.DB

func getDialectQueries(dialect string, tableName string) (dialectQueries, error) {
	switch dialect {
	case "mysql":
		return dialectQueries{
			createTableSQL: fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS %s (
					id BIGINT AUTO_INCREMENT PRIMARY KEY,
					timestamp DATETIME NOT NULL,
					level VARCHAR(10) NOT NULL,
					message TEXT NOT NULL,
					labels JSON,
					fields JSON
				)`, tableName),
			insertLogSQL: fmt.Sprintf(`
				INSERT INTO %s (timestamp, level, message, labels, fields)
				VALUES (?, ?, ?, ?, ?)`, tableName),
		}, nil

	case "postgres":
		return dialectQueries{
			createTableSQL: fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS %s (
					id BIGSERIAL PRIMARY KEY,
					timestamp TIMESTAMP NOT NULL,
					level VARCHAR(10) NOT NULL,
					message TEXT NOT NULL,
					labels JSONB,
					fields JSONB
				)`, tableName),
			insertLogSQL: fmt.Sprintf(`
				INSERT INTO %s (timestamp, level, message, labels, fields)
				VALUES ($1, $2, $3, $4, $5)`, tableName),
		}, nil

	case "sqlite":
		return dialectQueries{
			createTableSQL: fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS %s (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					timestamp DATETIME NOT NULL,
					level VARCHAR(10) NOT NULL,
					message TEXT NOT NULL,
					labels TEXT,
					fields TEXT
				)`, tableName),
			insertLogSQL: fmt.Sprintf(`
				INSERT INTO %s (timestamp, level, message, labels, fields)
				VALUES (?, ?, ?, ?, ?)`, tableName),
		}, nil

	case "mssql":
		return dialectQueries{
			createTableSQL: fmt.Sprintf(`
				IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='%s' AND xtype='U')
				CREATE TABLE %s (
					id BIGINT IDENTITY(1,1) PRIMARY KEY,
					timestamp DATETIME NOT NULL,
					level VARCHAR(10) NOT NULL,
					message TEXT NOT NULL,
					labels NVARCHAR(MAX),
					fields NVARCHAR(MAX)
				)`, tableName, tableName),
			insertLogSQL: fmt.Sprintf(`
				INSERT INTO %s (timestamp, level, message, labels, fields)
				VALUES (@p1, @p2, @p3, @p4, @p5)`, tableName),
		}, nil

	default:
		return dialectQueries{}, fmt.Errorf("unsupported dialect: %s", dialect)
	}
}

func setupDbLogger(cfg DbConfig, dialect string) error {
	if cfg.DB == nil {
		return fmt.Errorf("database connection cannot be nil")
	}

	if cfg.TableName == "" {
		return fmt.Errorf("table name cannot be empty")
	}

	queries, err := getDialectQueries(dialect, cfg.TableName)
	if err != nil {
		return err
	}

	if _, err := cfg.DB.Exec(queries.createTableSQL); err != nil {
		return fmt.Errorf("failed to create log table: %w", err)
	}

	db = cfg.DB

	if cfg.TimeFormat == "" {
		cfg.TimeFormat = time.RFC3339
	}

	if cfg.LabelsMap == nil {
		cfg.LabelsMap = make(map[string]string)
	}

	writeToDb := func(level slog.Level, msg string, args ...any) {
		timestamp := time.Now().Format(cfg.TimeFormat)

		// Convert labels to JSON string
		labelsJSON, err := json.Marshal(cfg.LabelsMap)
		if err != nil {
			slog.Error("Failed to marshal labels to JSON", "error", err, "labels", cfg.LabelsMap)
			return
		}

		// Parse args into fields map and convert to JSON
		fields := make(map[string]any)
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fields[fmt.Sprint(args[i])] = args[i+1]
			}
		}
		fieldsJSON, err := json.Marshal(fields)
		if err != nil {
			slog.Error("Failed to marshal fields to JSON", "error", err, "fields", fields)
			return
		}

		_, err = db.Exec(queries.insertLogSQL,
			timestamp,
			levelToString(level),
			msg,
			string(labelsJSON),
			string(fieldsJSON),
		)
		if err != nil {
			slog.Error("Failed to write to database", "error", err, "message", msg, "level", levelToString(level))
		}
	}

	// Register callbacks for all levels
	RegisterCallback(slog.LevelDebug, func(msg string, args ...any) {
		writeToDb(slog.LevelDebug, msg, args...)
	})
	RegisterCallback(slog.LevelInfo, func(msg string, args ...any) {
		writeToDb(slog.LevelInfo, msg, args...)
	})
	RegisterCallback(slog.LevelWarn, func(msg string, args ...any) {
		writeToDb(slog.LevelWarn, msg, args...)
	})
	RegisterCallback(slog.LevelError, func(msg string, args ...any) {
		writeToDb(slog.LevelError, msg, args...)
	})

	return nil
}

// UseMysqlDb sets up logging to a MySQL database
func UseMysqlDb(cfg DbConfig) error {
	return setupDbLogger(cfg, "mysql")
}

// UsePostgresDb sets up logging to a PostgreSQL database
func UsePostgresDb(cfg DbConfig) error {
	return setupDbLogger(cfg, "postgres")
}

// UseSqlite sets up logging to a SQLite database
func UseSqlite(cfg DbConfig) error {
	return setupDbLogger(cfg, "sqlite")
}

// UseMssqlDb sets up logging to a Microsoft SQL Server database
func UseMssqlDb(cfg DbConfig) error {
	return setupDbLogger(cfg, "mssql")
}
