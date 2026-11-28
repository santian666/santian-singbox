package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultAppUpdateRepo          = "santian666/santian-singbox"
	appUpdateCheckTimeout         = 10 * time.Second
	appUpdateAPIResponseLimit     = 4 * 1024 * 1024   // 4 MiB GitHub release JSON limit
	appUpdateDownloadLimit        = 300 * 1024 * 1024 // 300 MiB archive download limit
	appUpdateMaxExtractedFileSize = 256 * 1024 * 1024 // 256 MiB single file extraction limit
	appUpdateMaxExtractedTotal    = 512 * 1024 * 1024 // 512 MiB cumulative extraction limit
)

type AppUpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes"`
	DownloadURL    string `json:"downloadUrl"`
	AssetSize      int64  `json:"assetSize"`
	AssetDigest    string `json:"assetDigest,omitempty"`
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
	mu          sync.Mutex
	repo        string
	lastInfo    *AppUpdateInfo
	lastCheckAt time.Time
	isUpdating  bool
	logger      *Logger
}

func newAppUpdateManager(logger *Logger) *AppUpdateManager {
	return &AppUpdateManager{
		repo:   defaultAppUpdateRepo,
		logger: logger,
	}
}

func (m *AppUpdateManager) LastInfo() (*AppUpdateInfo, time.Time) {
	if m == nil {
		return nil, time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastInfo == nil {
		return nil, m.lastCheckAt
	}
	copied := *m.lastInfo
	return &copied, m.lastCheckAt
}

func (m *AppUpdateManager) TryStartUpdate() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isUpdating {
		return false
	}
	m.isUpdating = true
	return true
}

func (m *AppUpdateManager) FinishUpdate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.isUpdating = false
	m.mu.Unlock()
}

func isTrustedGitHubDownloadHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	if h == "github.com" || h == "api.github.com" || h == "codeload.github.com" {
		return true
	}
	if strings.HasSuffix(h, ".githubusercontent.com") || strings.HasSuffix(h, ".github.com") {
		return true
	}
	return false
}

func validateTrustedUpdateURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("更新地址无效：%w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("拒绝非 HTTPS 更新下载地址：%s", parsed.Scheme)
	}
	if !isTrustedGitHubDownloadHost(parsed.Hostname()) {
		return fmt.Errorf("拒绝非官方 GitHub 域名的更新下载地址：%s", parsed.Hostname())
	}
	return nil
}

var validateUpdateDownloadURL = validateTrustedUpdateURL

func trustedUpdateRedirectCheck(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("更新下载重定向次数过多")
	}
	return validateUpdateDownloadURL(req.URL.String())
}

func parseExpectedSHA256Digest(digest string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(digest))
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "sha256:") {
		trimmed = strings.TrimPrefix(trimmed, "sha256:")
	}
	if len(trimmed) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(trimmed); err != nil {
		return "", false
	}
	return trimmed, true
}

