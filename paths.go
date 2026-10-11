package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func detectAppRoot() string {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
		if fileExists(filepath.Join(exeDir, "bin", exeName("sing-box"))) {
			return exeDir
		}
	}
	cwd, _ := os.Getwd()
	if fileExists(filepath.Join(cwd, "bin", exeName("sing-box"))) {
		return cwd
	}
	if exeDir != "" && !isArchiveTempDir(exeDir) {
		return exeDir
	}
	if dir := fallbackAppRoot(); dir != "" {
		return dir
	}
	if cwd != "" && !isArchiveTempDir(cwd) {
		return cwd
	}
	return cwd
}

func fallbackAppRoot() string {
	if dir := os.Getenv("LOCALAPPDATA"); strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "GoSingBox")
	}
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "GoSingBox")
	}
	return ""
}

func isArchiveTempDir(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	tempDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(tempDir, absDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		name := strings.ToLower(part)
		if strings.HasPrefix(name, "rar$") ||
			strings.HasPrefix(name, "7z") ||
			strings.HasPrefix(name, "temp1_") ||
			strings.HasPrefix(name, "temp2_") ||
			strings.HasPrefix(name, "temp3_") {
			return true
		}
	}
	return false
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
