package toolbox

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/AKhilRaghav0/hamr"
)

// DatabaseTools is a collection of read-only SQL database tools.
// It works with any database/sql-compatible driver.
type DatabaseTools struct {
	db *sql.DB
}

// Database returns a DatabaseTools collection backed by db.
// The query tool only permits SELECT statements; write operations are rejected.
func Database(db *sql.DB) *DatabaseTools {
	return &DatabaseTools{db: db}
}

// Tools implements mcpx.ToolCollection.
func (d *DatabaseTools) Tools() []mcpx.ToolInfo {
	return []mcpx.ToolInfo{
		{
			Name:        "query",
			Description: "Execute a read-only SQL SELECT query and return the results as a formatted table.",
			Handler:     d.query,
		},
		{
			Name:        "list_tables",
			Description: "List all user tables in the database.",
			Handler:     d.listTables,
		},
		{
			Name:        "describe_table",
			Description: "Show the column names and types for a specific table.",
			Handler:     d.describeTable,
		},
	}
}

// ---- input structs ----

// QueryInput is the input for the query tool.
type QueryInput struct {
	SQL  string `json:"sql" desc:"the SELECT SQL query to execute"`
	Args []any  `json:"args" desc:"optional positional query arguments" optional:"true"`
}

// ListTablesInput is the input for the list_tables tool.
type ListTablesInput struct{}

// DescribeTableInput is the input for the describe_table tool.
type DescribeTableInput struct {
	Table string `json:"table" desc:"name of the table to describe"`
}

// ---- helpers ----

// isSelectQuery performs a lightweight prefix check to ensure only SELECT
// statements are permitted. It is not a full SQL parser, so callers should
// also ensure the database user has only read permissions.
func isSelectQuery(q string) bool {
	trimmed := strings.TrimSpace(q)
	// Reject empty queries.
	if trimmed == "" {
		return false
	}
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// formatRows converts sql.Rows into a human-readable table string.
func formatRows(rows *sql.Rows) (string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("fetch columns: %w", err)
	}

	var sb strings.Builder

	// Header.
	sb.WriteString(strings.Join(cols, " | "))
	sb.WriteByte('\n')
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteByte('\n')

	// Rows.
	rowCount := 0
	vals := make([]any, len(cols))
	valPtrs := make([]any, len(cols))
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	for rows.Next() {
		if err := rows.Scan(valPtrs...); err != nil {
			return "", fmt.Errorf("scan row: %w", err)
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			if v == nil {
				parts[i] = "NULL"
			} else {
				parts[i] = fmt.Sprintf("%v", v)
			}
		}
		sb.WriteString(strings.Join(parts, " | "))
		sb.WriteByte('\n')
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate rows: %w", err)
	}

	sb.WriteString(fmt.Sprintf("\n(%d row(s))", rowCount))
	return sb.String(), nil
}

// ---- handlers ----

func (d *DatabaseTools) query(ctx context.Context, in QueryInput) (string, error) {
	if !isSelectQuery(in.SQL) {
		return "", fmt.Errorf("query: only SELECT (or WITH …) statements are permitted")
	}

	rows, err := d.db.QueryContext(ctx, in.SQL, in.Args...)
	if err != nil {
		return "", fmt.Errorf("query: execute: %w", err)
	}
	defer rows.Close()

	result, err := formatRows(rows)
	if err != nil {
		return "", fmt.Errorf("query: format: %w", err)
	}
	return result, nil
}

func (d *DatabaseTools) listTables(ctx context.Context, _ ListTablesInput) (string, error) {
	// This query works for PostgreSQL and SQLite. For MySQL the table name is
	// TABLE_NAME and the schema filter differs; drivers that need a different
	// query should subclass or use a custom tool.
	q := `SELECT table_name FROM information_schema.tables WHERE table_schema NOT IN ('information_schema', 'pg_catalog') AND table_type = 'BASE TABLE' ORDER BY table_name`

	rows, err := d.db.QueryContext(ctx, q)
	if err != nil {
		// Fall back to SQLite pragma if information_schema is unavailable.
		var err2 error
		rows, err2 = d.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
		if err2 != nil {
			return "", fmt.Errorf("list_tables: %w", err2)
		}
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", fmt.Errorf("list_tables: scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("list_tables: iterate: %w", err)
	}

	if len(tables) == 0 {
		return "no tables found", nil
	}
	return strings.Join(tables, "\n"), nil
}

func (d *DatabaseTools) describeTable(ctx context.Context, in DescribeTableInput) (string, error) {
	if in.Table == "" {
		return "", fmt.Errorf("describe_table: table name must not be empty")
	}

	// information_schema.columns works for PostgreSQL and MySQL.
	q := `SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position`

	rows, err := d.db.QueryContext(ctx, q, in.Table)
	if err != nil {
		// SQLite fallback using PRAGMA.
		rows, err = d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", in.Table))
		if err != nil {
			return "", fmt.Errorf("describe_table: %w", err)
		}
	}
	defer rows.Close()

	result, err := formatRows(rows)
	if err != nil {
		return "", fmt.Errorf("describe_table: %w", err)
	}
	return result, nil
}
