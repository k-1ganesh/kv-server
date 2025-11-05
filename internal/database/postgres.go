package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

func NewPostgresDB(host, port, user, password, dbname string) (*PostgresDB, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(10)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) Create(key, value string) error {
	query := `INSERT INTO kv_store (key, value) VALUES ($1, $2)
			  ON CONFLICT (key) DO UPDATE SET value = $2`
	_, err := p.db.Exec(query, key, value)
	return err
}

func (p *PostgresDB) Read(key string) (string, error) {
	var value string
	query := `SELECT value FROM kv_store WHERE key = $1`
	err := p.db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("key not found")
	}
	return value, err
}

func (p *PostgresDB) Delete(key string) error {
	query := `DELETE FROM kv_store WHERE key = $1`
	result, err := p.db.Exec(query, key)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("key not found")
	}
	return nil
}

func (p *PostgresDB) Close() error {
	return p.db.Close()
}
