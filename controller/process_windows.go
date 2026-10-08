//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func configureProcess(_ *exec.Cmd) {}

func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func processAlreadyGone(err error) bool {
	return errors.Is(err, os.ErrProcessDone)
}

func (c *controller) listener(port int) (*listenerProcess, error) {
	connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
	if dialErr != nil {
		return nil, nil
	}
	_ = connection.Close()

	netstat, err := c.toolPath("netstat")
	if err != nil {
		return &listenerProcess{Command: "unknown"}, nil
	}
	ctx, cancel := commandContext(3 * time.Second)
	defer cancel()
	output, err := c.run(ctx, "", netstat, "-ano", "-p", "tcp")
	if err != nil {
		return nil, fmt.Errorf("检查端口 %d 失败: %w", port, err)
	}
	wanted := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") || !strings.EqualFold(fields[len(fields)-2], "LISTENING") {
			continue
		}
		if !strings.HasSuffix(fields[1], wanted) {
			continue
		}
		pid, parseErr := strconv.Atoi(fields[len(fields)-1])
		if parseErr != nil {
			continue
		}
		command := "unknown"
		if tasklist, findErr := c.toolPath("tasklist"); findErr == nil {
			ctx, cancel := commandContext(3 * time.Second)
			text, taskErr := c.run(ctx, "", tasklist, "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
			cancel()
			if taskErr == nil {
				command = text
			}
		}
		return &listenerProcess{PID: pid, Command: command}, nil
	}
	return &listenerProcess{Command: "unknown"}, nil
}
