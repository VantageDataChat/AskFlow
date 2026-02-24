// Package loginlog provides a dedicated login audit logger that writes
// to /var/askflow/userlogin.log (Linux) or logs/userlogin.log (Windows).
//
// Features:
//   - Records admin login, logout, and session expired events
//   - Logs username, IP address, timestamp, and event type
//   - Automatic log rotation when file exceeds maxFileSize
//   - Rotated logs are gzip-compressed to save disk space
//   - Retains up to maxBackups compressed archives
//   - Thread-safe: all operations are protected by a mutex
package loginlog

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogDir = "/var/askflow"
	windowsLogDir = "logs"
	logFileName   = "userlogin.log"

	// maxFileSize is the threshold in bytes before rotation (100 MB).
	maxFileSize = 100 << 20
	// maxBackups is the number of compressed archives to keep.
	maxBackups = 5
	// writeBufSize is the size of the internal write buffer.
	writeBufSize = 4096
)

// Event types for login audit log.
const (
	EventLogin          = "LOGIN"
	EventLoginFailed    = "LOGIN_FAILED"
	EventLogout         = "LOGOUT"
	EventSessionExpired = "SESSION_EXPIRED"
)

// logger is the package-level singleton.
var (
	global *loginLogger
	mu     sync.Mutex
)

// loginLogger holds the state for the rotating login log writer.
type loginLogger struct {
	mu         sync.Mutex
	file       *os.File
	dir        string
	path       string
	size       int64
	buf        []byte
	closed     bool
	maxRotSize int64
}

// Init initializes the login logger. Safe to call multiple times.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if global != nil {
		return nil
	}

	dir := defaultLogDir
	if runtime.GOOS == "windows" {
		dir = windowsLogDir
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create login log directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open login log file %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat login log file: %w", err)
	}

	global = &loginLogger{
		file:       f,
		dir:        dir,
		path:       path,
		size:       info.Size(),
		buf:        make([]byte, 0, writeBufSize),
		maxRotSize: maxFileSize,
	}
	return nil
}

// Log writes a login audit entry.
// event is one of EventLogin, EventLoginFailed, EventLogout, EventSessionExpired.
func Log(event, username, ip, detail string) {
	mu.Lock()
	l := global
	mu.Unlock()

	if l == nil {
		return
	}
	l.log(event, username, ip, detail)
}

// Close flushes and closes the login log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if global == nil {
		return
	}
	global.close()
	global = nil
}

// --- internal methods ---

// sanitizeLogField replaces newlines and control characters in a log field
// to prevent log injection attacks. An attacker-controlled username or IP
// containing \n could forge additional log entries.
func sanitizeLogField(s string) string {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' {
			clean = append(clean, ' ')
		} else if c < 0x20 && c != '\t' {
			// Replace other control characters with space
			clean = append(clean, ' ')
		} else {
			clean = append(clean, c)
		}
	}
	return string(clean)
}

// log formats and writes a login audit entry.
func (l *loginLogger) log(event, username, ip, detail string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed || l.file == nil {
		return
	}

	// Sanitize user-controlled fields to prevent log injection
	username = sanitizeLogField(username)
	ip = sanitizeLogField(ip)
	detail = sanitizeLogField(detail)

	// Format: "2006/01/02 15:04:05 [EVENT] user=<username> ip=<ip> <detail>\n"
	now := time.Now()
	l.buf = l.buf[:0]
	l.buf = now.AppendFormat(l.buf, "2006/01/02 15:04:05")
	l.buf = append(l.buf, " ["...)
	l.buf = append(l.buf, event...)
	l.buf = append(l.buf, "] user="...)
	l.buf = append(l.buf, username...)
	l.buf = append(l.buf, " ip="...)
	l.buf = append(l.buf, ip...)
	if detail != "" {
		l.buf = append(l.buf, ' ')
		l.buf = append(l.buf, detail...)
	}
	if len(l.buf) == 0 || l.buf[len(l.buf)-1] != '\n' {
		l.buf = append(l.buf, '\n')
	}

	n, err := l.file.Write(l.buf)
	if err != nil {
		return
	}
	l.size += int64(n)

	if l.size >= l.maxRotSize {
		l.rotate()
	}
}

