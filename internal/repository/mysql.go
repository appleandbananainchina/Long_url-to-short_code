package repository

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"short-url-service/pkg/breaker"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

var ErrDuplicateKey = errors.New("duplicate key")

type MySQLRepo struct {
	db *sql.DB
}

func NewMySQLRepo(dsn string) (*MySQLRepo, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	// 连接池配置
	db.SetMaxOpenConns(150)
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := db.PingContext(ctx); err != nil {
				slog.ErrorContext(context.Background(), "MySQL health check failed", "error", err)
			}
			cancel()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &MySQLRepo{db: db}, nil
}

func (r *MySQLRepo) Save(ctx context.Context, shortCode, longURL string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO short_url.url_mapping (short_code, long_url) VALUES (?, ?)",
		shortCode, longURL)
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicateKey
		}
		return err
	}
	return nil
}

// SaveIdempotent 幂等保存：如果 shortCode 已存在且 longURL 一致，返回 nil；否则返回 ErrDuplicateKey
func (r *MySQLRepo) SaveIdempotent(ctx context.Context, shortCode, longURL string) error {
	// 先尝试插入
	err := r.Save(ctx, shortCode, longURL)
	if err == nil {
		return nil
	}
	// 不是唯一键冲突，直接返回
	if !errors.Is(err, ErrDuplicateKey) {
		return err
	}
	// 冲突：查询已存在的 longURL
	existing, getErr := r.Get(ctx, shortCode)
	if getErr != nil {
		// 查询失败，无法判断，返回原始冲突错误
		return err
	}
	if existing == longURL {
		// 已存在且内容相同，视为成功（幂等）
		return nil
	}
	// 内容不同，真冲突
	return ErrDuplicateKey
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

// GetAllShortKeysCursor 基于主键游标分页获取短码
// 参数：batchSize 每批数量，lastID 上一批最后一条记录的 ID（首次传 0）
// 返回：短码切片，最后一条记录的 ID，错误
func (r *MySQLRepo) GetAllShortKeysCursor(batchSize int, lastID int64) ([]string, int64, error) {
	rows, err := r.db.Query(`
        SELECT short_code, id 
        FROM short_url.url_mapping 
        WHERE id > ? 
        ORDER BY id 
        LIMIT ?`,
		lastID, batchSize,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	keys := make([]string, 0, batchSize)
	var newLastID int64
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, 0, err
		}
		keys = append(keys, key)
		newLastID = id // 最后一条记录的 ID 作为下一轮的游标
	}
	return keys, newLastID, rows.Err()
}

func (r *MySQLRepo) Close() error {
	return r.db.Close()
}

func isDuplicateKeyError(err error) bool {
	if strings.Contains(err.Error(), "Duplicate entry") {
		return true
	}
	if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok && mysqlErr.Number == 1062 {
		return true
	}
	return false
}

func (r *MySQLRepo) GetWithBreaker(ctx context.Context, shortCode string) (string, error) {
	return breaker.MySQLBreaker.Do(ctx, func() (string, error) {
		return r.Get(ctx, shortCode)
	})
}

// MySQLTx
type MySQLTx struct {
	tx *sql.Tx
}

func (r *MySQLRepo) BeginTx(ctx context.Context) (*MySQLTx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &MySQLTx{tx: tx}, nil
}

func (tx *MySQLTx) SaveIdempotent(ctx context.Context, shortCode, longURL string) error {
	_, err := tx.tx.ExecContext(ctx,
		"INSERT INTO short_url.url_mapping (short_code, long_url) VALUES (?, ?)",
		shortCode, longURL)
	if err != nil {
		if isDuplicateKeyError(err) {
			// 冲突：检查是否内容相同
			var existing string
			checkErr := tx.tx.QueryRowContext(ctx,
				"SELECT long_url FROM short_url.url_mapping WHERE short_code = ? FOR UPDATE",
				shortCode).Scan(&existing)
			if checkErr != nil {
				return ErrDuplicateKey
			}
			if existing == longURL {
				return nil // 幂等
			}
			return ErrDuplicateKey
		}
		return err
	}
	return nil
}

func (tx *MySQLTx) Commit() error {
	return tx.tx.Commit()
}

func (tx *MySQLTx) Rollback() error {
	return tx.tx.Rollback()
}
