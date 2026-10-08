// Command migrate initializes a brand-new development database from the ordered
// authoritative migrations/000000_init-N.sql fragments.
//
// Existing databases are deliberately never rewritten here. Small development
// changes are applied incrementally by an explicit SQL command and mirrored in
// the init fragments; large changes require dump, database recreation, init, and
// player-data restore.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var initPartName = regexp.MustCompile(`^000000_init-(0|[1-9][0-9]*)\.sql$`)

var (
	dsn     = flag.String("dsn", "", "PostgreSQL DSN（必填）")
	initDir = flag.String("dir", "migrations", "开发期初始化 SQL 分片目录")
)

var (
	forbiddenSQL  = regexp.MustCompile(`(?is)\b(?:TRUNCATE\b|DROP\s+(?:TABLE|SCHEMA|DATABASE)\b)`)
	deleteFromSQL = regexp.MustCompile(`(?is)\bDELETE\s+FROM\b`)
	whereSQL      = regexp.MustCompile(`(?is)\bWHERE\b`)
)

func main() {
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if *dsn == "" {
		log.Error("必须提供 -dsn")
		os.Exit(2)
	}
	body, checksum, parts, err := loadInit(*initDir)
	if err != nil {
		log.Error("初始化 SQL 预检失败", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Error("连接数据库失败", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	empty, err := databaseIsEmpty(ctx, pool)
	if err != nil {
		log.Error("检查数据库是否为空失败", "err", err)
		os.Exit(1)
	}
	if !empty {
		log.Error("目标数据库不是空库，拒绝执行 init 分片",
			"hint", "小改请增量执行 SQL 并同步 init；大改请先 dump，再删除并重建数据库")
		os.Exit(1)
	}

	start := time.Now()
	tx, err := pool.Begin(ctx)
	if err == nil {
		_, err = tx.Exec(ctx, string(body))
	}
	if err == nil {
		err = tx.Commit(ctx)
	} else if tx != nil {
		_ = tx.Rollback(ctx)
	}
	if err != nil {
		log.Error("初始化失败并已回滚", "dir", *initDir, "parts", parts, "err", err)
		os.Exit(1)
	}
	log.Info("空库初始化成功", "dir", *initDir, "parts", parts, "sha256", checksum,
		"耗时", time.Since(start).Round(time.Millisecond))
}

type initPart struct {
	index int
	path  string
}

func loadInit(dir string) ([]byte, string, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", 0, err
	}
	parts := make([]initPart, 0)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		match := initPartName.FindStringSubmatch(name)
		if match == nil || !entry.Type().IsRegular() {
			return nil, "", 0, fmt.Errorf("初始化目录只允许 000000_init-N.sql 普通文件，收到 %s", name)
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, "", 0, fmt.Errorf("解析分片序号 %s: %w", name, err)
		}
		parts = append(parts, initPart{index: index, path: filepath.Join(dir, name)})
	}
	if len(parts) == 0 {
		return nil, "", 0, fmt.Errorf("%s 没有 000000_init-N.sql 分片", dir)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].index < parts[j].index })
	var body bytes.Buffer
	for want, part := range parts {
		if part.index != want {
			return nil, "", 0, fmt.Errorf("初始化分片序号必须从 0 连续：期望 %d，实得 %d", want, part.index)
		}
		fragment, err := os.ReadFile(part.path)
		if err != nil {
			return nil, "", 0, err
		}
		if len(bytes.TrimSpace(fragment)) == 0 {
			return nil, "", 0, fmt.Errorf("%s 为空", part.path)
		}
		_, _ = body.Write(fragment)
	}
	joined := body.Bytes()
	if len(bytes.TrimSpace(joined)) == 0 {
		return nil, "", 0, fmt.Errorf("%s 的初始化分片合并后为空", dir)
	}
	if token := forbiddenSQL.Find(joined); token != nil {
		return nil, "", 0, fmt.Errorf("%s 的初始化分片含禁止的破坏性语句 %q", dir, string(token))
	}
	for _, location := range deleteFromSQL.FindAllIndex(joined, -1) {
		statement := joined[location[0]:]
		if end := bytes.IndexByte(statement, ';'); end >= 0 {
			statement = statement[:end]
		}
		if !whereSQL.Match(statement) {
			return nil, "", 0, fmt.Errorf("%s 的初始化分片含没有 WHERE 的 DELETE，拒绝执行", dir)
		}
	}
	sum := sha256.Sum256(joined)
	return joined, hex.EncodeToString(sum[:]), len(parts), nil
}

func databaseIsEmpty(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var empty bool
	err := pool.QueryRow(ctx, `
		SELECT NOT EXISTS (
			SELECT 1
			  FROM pg_class c
			  JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
			   AND n.nspname NOT LIKE 'pg_toast%'
			   AND c.relkind IN ('r', 'p', 'S', 'v', 'm')
		)`).Scan(&empty)
	return empty, err
}