// rotate compresses the current log file and opens a fresh one.
func (l *loginLogger) rotate() {
	l.file.Sync()
	l.file.Close()
	l.file = nil

	ts := time.Now().Format("20060102-150405")
	archiveName := fmt.Sprintf("userlogin-%s.log.gz", ts)
	archivePath := filepath.Join(l.dir, archiveName)

	if err := compressFile(l.path, archivePath); err != nil {
		os.Truncate(l.path, 0)
	} else {
		os.Truncate(l.path, 0)
	}

	l.pruneArchives()

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	l.file = f
	l.size = 0
}

// pruneArchives removes old compressed archives beyond maxBackups.
func (l *loginLogger) pruneArchives() {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}

	var archives []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "userlogin-") && strings.HasSuffix(name, ".log.gz") {
			archives = append(archives, name)
		}
	}

	if len(archives) <= maxBackups {
		return
	}

	sort.Strings(archives)
	toRemove := archives[:len(archives)-maxBackups]
	for _, name := range toRemove {
		os.Remove(filepath.Join(l.dir, name))
	}
}

func (l *loginLogger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true
	if l.file != nil {
		l.file.Sync()
		l.file.Close()
		l.file = nil
	}
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	gw, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}

	if _, err := io.Copy(gw, in); err != nil {
		gw.Close()
		out.Close()
		os.Remove(dst)
		return err
	}

	if err := gw.Close(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

// --- Exported helpers for log management API ---

// GetLogDir returns the login log directory path.
func GetLogDir() string {
	if runtime.GOOS == "windows" {
		return windowsLogDir
	}
	return defaultLogDir
}

// GetLogPath returns the full path to the current login log file.
func GetLogPath() string {
	return filepath.Join(GetLogDir(), logFileName)
}

// GetRotationSizeMB returns the current rotation threshold in megabytes.
func GetRotationSizeMB() int {
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		return int(global.maxRotSize >> 20)
	}
	return int(maxFileSize >> 20)
}

// SetRotationSizeMB updates the rotation threshold. sizeMB must be >= 1.
func SetRotationSizeMB(sizeMB int) {
	if sizeMB < 1 {
		sizeMB = 1
	}
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		global.mu.Lock()
		global.maxRotSize = int64(sizeMB) << 20
		global.mu.Unlock()
	}
}

// RecentLines reads the last n lines from the current login log file.
func RecentLines(n int) ([]string, error) {
	if n <= 0 {
		n = 50
	}
	path := GetLogPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return []string{}, nil
	}

	const maxRead = 256 * 1024
	readStart := int64(0)
	if size > maxRead {
		readStart = size - maxRead
	}
	readLen := size - readStart

	buf := make([]byte, readLen)
	_, err = f.ReadAt(buf, readStart)
	if err != nil && err != io.EOF {
		return nil, err
	}

	lines := make([]string, 0, n)
	end := len(buf)
	if end > 0 && buf[end-1] == '\n' {
		end--
	}
	for i := end - 1; i >= 0 && len(lines) < n; i-- {
		if buf[i] == '\n' {
			line := string(buf[i+1 : end])
			if line != "" {
				lines = append(lines, line)
			}
			end = i
		}
	}
	if len(lines) < n && end > 0 {
		line := string(buf[:end])
		if line != "" {
			lines = append(lines, line)
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}

// ListArchives returns the names of compressed login log archives.
func ListArchives() ([]string, error) {
	dir := GetLogDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var archives []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "userlogin-") && strings.HasSuffix(name, ".log.gz") {
			archives = append(archives, name)
		}
	}
	sort.Strings(archives)
	return archives, nil
}

// ClearLogs truncates the current login log file and removes all archived logs.
func ClearLogs() (int, error) {
	mu.Lock()
	defer mu.Unlock()

	dir := GetLogDir()
	path := filepath.Join(dir, logFileName)

	entries, err := os.ReadDir(dir)
	archivesRemoved := 0
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "userlogin-") && strings.HasSuffix(name, ".log.gz") {
				if os.Remove(filepath.Join(dir, name)) == nil {
					archivesRemoved++
				}
			}
		}
	}

	if global != nil {
		global.mu.Lock()
		if global.file != nil {
			global.file.Sync()
			global.file.Close()
			global.file = nil
		}
		os.Truncate(path, 0)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			global.file = f
			global.size = 0
		}
		global.mu.Unlock()
	} else {
		os.Truncate(path, 0)
	}

	return archivesRemoved, nil
}
