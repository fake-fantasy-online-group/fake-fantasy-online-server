package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dockerInspect struct {
	Config struct {
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

type databaseRuntime struct {
	Exists   bool
	Running  bool
	Ready    bool
	State    string
	User     string
	Password string
	Database string
	Port     string
}

func (d databaseRuntime) DSN() (string, error) {
	if !d.Exists {
		return "", fmt.Errorf("数据库容器不存在")
	}
	if d.Port == "" {
		return "", fmt.Errorf("数据库容器没有发布 PostgreSQL 端口")
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(d.User, d.Password), Host: net.JoinHostPort("127.0.0.1", d.Port), Path: "/" + d.Database}
	query := u.Query()
	query.Set("sslmode", "disable")
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func (c *controller) databaseStatus() serviceStatus {
	status := serviceStatus{ID: serviceDatabase, Name: displayName(serviceDatabase), Phase: "stopped", Endpoint: "127.0.0.1:5432"}
	db, err := c.inspectDatabase()
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			status.Detail = "尚未创建 · 首次启动会自动创建"
			return status
		}
		status.Phase = "error"
		status.Detail = err.Error()
		return status
	}
	if db.Port != "" {
		status.Endpoint = "127.0.0.1:" + db.Port
	}
	if !db.Running {
		status.Detail = "容器已停止"
		return status
	}
	if !db.Ready {
		status.Phase = "starting"
		status.Detail = "容器运行中，等待 PostgreSQL 就绪"
		return status
	}
	status.Phase = "running"
	status.Detail = "PostgreSQL 已就绪 · 容器 fantasy-postgres"
	return status
}

func (c *controller) inspectDatabase() (databaseRuntime, error) {
	docker, err := c.toolPath("docker")
	if err != nil {
		return databaseRuntime{}, err
	}
	ctx, cancel := commandContext(4 * time.Second)
	defer cancel()
	output, err := c.run(ctx, "", docker, "inspect", "fantasy-postgres")
	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "no such object") || strings.Contains(lower, "no such container") {
			return databaseRuntime{}, fmt.Errorf("数据库容器不存在")
		}
		if output == "" {
			output = err.Error()
		}
		return databaseRuntime{}, fmt.Errorf("Docker 不可用: %s", firstLine(output))
	}
	var decoded []dockerInspect
	if err := json.Unmarshal([]byte(output), &decoded); err != nil || len(decoded) != 1 {
		return databaseRuntime{}, fmt.Errorf("无法解析 fantasy-postgres 容器信息")
	}
	item := decoded[0]
	if item.Config.Labels["com.docker.compose.project"] != "fake-fantasy-server" || item.Config.Labels["com.docker.compose.service"] != "postgres" {
		return databaseRuntime{}, fmt.Errorf("容器 fantasy-postgres 已存在，但不属于预期的 Compose 项目 fake-fantasy-server/postgres；为避免操作无关容器已停止")
	}
	db := databaseRuntime{Exists: true, Running: item.State.Running, State: item.State.Status, User: "fantasy", Database: "fantasy"}
	for _, value := range item.Config.Env {
		key, raw, found := strings.Cut(value, "=")
		if !found {
			continue
		}
		switch key {
		case "POSTGRES_USER":
			db.User = raw
		case "POSTGRES_PASSWORD":
			db.Password = raw
		case "POSTGRES_DB":
			db.Database = raw
		}
	}
	if bindings := item.NetworkSettings.Ports["5432/tcp"]; len(bindings) > 0 {
		db.Port = bindings[0].HostPort
	}
	if db.Running {
		ctx, cancel := commandContext(3 * time.Second)
		defer cancel()
		_, readyErr := c.run(ctx, "", docker, "exec", "fantasy-postgres", "pg_isready", "-U", db.User, "-d", db.Database)
		db.Ready = readyErr == nil
	}
	return db, nil
}