func verifyFileSHA256(filePath string, expectedDigest string) error {
	wantHex, ok := parseExpectedSHA256Digest(expectedDigest)
	if !ok {
		return nil
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}
	gotHex := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(gotHex, wantHex) {
		return fmt.Errorf("更新包 SHA-256 校验失败（期望 %s，实际 %s）", wantHex[:12], gotHex[:12])
	}
	return nil
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
	repo := m.repo
	m.mu.Unlock()

	if proxyPort <= 0 {
		return nil, errors.New("proxy_not_running")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SanTian-Singbox/"+AppVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	proxyParsed, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort))
	if proxyParsed != nil {
		transport.Proxy = http.ProxyURL(proxyParsed)
	}

	client := &http.Client{
		Timeout:       appUpdateCheckTimeout,
		Transport:     transport,
		CheckRedirect: trustedUpdateRedirectCheck,
	}
	resp, err := client.Do(req)
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
			Digest             string `json:"digest"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, appUpdateAPIResponseLimit)).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析发布信息失败：%w", err)
	}

	latestVer := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	curVer := strings.TrimPrefix(strings.TrimSpace(AppVersion), "v")

	var downloadURL string
	var assetSize int64
	var assetDigest string

	for _, asset := range release.Assets {
		lower := strings.ToLower(asset.Name)
		if strings.HasSuffix(lower, ".zip") && (strings.Contains(lower, "windows") || strings.Contains(lower, "amd64") || strings.Contains(lower, "santian")) {
			if validateTrustedUpdateURL(asset.BrowserDownloadURL) == nil {
				downloadURL = asset.BrowserDownloadURL
				assetSize = asset.Size
				assetDigest = strings.TrimSpace(asset.Digest)
				break
			}
		}
	}
	if downloadURL == "" && len(release.Assets) > 0 {
		for _, asset := range release.Assets {
			if strings.HasSuffix(strings.ToLower(asset.Name), ".zip") && validateTrustedUpdateURL(asset.BrowserDownloadURL) == nil {
				downloadURL = asset.BrowserDownloadURL
				assetSize = asset.Size
				assetDigest = strings.TrimSpace(asset.Digest)
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
		AssetDigest:    assetDigest,
		PublishedAt:    release.PublishedAt,
	}

	if isNewer {
		info.Status = "update_available"
	} else {
		info.Status = "already_latest"
	}

	m.mu.Lock()
	copied := *info
	m.lastInfo = &copied
	m.lastCheckAt = time.Now()
	m.mu.Unlock()
	return info, nil
}

type progressWriter struct {
	total      int64
	downloaded int64
	lastEmit   time.Time
	lastBytes  int64
	onProgress func(downloaded, total int64, speed string, percent float64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)

	now := time.Now()
	if now.Sub(pw.lastEmit) >= 200*time.Millisecond || (pw.total > 0 && pw.downloaded == pw.total) {
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

func isValidUpdateZip(zipPath string, expectedSize int64, expectedDigest string) bool {
	fi, err := os.Stat(zipPath)
	if err != nil || fi.Size() == 0 {
		return false
	}
	if expectedSize > 0 && fi.Size() != expectedSize {
		return false
	}
	// 安全校验：若发布元数据提供 SHA-256 摘要，必须严格匹配才允许复用缓存
	if _, hasDigest := parseExpectedSHA256Digest(expectedDigest); hasDigest {
		if err := verifyFileSHA256(zipPath, expectedDigest); err != nil {
			return false
		}
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer reader.Close()

	for _, file := range reader.File {
		normalizedName := strings.ReplaceAll(file.Name, "\\", "/")
		base := filepath.Base(normalizedName)
		if strings.EqualFold(base, "SanTian-Singbox.exe") {
			return true
		}
	}
	return false
}

func downloadUpdateAsset(downloadURL string, destPath string, proxyPort int, expectedSize int64, expectedDigest string, onProgress func(downloaded, total int64, speed string, percent float64)) error {
	if err := validateTrustedUpdateURL(downloadURL); err != nil {
		return err
	}
	if proxyPort <= 0 {
		return errors.New("请先开启代理后再下载更新包")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	proxyParsed, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort))
	if proxyParsed != nil {
		transport.Proxy = http.ProxyURL(proxyParsed)
	}
	client := &http.Client{
		Timeout:       10 * time.Minute,
		Transport:     transport,
		CheckRedirect: trustedUpdateRedirectCheck,
	}

	for attempt := 0; attempt < 2; attempt++ {
		var existingSize int64
		if attempt == 0 {
			if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
				if expectedSize > 0 && fi.Size() < expectedSize {
					existingSize = fi.Size()
				} else {
					_ = os.Remove(destPath)
				}
			}
		}

		req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "SanTian-Singbox/"+AppVersion)
		if existingSize > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("下载更新包失败：%w", err)
		}

		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && attempt == 0 {
			_ = resp.Body.Close()
			_ = os.Remove(destPath)
			continue
		}

		var out *os.File
		var total int64
		var downloadedSoFar int64

		if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
			out, err = os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				_ = resp.Body.Close()
				return err
			}
			if resp.ContentLength > 0 {
				total = existingSize + resp.ContentLength
			}
			downloadedSoFar = existingSize
		} else if resp.StatusCode == http.StatusOK {
			out, err = os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				_ = resp.Body.Close()
				return err
			}
			total = resp.ContentLength
			downloadedSoFar = 0
		} else {
			statusCode := resp.StatusCode
			_ = resp.Body.Close()
			return fmt.Errorf("下载服务器返回状态码 %d", statusCode)
		}

		if total > appUpdateDownloadLimit || downloadedSoFar >= appUpdateDownloadLimit {
			_ = out.Close()
			_ = resp.Body.Close()
			_ = os.Remove(destPath)
			return errors.New("更新文件大小超过安全限制")
		}

		remainingLimit := appUpdateDownloadLimit - downloadedSoFar
		pw := &progressWriter{
			total:      total,
			downloaded: downloadedSoFar,
			lastEmit:   time.Now(),
			lastBytes:  downloadedSoFar,
			onProgress: onProgress,
		}

		written, copyErr := io.Copy(out, io.TeeReader(io.LimitReader(resp.Body, remainingLimit+1), pw))
		closeErr := out.Close()
		_ = resp.Body.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > remainingLimit {
			_ = os.Remove(destPath)
			return errors.New("更新文件大小超过安全限制")
		}
		if err := verifyFileSHA256(destPath, expectedDigest); err != nil {
			_ = os.Remove(destPath)
			return err
		}
		return nil
	}
	return errors.New("下载更新包失败：断点续传范围无效")
}

func extractZipArchive(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	cleanDest := filepath.Clean(destDir)
	prefix := cleanDest + string(os.PathSeparator)
	var totalWritten int64

	for _, file := range reader.File {
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝解压符号链接条目：%s", file.Name)
		}
		normalizedName := strings.ReplaceAll(file.Name, "\\", "/")
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(normalizedName, "/")

		path := filepath.Join(cleanDest, filepath.FromSlash(normalizedName))
		if !strings.HasPrefix(filepath.Clean(path), prefix) {
			continue // Prevent zip slip
		}
		if isDir {
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
		mode := file.Mode() & 0755
		if mode == 0 {
			mode = 0644
		}
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = src.Close()
			return err
		}
		remainingTotal := appUpdateMaxExtractedTotal - totalWritten
		if remainingTotal <= 0 {
			_ = src.Close()
			_ = dst.Close()
			return errors.New("更新包解压总大小超过安全限制")
		}
		fileLimit := int64(appUpdateMaxExtractedFileSize)
		if remainingTotal < fileLimit {
			fileLimit = remainingTotal
		}
		n, copyErr := io.Copy(dst, io.LimitReader(src, fileLimit+1))
		_ = src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n > fileLimit {
			return fmt.Errorf("更新包解压体积超过安全限制（%s）", file.Name)
		}
		totalWritten += n
	}
	return nil
}

func locateExtractedAppDir(stageDir string, exeName string) string {
	if _, err := os.Stat(filepath.Join(stageDir, exeName)); err == nil {
		return stageDir
	}
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

func performAtomicInPlaceUpdate(appRoot, extractedAppDir, exePath string) error {
	exeName := filepath.Base(exePath)

	// 1. Rename currently running SanTian-Singbox.exe -> .old (NTFS allows renaming running binaries)
	oldExe := exePath + ".old"
	_ = os.Remove(oldExe)

	var renameErr error
	for i := 0; i < 5; i++ {
		renameErr = os.Rename(exePath, oldExe)
		if renameErr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if renameErr != nil {
		return fmt.Errorf("重命名旧主程序失败（可能被安全软件或系统占用）：%w", renameErr)
	}

	// 2. Copy new executable directly into place
	newExeSrc := filepath.Join(extractedAppDir, exeName)
	if err := copyFile(newExeSrc, exePath); err != nil {
		_ = os.Rename(oldExe, exePath)
		return fmt.Errorf("复制新主程序失败：%w", err)
	}

	// 3. Update bin/ directory with rollback on failure
	stagingBin := filepath.Join(extractedAppDir, "bin")
	if entries, err := os.ReadDir(stagingBin); err == nil {
		dstBinDir := filepath.Join(appRoot, "bin")
		if err := os.MkdirAll(dstBinDir, 0755); err != nil {
			_ = os.Remove(exePath)
			_ = os.Rename(oldExe, exePath)
			return fmt.Errorf("创建 bin 目录失败：%w", err)
		}
		type swappedFile struct {
			dst string
			old string
			had bool
		}
		var swapped []swappedFile
		rollbackBin := func() {
			for i := len(swapped) - 1; i >= 0; i-- {
				item := swapped[i]
				_ = os.Remove(item.dst)
				if item.had {
					_ = os.Rename(item.old, item.dst)
				}
			}
			_ = os.Remove(exePath)
			_ = os.Rename(oldExe, exePath)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			srcFile := filepath.Join(stagingBin, e.Name())
			dstFile := filepath.Join(dstBinDir, e.Name())
			dstOld := dstFile + ".old"
			_ = os.Remove(dstOld)
			hadOld := false
			if fileExists(dstFile) {
				if err := os.Rename(dstFile, dstOld); err != nil {
					rollbackBin()
					return fmt.Errorf("备份旧内核文件 %s 失败：%w", e.Name(), err)
				}
				hadOld = true
			}
			if err := copyFile(srcFile, dstFile); err != nil {
				if hadOld {
					_ = os.Rename(dstOld, dstFile)
				}
				rollbackBin()
				return fmt.Errorf("更新内核文件 %s 失败：%w", e.Name(), err)
			}
			swapped = append(swapped, swappedFile{dst: dstFile, old: dstOld, had: hadOld})
		}
	}

	// 4. Update data/gui/rules/ directory (only rules, NEVER touching state.dat or user configs)
	stagingRules := filepath.Join(extractedAppDir, "data", "gui", "rules")
	if entries, err := os.ReadDir(stagingRules); err == nil {
		dstRulesDir := filepath.Join(appRoot, "data", "gui", "rules")
		if err := os.MkdirAll(dstRulesDir, 0755); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				srcFile := filepath.Join(stagingRules, e.Name())
				dstFile := filepath.Join(dstRulesDir, e.Name())
				if err := copyFile(srcFile, dstFile); err != nil {
					return fmt.Errorf("更新规则文件 %s 失败：%w", e.Name(), err)
				}
			}
		}
	}

	return nil
}

func launchNewAppProcess(exePath, appRoot string) error {
	args := []string{adminRestartArg}
	if pid := os.Getpid(); pid > 0 {
		args = append(args, waitParentPIDArgPrefix+strconv.Itoa(pid))
	}
	cmd := exec.Command(exePath, args...)
	cmd.Dir = appRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}

func cleanupOldUpdateArtifacts(appRoot string) {
	exePath, err := os.Executable()
	if err == nil && exePath != "" {
		_ = os.Remove(exePath + ".old")
	}
	binDir := filepath.Join(appRoot, "bin")
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".old") {
				_ = os.Remove(filepath.Join(binDir, e.Name()))
			}
		}
	}

	updatesDir := filepath.Join(appRoot, "data", "updates")
	targetVersionPath := filepath.Join(updatesDir, "target_version.txt")
	targetVerBytes, err := os.ReadFile(targetVersionPath)
	if err == nil {
		targetVer := strings.TrimSpace(string(targetVerBytes))
		if targetVer != "" {
			if compareSemver(AppVersion, targetVer) >= 0 {
				_ = os.RemoveAll(updatesDir)
				return
			}
			_ = os.RemoveAll(filepath.Join(updatesDir, "staging"))
			return
		}
	}
	_ = os.RemoveAll(filepath.Join(updatesDir, "staging"))
}

func (a *App) ApplyAppUpdate() (result *AppUpdateInfo, err error) {
	defer a.recoverError("执行软件更新", &err)
	if a == nil || a.appUpdater == nil {
		return nil, errors.New("软件更新管理器未初始化")
	}
	if a.manager == nil || !a.manager.Running() {
		return nil, errors.New("请先开启代理后再执行客户端更新（开启代理可保证下载速度与连接稳定性）")
	}
	if !a.appUpdater.TryStartUpdate() {
		return nil, errors.New("软件更新正在进行中，请勿重复操作")
	}
	defer a.appUpdater.FinishUpdate()

	proxyPort := 0
	if a.store != nil {
		proxyPort = a.store.Snapshot().Settings.MixedPort
	}
	if proxyPort <= 0 {
		proxyPort = 20808
	}

	info, err := a.appUpdater.CheckUpdate(proxyPort)
	if err != nil {
		return nil, err
	}
	if !info.Available || info.DownloadURL == "" {
		return info, errors.New("未检测到可更新的版本下载包")
	}

	updatesDir := filepath.Join(a.root, "data", "updates")
	_ = os.MkdirAll(updatesDir, 0700)
	zipPath := filepath.Join(updatesDir, "update.zip")
	targetVersionPath := filepath.Join(updatesDir, "target_version.txt")
	stageDir := filepath.Join(updatesDir, "staging")

	// 1. 检查本地是否已存在针对目标版本且完整（含 SHA-256）有效的安装包
	needDownload := true
	savedVerBytes, _ := os.ReadFile(targetVersionPath)
	savedVer := strings.TrimSpace(string(savedVerBytes))
	if savedVer == info.LatestVersion && isValidUpdateZip(zipPath, info.AssetSize, info.AssetDigest) {
		needDownload = false
		if a.logger != nil {
			a.logger.Info("检测到已存在完整的更新包 (%s)，SHA-256 校验通过，直接复用", info.LatestVersion)
		}
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
				Phase:       "downloading",
				Percent:     100,
				Bytes:       info.AssetSize,
				Total:       info.AssetSize,
				SpeedString: "本地缓存",
				Message:     "已检测到本地完整安装包，校验通过，直接复用安装...",
			})
		}
	}

	if needDownload {
		_ = os.RemoveAll(stageDir)
		_ = os.MkdirAll(stageDir, 0700)
		if a.logger != nil {
			a.logger.Info("开始下载软件更新包：%s", info.DownloadURL)
		}

		onProgress := func(downloaded, total int64, speed string, percent float64) {
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
					Phase:       "downloading",
					Percent:     percent,
					Bytes:       downloaded,
					Total:       total,
					SpeedString: speed,
					Message:     fmt.Sprintf("正在下载：%.1f%% (%s)", percent, speed),
				})
			}
		}

		if err := downloadUpdateAsset(info.DownloadURL, zipPath, proxyPort, info.AssetSize, info.AssetDigest, onProgress); err != nil {
			return nil, fmt.Errorf("下载更新包失败：%w", err)
		}
		_ = os.WriteFile(targetVersionPath, []byte(info.LatestVersion), 0600)
	}

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
			Phase:   "extracting",
			Percent: 100,
			Message: "更新包就绪，正在解压更新文件...",
		})
	}
	if a.logger != nil {
		a.logger.Info("更新包已就绪，正在解压...")
	}

	_ = os.RemoveAll(stageDir)
	_ = os.MkdirAll(stageDir, 0700)
	if err := extractZipArchive(zipPath, stageDir); err != nil {
		return nil, fmt.Errorf("解压更新包失败：%w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		exePath = filepath.Join(a.root, "SanTian-Singbox.exe")
	}
	exeName := filepath.Base(exePath)
	extractedAppDir := locateExtractedAppDir(stageDir, exeName)

	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "app-update-progress", AppUpdateProgress{
			Phase:   "applying",
			Percent: 100,
			Message: "准备就绪，正在进行原子级热替换并重启...",
		})
	}
	if a.logger != nil {
		a.logger.Info("开始执行原子级更新文件替换...")
	}

	// 1. 停止运行中工具与 sing-box 核心以释放 bin/ 目录下的二进制文件句柄
	a.stopRunningToolsForExit()
	if a.manager != nil && a.manager.Running() {
		_ = a.Stop()
		time.Sleep(300 * time.Millisecond)
	}

	// 2. 进程内原子级替换文件（重命名旧程序，拷贝新程序）
	if err := performAtomicInPlaceUpdate(a.root, extractedAppDir, exePath); err != nil {
		return nil, fmt.Errorf("原子更新失败：%w", err)
	}

	// 3. 释放单实例互斥锁并通过 --wait-pid 拉起新版本程序，确保普通权限下热更新也能正常重启
	releaseSingleInstanceLock()
	if err := launchNewAppProcess(exePath, a.root); err != nil {
		return nil, fmt.Errorf("启动新版本程序失败：%w", err)
	}

	if a.logger != nil {
		a.logger.Info("新版本程序已成功拉起，旧进程即将安全退出")
	}

	// 4. 旧进程安全退出
	go func() {
		time.Sleep(200 * time.Millisecond)
		if a.ctx != nil {
			wailsRuntime.Quit(a.ctx)
		}
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()

	return info, nil
}
