//go:build !windows

package main

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func processAlreadyGone(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}

func (c *controller) listener(port int) (*listenerProcess, error) {
	lsof, err := c.toolPath("lsof")
	if err != nil {
		connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
		if dialErr != nil {
			return nil, nil
		}
		_ = connection.Close()
		return &listenerProcess{Command: "unknown"}, nil
	}
	ctx, cancel := commandContext(2 * time.Second)
	defer cancel()
	output, runErr := c.run(ctx, "", lsof, "-nP", fmt.Sprintf("-tiTCP:%d", port), "-sTCP:LISTEN")
	if runErr != nil {
		if strings.TrimSpace(output) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("检查端口 %d 失败: %w", port, runErr)
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return nil, nil
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil, fmt.Errorf("解析端口 %d 的进程号失败", port)
	}
	ps, err := c.toolPath("ps")
	if err != nil {
		return nil, err
	}
	ctx, cancel = commandContext(2 * time.Second)
	defer cancel()
	command, err := c.run(ctx, "", ps, "-p", strconv.Itoa(pid), "-o", "command=")
	if err != nil {
		return nil, fmt.Errorf("读取 PID %d 失败: %w", pid, err)
	}
	return &listenerProcess{PID: pid, Command: strings.TrimSpace(command)}, nil
}
