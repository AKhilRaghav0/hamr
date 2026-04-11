//go:build ignore

// postgres-mcp is a practical example: an MCP server that lets Claude
// query your PostgreSQL database safely. Read-only by default.
//
// Usage:
//
//	export DATABASE_URL="postgres://user:pass@localhost:5432/mydb?sslmode=disable"
//	go run .
//
// Then add to Claude Desktop:
//
//	{
//	  "mcpServers": {
//	    "my-database": {
//	      "command": "/path/to/postgres-mcp",
//	      "env": {
//	        "DATABASE_URL": "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
//	      }
//	    }
//	  }
//	}
//
// Now Claude can ask your database questions directly.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// --- Input types (struct tags = JSON schema) ---

type QueryInput struct {
	SQL string `json:"sql" desc:"SQL SELECT query to run (read-only, no writes allowed)"`
}

type ListTablesInput struct{}

type DescribeTableInput struct {
	Table string `json:"table" desc:"name of the table to describe"`
}

type SearchDataInput struct {
	Table  string `json:"table" desc:"table to search in"`
	Column string `json:"column" desc:"column to search"`
	Value  string `json:"value" desc:"value to search for (uses ILIKE for partial match)"`
	Limit  int    `json:"limit" desc:"max rows to return" default:"20"`
}

type TableStatsInput struct {
	Table string `json:"table" desc:"table to get stats for"`
}

// --- Global DB handle ---

var db *sql.DB

// --- Handlers ---

func query(_ context.Context, in QueryInput) (string, error) {
	// Safety: only allow SELECT/WITH/EXPLAIN
	upper := strings.TrimSpace(strings.ToUpper(in.SQL))
	if !strings.HasPrefix(upper, "SELECT") &&
		!strings.HasPrefix(upper, "WITH") &&
		!strings.HasPrefix(upper, "EXPLAIN") {
		return "", fmt.Errorf("only SELECT, WITH, and EXPLAIN queries are allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, in.SQL)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return formatRows(rows, 100)
}

func listTables(_ context.Context, _ ListTablesInput) (string, error) {
	rows, err := db.Query(`
		SELECT table_name, pg_size_pretty(pg_total_relation_size(quote_ident(table_name))) as size
		FROM information_schema.tables
		WHERE table_schema = 'public'
		ORDER BY table_name
	`)
	if err != nil {
		return "", fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Tables in public schema:\n\n")
	for rows.Next() {
		var name, size string
		if err := rows.Scan(&name, &size); err != nil {
			continue
		}
		fmt.Fprintf(&sb, "  %-30s %s\n", name, size)
	}
	return sb.String(), nil
}

func describeTable(_ context.Context, in DescribeTableInput) (string, error) {
	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = $1 AND table_schema = 'public'
		ORDER BY ordinal_position
	`, in.Table)
	if err != nil {
		return "", fmt.Errorf("describe table: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "Table: %s\n\n", in.Table)
	fmt.Fprintf(&sb, "  %-25s %-20s %-10s %s\n", "COLUMN", "TYPE", "NULLABLE", "DEFAULT")
	fmt.Fprintf(&sb, "  %s\n", strings.Repeat("-", 75))

	for rows.Next() {
		var name, dtype, nullable string
		var def sql.NullString
		if err := rows.Scan(&name, &dtype, &nullable, &def); err != nil {
			continue
		}
		defStr := ""
		if def.Valid {
			defStr = def.String
		}
		fmt.Fprintf(&sb, "  %-25s %-20s %-10s %s\n", name, dtype, nullable, defStr)
	}
	return sb.String(), nil
}

func searchData(_ context.Context, in SearchDataInput) (string, error) {
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	q := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s::text ILIKE $1 LIMIT $2",
		quoteIdent(in.Table), quoteIdent(in.Column),
	)

	rows, err := db.Query(q, "%"+in.Value+"%", limit)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	return formatRows(rows, limit)
}

func tableStats(_ context.Context, in TableStatsInput) (string, error) {
	var count int64
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(in.Table))).Scan(&count)
	if err != nil {
		return "", fmt.Errorf("count: %w", err)
	}

	var size string
	err = db.QueryRow("SELECT pg_size_pretty(pg_total_relation_size($1))", in.Table).Scan(&size)
	if err != nil {
		size = "unknown"
	}

	return fmt.Sprintf("Table: %s\nRows: %d\nSize: %s", in.Table, count, size), nil
}

// --- Helpers ---

func formatRows(rows *sql.Rows, maxRows int) (string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	// Header
	for i, col := range cols {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(col)
	}
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", 80))
	sb.WriteString("\n")

	// Rows
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}

	rowCount := 0
	for rows.Next() && rowCount < maxRows {
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		for i, v := range values {
			if i > 0 {
				sb.WriteString(" | ")
			}
			if v == nil {
				sb.WriteString("NULL")
			} else {
				sb.WriteString(fmt.Sprintf("%v", v))
			}
		}
		sb.WriteString("\n")
		rowCount++
	}

	fmt.Fprintf(&sb, "\n(%d rows)", rowCount)
	return sb.String(), nil
}

func quoteIdent(s string) string {
	// Basic SQL identifier quoting to prevent injection
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required\n\nExample: export DATABASE_URL=\"postgres://user:pass@localhost:5432/mydb?sslmode=disable\"")
	}

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	s := hamr.New("postgres-mcp", "1.0.0",
		hamr.WithDescription("Query your PostgreSQL database with Claude"),
	)

	s.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Timeout(15*time.Second),
	)

	s.Tool("query", "Run a read-only SQL query (SELECT/WITH/EXPLAIN only)", query)
	s.Tool("list_tables", "List all tables in the public schema with sizes", listTables)
	s.Tool("describe_table", "Show columns, types, and defaults for a table", describeTable)
	s.Tool("search_data", "Search a table column for a value (partial match)", searchData)
	s.Tool("table_stats", "Get row count and size for a table", tableStats)

	// This is all it takes. 5 tools, ~250 lines total, production-ready MCP server.
	log.Fatal(s.Run())
}
