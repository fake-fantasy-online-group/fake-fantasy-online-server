package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type serviceID string

const (
	serviceDatabase serviceID = "database"
	serviceDispatch serviceID = "dispatch"
	serviceGame     serviceID = "gameserver"
)

type logEntry struct {
	Time    string `json:"time"`
	Service string `json:"service"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type serviceStatus struct {
	ID       serviceID `json:"id"`
	Name     string    `json:"name"`
	Phase    string    `json:"phase"`
	Detail   string    `json:"detail"`
	Endpoint string    `json:"endpoint"`
	PID      int       `json:"pid,omitempty"`
}

type statusResponse struct {
	Services   []serviceStatus
	AllReady   bool
	Busy       string
	Repository string
	Logs       []logEntry
	UpdatedAt  time.Time
}

type managedProcess struct {
	cmd *exec.Cmd
	pid int
}

type controller struct {
	root       string
	serverDir  string
	runtimeDir string
	env        []string
	httpClient *http.Client

	operationMu sync.Mutex
	stateMu     sync.RWMutex
	busy        string
	logs        []logEntry
	processes   map[serviceID]*managedProcess
	stopping    map[serviceID]bool
}

func newController(root string) *controller {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	runtimeDir := filepath.Join(configDir, "FantasyServerController", "bin")
	_ = os.MkdirAll(runtimeDir, 0o755)
	pathParts := []string{os.Getenv("PATH")}
	if runtime.GOOS == "darwin" {
		pathParts = append([]string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, pathParts...)
	}
	environment := withEnvironmentValue(os.Environ(), "PATH", strings.Join(pathParts, string(os.PathListSeparator)))
	return &controller{
		root: root, serverDir: filepath.Join(root, "server"), runtimeDir: runtimeDir,
		env:        environment,
		httpClient: &http.Client{Timeout: 1200 * time.Millisecond},
		processes:  make(map[serviceID]*managedProcess),
		stopping:   make(map[serviceID]bool),
	}
}

func (c *controller) snapshot() statusResponse {
	database := c.databaseStatus()
	dispatch := c.networkServiceStatus(serviceDispatch)
	game := c.networkServiceStatus(serviceGame)
	services := []serviceStatus{database, dispatch, game}
	ready := true
	for _, service := range services {
		ready = ready && service.Phase == "running"
	}
	c.stateMu.RLock()
	busy := c.busy
	logs := append([]logEntry(nil), c.logs...)
	c.stateMu.RUnlock()
	return statusResponse{
		Services: services, AllReady: ready, Busy: busy, Repository: c.root,
		Logs: logs, UpdatedAt: time.Now(),
	}
}

func (c *controller) prepareDatabaseWeb() error {
	if err := c.startDatabase(); err != nil {
		return err
	}
	if err := c.ensureCatalog(); err != nil {
		return err
	}
	return nil
}

func validService(service serviceID) bool {
	return service == serviceDatabase || service == serviceDispatch || service == serviceGame
}

func (c *controller) setBusy(value string) {
	c.stateMu.Lock()
	c.busy = value
	c.stateMu.Unlock()
}

func (c *controller) logf(service, level, format string, args ...any) {
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		return
	}
	c.stateMu.Lock()
	c.logs = append(c.logs, logEntry{Time: time.Now().Format("15:04:05"), Service: service, Level: level, Message: message})
	if len(c.logs) > 240 {
		c.logs = append([]logEntry(nil), c.logs[len(c.logs)-240:]...)
	}
	c.stateMu.Unlock()
}

func (c *controller) startAll() error {
	c.logf("controller", "info", "开始启动全部服务")
	if err := c.startDatabase(); err != nil {
		return err
	}
	if err := c.ensureCatalog(); err != nil {
		return err
	}
	if err := c.startGame(); err != nil {
		return err
	}
	c.logf("controller", "success", "全部服务已就绪")
	return nil
}

func (c *controller) stopAll() error {
	c.logf("controller", "info", "开始安全关闭全部服务")
	if err := c.stopNetworkService(serviceGame); err != nil {
		return err
	}
	if err := c.stopNetworkService(serviceDispatch); err != nil {
		return err
	}
	if err := c.stopDatabase(); err != nil {
		return err
	}
	c.logf("controller", "success", "全部服务已关闭")
	return nil
}

func (c *controller) startService(service serviceID) error {
	switch service {
	case serviceDatabase:
		return c.startDatabase()
	case serviceDispatch:
		return c.startDispatch()
	case serviceGame:
		return c.startGame()
	default:
		return fmt.Errorf("未知服务 %s", service)
	}
}

func (c *controller) stopService(service serviceID) error {
	if service == serviceDatabase {
		return c.stopDatabase()
	}
	return c.stopNetworkService(service)
}

func (c *controller) startGame() error {
	db, err := c.inspectDatabase()
	if err != nil {
		return err
	}
	if !db.Running || !db.Ready {
		return fmt.Errorf("数据库尚未就绪，请先启动数据库")
	}
	dsn, err := db.DSN()
	if err != nil {
		return err
	}
	return c.startNetworkService(serviceGame, []string{"-addr", ":19000", "-debug", "-dsn", dsn})
}

func (c *controller) startDispatch() error {
	db, err := c.inspectDatabase()
	if err != nil {
		return err
	}
	if !db.Running || !db.Ready {
		return fmt.Errorf("数据库尚未就绪，请先启动数据库")
	}
	dsn, err := db.DSN()
	if err != nil {
		return err
	}
	return c.startNetworkService(serviceDispatch, []string{"-addr", ":8088", "-game-host", "127.0.0.1", "-game-port", "19000", "-dsn", dsn})
}

func (c *controller) ensureCatalog() error {
	status := c.networkServiceStatus(serviceDispatch)
	if status.Phase == "running" && c.urlHealthy("http://127.0.0.1:8088/api/catalog/meta") {
		return nil
	}
	if status.Phase == "running" || status.Phase == "error" {
		c.logf("dispatch", "info", "地址发现需要重新连接数据库，正在重启")
		if err := c.stopNetworkService(serviceDispatch); err != nil {
			return err
		}
	}
	if err := c.startDispatch(); err != nil {
		return err
	}
	if !c.urlHealthy("http://127.0.0.1:8088/api/catalog/meta") {
		return fmt.Errorf("地址发现已启动，但幻想数据库接口仍不可用")
	}
	return nil
}

func (c *controller) startNetworkService(service serviceID, arguments []string) error {
	status := c.networkServiceStatus(service)
	if status.Phase == "running" {
		c.logf(string(service), "info", "%s 已在运行", displayName(service))
		return nil
	}
	if status.Phase == "conflict" {
		return fmt.Errorf("%s", status.Detail)
	}

	binary, err := c.buildBinary(service)
	if err != nil {
		return err
	}
	cmd := exec.Command(binary, arguments...)
	cmd.Dir = c.serverDir
	cmd.Env = c.env
	configureProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动%s失败: %w", displayName(service), err)
	}
	managed := &managedProcess{cmd: cmd, pid: cmd.Process.Pid}
	exited := make(chan error, 1)
	c.stateMu.Lock()
	c.processes[service] = managed
	delete(c.stopping, service)
	c.stateMu.Unlock()
	go c.captureOutput(service, stdout)
	go c.captureOutput(service, stderr)
	go func() {
		err := cmd.Wait()
		c.stateMu.Lock()
		wasStopping := c.stopping[service]
		if c.processes[service] == managed {
			delete(c.processes, service)
		}
		delete(c.stopping, service)
		c.stateMu.Unlock()
		exited <- err
		if err != nil && !wasStopping {
			c.logf(string(service), "error", "%s进程已退出: %v", displayName(service), err)
		} else {
			c.logf(string(service), "info", "%s进程已退出", displayName(service))
		}
	}()
	c.logf(string(service), "info", "正在启动%s · PID %d", displayName(service), cmd.Process.Pid)

	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		status = c.networkServiceStatus(service)
		if status.Phase == "running" {
			if service == serviceDispatch && !c.urlHealthy("http://127.0.0.1:8088/dispatch.json") {
				time.Sleep(250 * time.Millisecond)
				continue
			}
			c.logf(string(service), "success", "%s已就绪 · %s", displayName(service), status.Endpoint)
			return nil
		}
		select {
		case <-exited:
			return fmt.Errorf("%s启动后立即退出，请查看运行日志", displayName(service))
		default:
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("等待%s就绪超时", displayName(service))
}

func (c *controller) buildBinary(service serviceID) (string, error) {
	goPath, err := c.toolPath("go")
	if err != nil {
		return "", err
	}
	target := filepath.Join(c.runtimeDir, executableName(string(service)))
	temporary := fmt.Sprintf("%s.tmp-%d", target, os.Getpid())
	_ = os.Remove(temporary)
	c.logf(string(service), "info", "正在编译最新%s…", displayName(service))
	ctx, cancel := commandContext(8 * time.Minute)
	defer cancel()
	output, err := c.run(ctx, c.serverDir, goPath, "build", "-o", temporary, "./cmd/"+string(service))
	if err != nil {
		_ = os.Remove(temporary)
		c.logCommandFailure(string(service), "编译失败", output, err)
		return "", fmt.Errorf("编译%s失败: %w", displayName(service), err)
	}
	if err := replaceFile(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("安装%s运行文件失败: %w", displayName(service), err)
	}
	return target, nil
}

func (c *controller) stopNetworkService(service serviceID) error {
	listener, err := c.listener(servicePort(service))
	if err != nil {
		return err
	}
	if listener == nil {
		c.logf(string(service), "info", "%s已经停止", displayName(service))
		return nil
	}
	if !processMatchesService(listener.Command, service) {
		return fmt.Errorf("端口 %d 被其他进程占用，已拒绝终止 · PID %d", servicePort(service), listener.PID)
	}
	c.logf(string(service), "info", "正在安全关闭%s · PID %d", displayName(service), listener.PID)
	c.stateMu.Lock()
	if managed := c.processes[service]; managed != nil && managed.pid == listener.PID {
		c.stopping[service] = true
	}
	c.stateMu.Unlock()
	if err := terminateProcess(listener.PID); err != nil && !processAlreadyGone(err) {
		return fmt.Errorf("关闭%s失败: %w", displayName(service), err)
	}
	wait := 8 * time.Second
	if service == serviceGame {
		wait = 38 * time.Second
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		remaining, _ := c.listener(servicePort(service))
		if remaining == nil {
			c.logf(string(service), "success", "%s已关闭", displayName(service))
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("%s未能在安全等待时间内退出；没有强制杀进程", displayName(service))
}

func (c *controller) captureOutput(service serviceID, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := redactDSN(scanner.Text())
		level := "info"
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "失败") || strings.Contains(lower, "fatal") {
			level = "error"
		}
		c.logf(string(service), level, "%s", line)
	}
}

func redactDSN(value string) string {
	start := strings.Index(value, "postgres://")
	if start < 0 {
		return value
	}
	rest := value[start:]
	end := strings.IndexAny(rest, " \t\"'")
	if end < 0 {
		end = len(rest)
	}
	raw := rest[:end]
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return value
	}
	parsed.User = url.User(parsed.User.Username())
	return value[:start] + parsed.String() + rest[end:]
}

func (c *controller) networkServiceStatus(service serviceID) serviceStatus {
	status := serviceStatus{ID: service, Name: displayName(service), Phase: "stopped", Endpoint: serviceEndpoint(service)}
	listener, err := c.listener(servicePort(service))
	if err != nil {
		status.Phase = "error"
		status.Detail = err.Error()
		return status
	}
	if listener == nil {
		status.Detail = "未运行"
		return status
	}
	status.PID = listener.PID
	if !processMatchesService(listener.Command, service) {
		status.Phase = "conflict"
		status.Detail = fmt.Sprintf("端口 %d 被其他进程占用 · PID %d", servicePort(service), listener.PID)
		return status
	}
	status.Phase = "running"
	status.Detail = fmt.Sprintf("运行中 · PID %d", listener.PID)
	return status
}

type listenerProcess struct {
	PID     int
	Command string
}

func processMatchesService(command string, service serviceID) bool {
	command = strings.ToLower(command)
	name := string(service)
	return strings.Contains(command, "/"+name) || strings.Contains(command, "\\"+name) ||
		strings.Contains(command, "cmd/"+name) || strings.Contains(command, name+".exe")
}

func (c *controller) urlHealthy(rawURL string) bool {
	response, err := c.httpClient.Get(rawURL)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func servicePort(service serviceID) int {
	if service == serviceDispatch {
		return 8088
	}
	return 19000
}

func serviceEndpoint(service serviceID) string {
	if service == serviceDispatch {
		return "http://127.0.0.1:8088"
	}
	return "127.0.0.1:19000"
}

func displayName(service serviceID) string {
	switch service {
	case serviceDatabase:
		return "数据库"
	case serviceDispatch:
		return "地址发现"
	case serviceGame:
		return "游戏服务器"
	default:
		return string(service)
	}
}

func (c *controller) toolPath(name string) (string, error) {
	pathEnvironment := ""
	for _, value := range c.env {
		if key, raw, found := strings.Cut(value, "="); found && strings.EqualFold(key, "PATH") {
			pathEnvironment = raw
			break
		}
	}
	for _, directory := range filepath.SplitList(pathEnvironment) {
		candidate := filepath.Join(directory, executableName(name))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("找不到命令 %s，请先安装并确认可用", name)
}

func (c *controller) run(ctx context.Context, directory, executable string, arguments ...string) (string, error) {
	cmd := exec.CommandContext(ctx, executable, arguments...)
	cmd.Dir = directory
	cmd.Env = c.env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func (c *controller) logCommandFailure(service, prefix, output string, err error) {
	output = strings.TrimSpace(redactDSN(output))
	if output != "" {
		lines := strings.Split(output, "\n")
		if len(lines) > 8 {
			lines = lines[len(lines)-8:]
		}
		c.logf(service, "error", "%s: %s", prefix, strings.Join(lines, " · "))
		return
	}
	c.logf(service, "error", "%s: %v", prefix, err)
}

func withEnvironmentValue(environment []string, key, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, current := range environment {
		currentKey, _, found := strings.Cut(current, "=")
		if found && strings.EqualFold(currentKey, key) {
			continue
		}
		result = append(result, current)
	}
	return append(result, key+"="+value)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

func replaceFile(source, target string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(source, target)
}
