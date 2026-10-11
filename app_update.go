package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultAppUpdateRepo    = "santian666/santian-singbox"
	appUpdateCheckTimeout   = 10 * time.Second
	appUpdateDownloadLimit  = 300 * 1024 * 1024 // 300 MB limit
)

type AppUpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes"`
	DownloadURL    string `json:"downloadUrl"`
	AssetSize      int64  `json:"assetSize"`
	PublishedAt    string `json:"publishedAt"`
	Status         string `json:"status"` // "update_available", "already_latest", "check_failed"
	Message        string `json:"message,omitempty"`
}

type AppUpdateProgress struct {
	Phase       string  `json:"phase"` // "downloading", "extracting", "applying", "ready"
	Percent     float64 `json:"percent"`
	Bytes       int64   `json:"bytes"`
	Total       int64   `json:"total"`
	SpeedString string  `json:"speedString"`
	Message     string  `json:"message"`
}

type AppUpdateManager struct {
	mu           sync.Mutex
	repo         string
	lastInfo     *AppUpdateInfo
	lastCheckAt  time.Time
	isUpdating   bool
	logger       *Logger
}

func newAppUpdateManager(logger *Logger) *AppUpdateManager {
	return &AppUpdateManager{
		repo:   defaultAppUpdateRepo,
		logger: logger,
	}
}

func compareSemver(v1, v2 string) int {
	v1 = strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2 = strings.TrimPrefix(strings.TrimSpace(v2), "v")
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

func (m *AppUpdateManager) CheckUpdate(proxyPort int) (*AppUpdateInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", m.repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SanTian-Singbox/"+AppVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyPort > 0 {
		proxyParsed, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort))
		if proxyParsed != nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
		}
	}

	client := &http.Client{Timeout: appUpdateCheckTimeout, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		// Fallback without proxy if proxy failed
		if proxyPort > 0 {
			transport.Proxy = nil
			resp, err = client.Do(req)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("检查软件更新失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &AppUpdateInfo{
			Available:      false,
			CurrentVersion: AppVersion,
			Status:         "already_latest",
			Message:        "未发布新版本",
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回状态码 %d", resp.StatusCode)
	}

	var release struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析发布信息失败：%w", err)
	}

	latestVer := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	curVer := strings.TrimPrefix(strings.TrimSpace(AppVersion), "v")

	var downloadURL string
	var assetSize int64

	// Prefer zip asset matching windows-amd64
	for _, asset := range release.Assets {
		lower := strings.ToLower(asset.Name)
		if strings.HasSuffix(lower, ".zip") && (strings.Contains(lower, "windows") || strings.Contains(lower, "amd64") || strings.Contains(lower, "santian")) {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			break
		}
	}
	if downloadURL == "" && len(release.Assets) > 0 {
		for _, asset := range release.Assets {
			if strings.HasSuffix(strings.ToLower(asset.Name), ".zip") {
				downloadURL = asset.BrowserDownloadURL
				assetSize = asset.Size
				break
			}
		}
	}

	isNewer := compareSemver(latestVer, curVer) > 0 && downloadURL != ""

	info := &AppUpdateInfo{
		Available:      isNewer,
		CurrentVersion: AppVersion,
		LatestVersion:  release.TagName,
		ReleaseNotes:   release.Body,
		DownloadURL:    downloadURL,
		AssetSize:      assetSize,
		PublishedAt:    release.PublishedAt,
	}

	if isNewer {
		info.Status = "update_available"
	} else {
		info.Status = "already_latest"
	}

	m.lastInfo = info
	m.lastCheckAt = time.Now()
	return info, nil
}

type progressWriter struct {
	total       int64
	downloaded  int64
	lastEmit    time.Time
	lastBytes   int64
	onProgress  func(downloaded, total int64, speed string, percent float64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)

	now := time.Now()
	if now.Sub(pw.lastEmit) >= 200*time.Millisecond || pw.downloaded == pw.total {
		elapsed := now.Sub(pw.lastEmit).Seconds()
		var speedStr string
		if elapsed > 0 {
			speedBytes := float64(pw.downloaded-pw.lastBytes) / elapsed
			if speedBytes >= 1024*1024 {
				speedStr = fmt.Sprintf("%.2f MB/s", speedBytes/(1024*1024))
			} else {
				speedStr = fmt.Sprintf("%.1f KB/s", speedBytes/1024)
			}
		}
		pw.lastEmit = now
		pw.lastBytes = pw.downloaded

		var percent float64
		if pw.total > 0 {
			percent = float64(pw.downloaded) / float64(pw.total) * 100
		}
		if pw.onProgress != nil {
			pw.onProgress(pw.downloaded, pw.total, speedStr, percent)
		}
	}
	return n, nil
}