func (c *controller) startDatabase() error {
	docker, err := c.toolPath("docker")
	if err != nil {
		return err
	}
	if db, inspectErr := c.inspectDatabase(); inspectErr != nil && !strings.Contains(inspectErr.Error(), "不存在") {
		return inspectErr
	} else if inspectErr == nil && db.Running && db.Ready {
		c.logf("database", "info", "数据库已在运行")
		return c.initializeDatabaseIfEmpty(db)
	}
	c.logf("database", "info", "正在通过项目 Compose 配置启动 PostgreSQL")
	ctx, cancel := commandContext(3 * time.Minute)
	output, runErr := c.run(ctx, c.serverDir, docker, c.composeArgs("up", "-d", "postgres")...)
	cancel()
	if runErr != nil {
		c.logCommandFailure("database", "启动数据库失败", output, runErr)
		return fmt.Errorf("启动数据库失败: %w", runErr)
	}

	var db databaseRuntime
	var inspectErr error
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		db, inspectErr = c.inspectDatabase()
		if inspectErr == nil && db.Ready {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if inspectErr != nil || !db.Ready {
		return fmt.Errorf("等待 PostgreSQL 就绪超时")
	}
	c.logf("database", "success", "PostgreSQL 已就绪 · 127.0.0.1:%s", db.Port)
	return c.initializeDatabaseIfEmpty(db)
}

func (c *controller) initializeDatabaseIfEmpty(db databaseRuntime) error {
	docker, err := c.toolPath("docker")
	if err != nil {
		return err
	}
	const emptyQuery = `SELECT NOT EXISTS (
SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
AND c.relkind IN ('r','p','S','v','m'));`
	ctx, cancel := commandContext(15 * time.Second)
	output, queryErr := c.run(ctx, "", docker, "exec", "fantasy-postgres", "psql", "-U", db.User, "-d", db.Database, "-tAc", emptyQuery)
	cancel()
	if queryErr != nil {
		return fmt.Errorf("检查数据库初始化状态失败: %w", queryErr)
	}
	if strings.TrimSpace(output) != "t" {
		return nil
	}
	c.logf("database", "info", "检测到空库，正在执行 init 分片（首次启动会稍久）")
	dsn, err := db.DSN()
	if err != nil {
		return err
	}
	goPath, err := c.toolPath("go")
	if err != nil {
		return err
	}
	temporary := filepath.Join(c.runtimeDir, fmt.Sprintf("migrate.tmp-%d", os.Getpid()))
	target := filepath.Join(c.runtimeDir, executableName("migrate"))
	ctx, cancel = commandContext(8 * time.Minute)
	buildOutput, buildErr := c.run(ctx, c.serverDir, goPath, "build", "-o", temporary, "./cmd/migrate")
	cancel()
	if buildErr != nil {
		_ = os.Remove(temporary)
		c.logCommandFailure("database", "编译初始化器失败", buildOutput, buildErr)
		return fmt.Errorf("编译数据库初始化器失败: %w", buildErr)
	}
	if err := replaceFile(temporary, target); err != nil {
		return err
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Minute)
	migrateOutput, migrateErr := c.run(ctx, c.serverDir, target, "-dsn", dsn, "-dir", "migrations")
	cancel()
	if migrateErr != nil {
		c.logCommandFailure("database", "数据库初始化失败", migrateOutput, migrateErr)
		return fmt.Errorf("数据库初始化失败: %w", migrateErr)
	}
	c.logf("database", "success", "空库初始化完成")
	return nil
}

func (c *controller) stopDatabase() error {
	db, err := c.inspectDatabase()
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			c.logf("database", "info", "数据库尚未创建")
			return nil
		}
		return err
	}
	if !db.Running {
		c.logf("database", "info", "数据库已经停止")
		return nil
	}
	docker, err := c.toolPath("docker")
	if err != nil {
		return err
	}
	c.logf("database", "info", "正在停止 PostgreSQL 容器（数据卷会保留）")
	ctx, cancel := commandContext(45 * time.Second)
	output, stopErr := c.run(ctx, c.serverDir, docker, c.composeArgs("stop", "postgres")...)
	cancel()
	if stopErr != nil {
		c.logCommandFailure("database", "停止数据库失败", output, stopErr)
		return fmt.Errorf("停止数据库失败: %w", stopErr)
	}
	c.logf("database", "success", "数据库已停止 · 数据卷已保留")
	return nil
}

func (c *controller) composeArgs(arguments ...string) []string {
	composeFile := filepath.Join(c.serverDir, "docker-compose.yml")
	result := []string{"compose", "--project-directory", c.serverDir, "-f", composeFile, "--project-name", "fake-fantasy-server"}
	envFile := filepath.Join(c.serverDir, ".env")
	if info, err := os.Stat(envFile); err == nil && !info.IsDir() {
		result = append(result, "--env-file", envFile)
	}
	return append(result, arguments...)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if before, _, found := strings.Cut(value, "\n"); found {
		return before
	}
	return value
}
