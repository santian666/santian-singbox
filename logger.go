package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logBufferLimit = 800
	logKeepFor     = 5 * time.Minute
)

type Logger struct {
	mu        sync.Mutex
	path      string
	transient bool
	file      *os.File
	writer    *bufio.Writer
	fileBytes int64
	maxBytes  int64
	lastSync  time.Time
	buffer    []LogMessage
	created   []time.Time
	listener  func(LogMessage)
}

func NewLogger(path string) *Logger {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	logger := &Logger{path: path, transient: isTempLogPath(path)}
	if !logger.transient {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return logger
		}
		logger.file = file
		logger.writer = bufio.NewWriterSize(file, 32*1024)
		if info, statErr := file.Stat(); statErr == nil {
			logger.fileBytes = info.Size()
		}
	}
	return logger
}

func isTempLogPath(path string) bool {
	cleanPath := strings.ToLower(filepath.Clean(path))
	tempDir := strings.ToLower(filepath.Clean(os.TempDir()))
	return cleanPath == tempDir || strings.HasPrefix(cleanPath, tempDir+string(os.PathSeparator))
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.add("INFO", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.add("ERROR", format, args...)
}

func (l *Logger) Logs() []LogMessage {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.trimLocked(time.Now())
	out := make([]LogMessage, len(l.buffer))
	copy(out, l.buffer)
	return out
}

func (l *Logger) add(level, format string, args ...interface{}) {
	text := stripANSI(fmt.Sprintf(format, args...))
	now := time.Now()
	msg := LogMessage{
		Time:  now.Format("15:04:05"),
		Level: level,
		Text:  text,
	}
	l.mu.Lock()
	l.buffer = append(l.buffer, msg)
	l.created = append(l.created, now)
	l.trimLocked(now)
	l.writeFileLocked(now, level, msg.Text)
	listener := l.listener
	l.mu.Unlock()
	if listener != nil {
		listener(msg)
	}
}

func (l *Logger) SetListener(listener func(LogMessage)) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.listener = listener
	l.mu.Unlock()
}

func (l *Logger) writeFileLocked(now time.Time, level, text string) {
	if l.maxBytes > 0 && l.fileBytes >= l.maxBytes {
		l.rotateFileLocked()
	}
	if l.writer == nil {
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		l.file = file
		if l.transient {
			l.writer = bufio.NewWriter(file)
		} else {
			l.writer = bufio.NewWriterSize(file, 32*1024)
		}
	}
	written, _ := fmt.Fprintf(l.writer, "%s [%s] %s\n", now.Format("2006-01-02 15:04:05"), level, text)
	l.fileBytes += int64(written)
	if l.transient {
		l.closeFileLocked()
		return
	}
	if level == "ERROR" || now.Sub(l.lastSync) >= time.Second {
		_ = l.writer.Flush()
		l.lastSync = now
	}
}

func (l *Logger) closeFileLocked() {
	if l.writer != nil {
		_ = l.writer.Flush()
		l.writer = nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.lastSync = time.Time{}
}

func (l *Logger) rotateFileLocked() {
	l.closeFileLocked()
	backup := l.path + ".bak"
	_ = os.Remove(backup)
	_ = os.Rename(l.path, backup)
	l.fileBytes = 0
}

func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeFileLocked()
}

func (l *Logger) trimLocked(now time.Time) {
	if len(l.buffer) == 0 {
		return
	}
	if len(l.created) != len(l.buffer) {
		l.created = make([]time.Time, len(l.buffer))
	}
	if logKeepFor > 0 {
		cutoff := now.Add(-logKeepFor)
		start := 0
		for start < len(l.created) && !l.created[start].IsZero() && l.created[start].Before(cutoff) {
			start++
		}
		if start > 0 && start < len(l.buffer) {
			l.dropPrefixLocked(start)
		} else if start >= len(l.buffer) {
			l.clearBufferLocked()
		}
	}
	if len(l.buffer) > logBufferLimit {
		l.dropPrefixLocked(len(l.buffer) - logBufferLimit)
	}
}

func (l *Logger) dropPrefixLocked(count int) {
	if count <= 0 {
		return
	}
	if count >= len(l.buffer) {
		l.clearBufferLocked()
		return
	}
	copy(l.buffer, l.buffer[count:])
	clear(l.buffer[len(l.buffer)-count:])
	l.buffer = l.buffer[:len(l.buffer)-count]
	copy(l.created, l.created[count:])
	clear(l.created[len(l.created)-count:])
	l.created = l.created[:len(l.created)-count]
}

func (l *Logger) clearBufferLocked() {
	clear(l.buffer)
	clear(l.created)
	l.buffer = l.buffer[:0]
	l.created = l.created[:0]
	if cap(l.buffer) > logBufferLimit*2 {
		l.buffer = nil
	}
	if cap(l.created) > logBufferLimit*2 {
		l.created = nil
	}
}

func (l *Logger) RotateIfLarge(maxBytes int64) {
	if l == nil || maxBytes <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxBytes = maxBytes
	info, err := os.Stat(l.path)
	if err == nil {
		l.fileBytes = info.Size()
	}
	if err != nil || l.fileBytes <= maxBytes {
		return
	}
	l.rotateFileLocked()
	l.writeFileLocked(time.Now(), "INFO", "日志文件已轮转")
}
