package wgmanager

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	// LockFilePath 是进程锁文件路径
	LockFilePath = "/var/run/aria.lock"
)

// ProcessLock 进程单例锁，防止多个 Agent 实例同时运行
type ProcessLock struct {
	file *os.File
	path string
}

// AcquireProcessLock 获取进程锁
// 如果另一个实例正在运行，返回错误
func AcquireProcessLock() (*ProcessLock, error) {
	return AcquireProcessLockWithPath(LockFilePath)
}

// AcquireProcessLockWithPath 使用指定路径获取进程锁
// 优化逻辑：如果检测到陈旧锁（旧进程已退出），自动清理并继续
func AcquireProcessLockWithPath(path string) (*ProcessLock, error) {
	// 先尝试清理可能存在的陈旧锁
	if err := cleanStaleLock(path); err != nil {
		return nil, err
	}

	// 尝试打开锁文件（不创建）
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("无法访问锁文件 %s: %v", path, err)
	}

	// 尝试获取文件锁（非阻塞）
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// 文件锁被占用，检查是否是陈旧的锁
		content, _ := os.ReadFile(path)
		pidStr := strings.TrimSpace(string(content))

		if pid, err := strconv.Atoi(pidStr); err == nil {
			if !isProcessRunning(pid) {
				// 进程已退出，锁文件是陈旧的（文件锁已自动释放），清理并重试
				file.Close()
				os.Remove(path)
				return AcquireProcessLockWithPath(path)
			}
		}

		// 文件锁被占用且进程仍在运行
		file.Close()
		return nil, fmt.Errorf("❌ 错误: 另一个 Aria Agent 实例正在运行！(PID: %s)\n"+
			"请使用 'systemctl restart aria' 重启，或手动删除锁文件: sudo rm %s", pidStr, path)
	}

	// 将当前进程 PID 写入锁文件
	currentPID := os.Getpid()
	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, fmt.Errorf("无法写入锁文件: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("无法写入锁文件: %v", err)
	}
	if _, err := file.WriteString(strconv.Itoa(currentPID)); err != nil {
		file.Close()
		return nil, fmt.Errorf("无法写入锁文件: %v", err)
	}

	return &ProcessLock{
		file: file,
		path: path,
	}, nil
}

// Release 释放进程锁
func (l *ProcessLock) Release() error {
	if l.file == nil {
		return nil
	}

	// 释放文件锁
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("释放锁失败: %v", err)
	}

	// 关闭文件
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("关闭锁文件失败: %v", err)
	}

	// 删除锁文件
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除锁文件失败: %v", err)
	}

	l.file = nil
	return nil
}

// IsRunning 检查是否有其他 Aria Agent 实例正在运行
func IsRunning() (bool, string, error) {
	return IsRunningWithPath(LockFilePath)
}

// IsRunningWithPath 使用指定路径检查是否有其他实例运行
func IsRunningWithPath(path string) (bool, string, error) {
	// 尝试打开锁文件
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("无法访问锁文件: %v", err)
	}
	defer file.Close()

	// 尝试获取文件锁（非阻塞）
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// 锁被占用，说明有其他实例运行
		content, _ := os.ReadFile(path)
		pid := string(content)
		return true, pid, nil
	}

	// 能获取到锁，说明没有其他实例（释放锁）
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false, "", nil
}

// cleanStaleLock 清理陈旧的锁文件
// 如果锁文件存在但对应的进程已经不存在，则删除锁文件
func cleanStaleLock(path string) error {
	// 检查锁文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // 锁文件不存在，无需清理
	}

	// 读取锁文件中的 PID
	content, err := os.ReadFile(path)
	if err != nil {
		return nil // 无法读取，跳过清理
	}

	pidStr := string(content)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// PID 格式无效，删除陈旧锁
		os.Remove(path)
		return nil
	}

	// 检查进程是否还存在
	if !isProcessRunning(pid) {
		// 进程不存在，删除陈旧锁
		os.Remove(path)
	}

	return nil
}

// isProcessRunning 检查指定 PID 的进程是否正在运行
func isProcessRunning(pid int) bool {
	// 发送信号 0 来检查进程是否存在
	// 不会真正发送信号，只是检查权限
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// 发送信号 0 检查进程是否存在
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// ESRCH: 进程不存在
		// EPERM: 进程存在但无权限（说明进程在运行）
		if err == syscall.ESRCH {
			return false
		}
		// 其他错误（包括 EPERM）认为进程存在
		return true
	}

	return true
}
