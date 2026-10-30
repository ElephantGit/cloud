//go:build linux

package node

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// cleanOwned signals only executables copied into this private root. pidfds prevent PID-reuse
// signals; remaining unclassified processes in the root block cleanup instead of being killed.
func cleanOwned(ctx context.Context, paths Paths, grace time.Duration) error {
	for _, name := range []string{"nb", "hb", "gb"} {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			proc := filepath.Join("/proc", entry.Name())
			exe, err := os.Readlink(filepath.Join(proc, "exe"))
			if err != nil || exe != filepath.Join(paths.Root, name) {
				continue
			}
			before, err := processStart(proc)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			fd, err := unix.PidfdOpen(pid, 0)
			if errors.Is(err, unix.ESRCH) {
				continue
			}
			if err != nil {
				return fmt.Errorf("pidfd required for cleanup: %w", err)
			}
			after, statErr := processStart(proc)
			current, exeErr := os.Readlink(filepath.Join(proc, "exe"))
			if statErr != nil || exeErr != nil || before != after || current != exe {
				unix.Close(fd)
				continue
			}
			err = stopPID(ctx, fd, grace)
			unix.Close(fd)
			if err != nil {
				return err
			}
		}
	}
	// A process escaping a guardian must not be silently called cleaned. Do not kill by cwd:
	// it could be a user's shell; keep the Workspace unavailable for explicit investigation.
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		cwd, cwdErr := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		exe, exeErr := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if (cwdErr == nil && within(paths.Root, cwd)) || (exeErr == nil && (exe == filepath.Join(paths.Root, "nb") || exe == filepath.Join(paths.Root, "hb") || exe == filepath.Join(paths.Root, "gb"))) {
			return fmt.Errorf("runtime cleanup unconfirmed: process %s remains in workspace", entry.Name())
		}
	}
	for _, root := range []string{paths.NodeHome, paths.HostHome} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSocket != 0 {
				return os.Remove(path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func processStart(proc string) (string, error) {
	data, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return "", err
	}
	// comm is parenthesized and may itself contain spaces or parentheses.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return "", errors.New("invalid proc stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return "", errors.New("incomplete proc stat")
	}
	return fields[19], nil // field 22, counting from state (field 3).
}

func stopPID(ctx context.Context, fd int, grace time.Duration) error {
	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	deadline := time.Now().Add(grace)
	killed := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(poll, 20)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return err
		}
		if n > 0 && poll[0].Revents&unix.POLLIN != 0 {
			return nil
		}
		if !killed && time.Now().After(deadline) {
			if err := unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
				return err
			}
			killed = true
		}
	}
}