func downloadUpdateAsset(downloadURL string, destPath string, proxyPort int, onProgress func(downloaded, total int64, speed string, percent float64)) error {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SanTian-Singbox/"+AppVersion)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyPort > 0 {
		proxyParsed, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort))
		if proxyParsed != nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
		}
	}

	client := &http.Client{Timeout: 10 * time.Minute, Transport: transport}
	resp, err := client.Do(req)
	if err != nil && proxyPort > 0 {
		transport.Proxy = nil
		resp, err = client.Do(req)
	}
	if err != nil {
		return fmt.Errorf("下载更新包失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载服务器返回状态码 %d", resp.StatusCode)
	}

	total := resp.ContentLength
	if total > appUpdateDownloadLimit {
		return errors.New("更新文件大小超过安全限制")
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := &progressWriter{
		total:      total,
		lastEmit:   time.Now(),
		onProgress: onProgress,
	}

	_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
	return err
}

func extractZipArchive(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue // Prevent zip slip
		}
		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(path, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		_ = src.Close()
		_ = dst.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func locateExtractedAppDir(stageDir string, exeName string) string {
	// Check if stageDir directly contains the exe
	if _, err := os.Stat(filepath.Join(stageDir, exeName)); err == nil {
		return stageDir
	}
	// Check one-level subdirectories
	entries, err := os.ReadDir(stageDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				subPath := filepath.Join(stageDir, e.Name())
				if _, err := os.Stat(filepath.Join(subPath, exeName)); err == nil {
					return subPath
				}
			}
		}
	}
	return stageDir
}

func generateUpdaterScript(updaterPath string) error {
	scriptContent := `@echo off
chcp 65001 >nul
setlocal EnableExtensions EnableDelayedExpansion

set "APP_DIR=%~1"
set "STAGE_DIR=%~2"
set "EXE_NAME=%~3"
set "OLD_PID=%~4"

if "%APP_DIR%"=="" exit /b 1
if "%STAGE_DIR%"=="" exit /b 1
if "%EXE_NAME%"=="" exit /b 1

:: 等待旧应用进程退出
:wait_pid_loop
if not "%OLD_PID%"=="" (
    tasklist /fi "pid eq %OLD_PID%" 2>nul | find "%OLD_PID%" >nul
    if not errorlevel 1 (
        timeout /t 1 /nobreak >nul
        goto wait_pid_loop
    )
)
timeout /t 1 /nobreak >nul

:: 备份并安全保护数据目录
set "BACKUP_DATA=%TEMP%\santian_data_backup_%RANDOM%"
if exist "%APP_DIR%\data\gui\state.dat" (
    mkdir "%BACKUP_DATA%" >nul 2>nul
    copy /y "%APP_DIR%\data\gui\state.dat" "%BACKUP_DATA%\" >nul 2>nul
    copy /y "%APP_DIR%\data\gui\selected.dat" "%BACKUP_DATA%\" >nul 2>nul
)

:: 覆盖新版本文件
xcopy /e /i /y "%STAGE_DIR%\*" "%APP_DIR%\" >nul

:: 恢复用户配置
if exist "%BACKUP_DATA%\state.dat" (
    mkdir "%APP_DIR%\data\gui" >nul 2>nul
    copy /y "%BACKUP_DATA%\state.dat" "%APP_DIR%\data\gui\state.dat" >nul 2>nul
    copy /y "%BACKUP_DATA%\selected.dat" "%APP_DIR%\data\gui\selected.dat" >nul 2>nul
    rmdir /s /q "%BACKUP_DATA%" >nul 2>nul
)

:: 启动新版本程序
start "" "%APP_DIR%\%EXE_NAME%"

:: 清理暂存文件
timeout /t 2 /nobreak >nul
rmdir /s /q "%STAGE_DIR%" >nul 2>nul
exit
`
	return os.WriteFile(updaterPath, []byte(scriptContent), 0755)
}

func launchDetachedUpdater(scriptPath, appDir, stageDir, exeName string, pid int) error {
	cmd := exec.Command("cmd.exe", "/c", scriptPath, appDir, stageDir, exeName, strconv.Itoa(pid))
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd.Start()
}
