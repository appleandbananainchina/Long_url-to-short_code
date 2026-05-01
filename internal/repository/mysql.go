package repository

import (
	"context"
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLRepo struct {
	db *sql.DB
}

func NewMySQLRepo(dsn string) (*MySQLRepo, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// 连接池配置
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(10)
	return &MySQLRepo{db: db}, nil
}

func (r *MySQLRepo) Save(ctx context.Context, shortCode, longURL string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO short_url.url_mapping (short_code, long_url) VALUES (?, ?)",
		shortCode, longURL)
	return err
}

func (r *MySQLRepo) Get(ctx context.Context, shortCode string) (string, error) {
	var longURL string
	err := r.db.QueryRowContext(ctx,
		"SELECT long_url FROM short_url.url_mapping WHERE short_code = ?", shortCode).Scan(&longURL)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return longURL, err
}

func (r *MySQLRepo) GetAllShortKeys(batchSize, offset int) ([]string, error) {
	rows, err := r.db.Query("SELECT short_code FROM short_url.url_mapping LIMIT ? OFFSET ?", batchSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]string, 0, batchSize)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (r *MySQLRepo) GetTotalCount() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM short_url.url_mapping").Scan(&count)
	return count, err
}

func (r *MySQLRepo) Close() error {
	return r.db.Close()
}
