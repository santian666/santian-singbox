package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const backupFileVersion = 2

const maxBackupFileBytes = 128 * 1024 * 1024

func (a *App) ExportBackup() (path string, err error) {
	defer a.recoverError("导出备份", &err)
	if a.ctx == nil {
		return "", errors.New("窗口未就绪")
	}
	if a.store == nil {
		return "", errors.New("配置存储未初始化")
	}
	state := a.store.Snapshot()
	backup := NodeBackupFile{
		App:        "SanTian-Singbox",
		Version:    backupFileVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
		Nodes:      state.Nodes,
		Groups:     state.Groups,
		SelectedID: state.SelectedID,
		ExternalProxyEndpoints: append([]ExternalProxyEndpoint(nil),
			state.Settings.ExternalProxyEndpoints...),
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", err
	}
	defaultName := fmt.Sprintf("SanTian-Singbox-配置备份-%s.santian-backup.json", time.Now().Format("20060102-150405"))
	path, err = wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "导出配置备份",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "SanTian 备份 (*.santian-backup.json)", Pattern: "*.santian-backup.json"},
			{DisplayName: "JSON 文件 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		path += ".santian-backup.json"
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	a.logger.Info("已导出配置备份：%s", path)
	return path, nil
}

func (a *App) ImportBackup() (result map[string]interface{}, err error) {
	defer a.recoverError("导入备份", &err)
	if a.ctx == nil {
		return nil, errors.New("窗口未就绪")
	}
	if a.store == nil {
		return nil, errors.New("配置存储未初始化")
	}
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "导入配置备份",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "SanTian 备份 (*.santian-backup.json)", Pattern: "*.santian-backup.json"},
			{DisplayName: "JSON 文件 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return map[string]interface{}{"cancelled": true}, nil
	}
	if info, statErr := os.Stat(path); statErr != nil {
		return nil, statErr
	} else if info.Size() > maxBackupFileBytes {
		return nil, errors.New("备份文件超过 128 MB 限制")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	backup, err := parseNodeBackup(data)
	if err != nil {
		return nil, err
	}
	previous := a.store.Snapshot()
	if err := a.store.ReplaceGroupsAndNodes(backup.Groups, backup.Nodes, backup.SelectedID); err != nil {
		return nil, err
	}
	settings, restoreExternalProxyEndpoints := settingsWithBackupExternalProxyEndpoints(a.store.Snapshot().Settings, backup)
	if restoreExternalProxyEndpoints {
		if err := a.store.UpdateSettings(settings); err != nil {
			return nil, err
		}
	}
	state := a.store.Snapshot()
	if err := a.syncRuntimeNodeList(previous, state, "导入备份后同步运行配置"); err != nil {
		return nil, err
	}
	a.emitState()
	a.logger.Info("已导入配置备份：%s，分组 %d 个，节点 %d 个，外部代理入口 %d 个", path, len(state.Groups), len(state.Nodes), len(state.Settings.ExternalProxyEndpoints))
	return map[string]interface{}{
		"groups":                 len(state.Groups),
		"nodes":                  len(state.Nodes),
		"externalProxyEndpoints": len(state.Settings.ExternalProxyEndpoints),
		"path":                   path,
	}, nil
}

func parseNodeBackup(data []byte) (NodeBackupFile, error) {
	var backup NodeBackupFile
	if err := json.Unmarshal(data, &backup); err != nil {
		return NodeBackupFile{}, errors.New("备份文件格式错误")
	}
	if backup.App != "" && backup.App != "SanTian-Singbox" {
		return NodeBackupFile{}, errors.New("不是 SanTian-Singbox 备份文件")
	}
	if backup.Version > backupFileVersion {
		return NodeBackupFile{}, errors.New("备份文件版本高于当前软件版本，请升级软件后再导入")
	}
	if len(backup.Groups) == 0 {
		return NodeBackupFile{}, errors.New("备份文件没有节点分组")
	}
	if len(backup.Nodes) == 0 {
		return NodeBackupFile{}, errors.New("备份文件没有节点")
	}
	if err := validateExternalProxyEndpoints(backup.ExternalProxyEndpoints); err != nil {
		return NodeBackupFile{}, fmt.Errorf("备份文件的外部代理入口无效：%w", err)
	}
	return backup, nil
}

func settingsWithBackupExternalProxyEndpoints(settings Settings, backup NodeBackupFile) (Settings, bool) {
	if backup.Version < 2 {
		return settings, false
	}
	settings.ExternalProxyEndpoints = resolveExternalProxyEndpoints(
		normalizeExternalProxyEndpoints(backup.ExternalProxyEndpoints),
	)
	return settings, true
}
