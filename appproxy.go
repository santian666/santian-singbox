package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type processInfo struct {
	PID       int
	ParentPID int
	Name      string
	Path      string
}

func enabledAppProxyTargets(targets []AppProxyTarget) []AppProxyTarget {
	result := make([]AppProxyTarget, 0, len(targets))
	for _, target := range normalizeAppProxyTargets(targets) {
		if target.Enabled {
			result = append(result, target)
		}
	}
	return result
}

func normalizeAppProxyTargets(targets []AppProxyTarget) []AppProxyTarget {
	result := make([]AppProxyTarget, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		target.Type = strings.TrimSpace(strings.ToLower(target.Type))
		if target.Type != "app" && target.Type != "folder" {
			continue
		}
		target.Path = normalizeAppProxyPath(target.Path)
		if target.Path == "" {
			continue
		}
		if target.Type == "app" && !strings.EqualFold(filepath.Ext(target.Path), ".exe") {
			continue
		}
		key := target.Type + "|" + strings.ToLower(target.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if target.Name = strings.TrimSpace(target.Name); target.Name == "" {
			target.Name = appProxyTargetName(target.Type, target.Path)
		}
		target.ID = appProxyTargetID(target.Type, target.Path)
		result = append(result, target)
	}
	return result
}

func normalizeAppProxyPath(path string) string {
	path = strings.TrimSpace(strings.Trim(path, `"`))
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func appProxyTargetName(targetType string, path string) string {
	if targetType == "folder" {
		return filepath.Base(path)
	}
	return filepath.Base(path)
}

func appProxyTargetID(targetType string, path string) string {
	return "app-proxy-" + strings.TrimPrefix(tagFromRaw(targetType+"|"+strings.ToLower(normalizeAppProxyPath(path))), "node-")
}

func validateAppProxyTarget(target AppProxyTarget) (AppProxyTarget, error) {
	target.Type = strings.TrimSpace(strings.ToLower(target.Type))
	target.Path = normalizeAppProxyPath(target.Path)
	if target.Type != "app" && target.Type != "folder" {
		return AppProxyTarget{}, errors.New("应用代理规则类型无效")
	}
	if target.Path == "" {
		return AppProxyTarget{}, errors.New("路径不能为空")
	}
	info, err := os.Stat(target.Path)
	if err != nil {
		return AppProxyTarget{}, err
	}
	if target.Type == "app" {
		if info.IsDir() {
			return AppProxyTarget{}, errors.New("请选择 exe 应用文件")
		}
		if !strings.EqualFold(filepath.Ext(target.Path), ".exe") {
			return AppProxyTarget{}, errors.New("只能选择 exe 应用文件")
		}
	}
	if target.Type == "folder" && !info.IsDir() {
		return AppProxyTarget{}, errors.New("请选择文件夹")
	}
	target.Enabled = true
	target.ID = appProxyTargetID(target.Type, target.Path)
	target.Name = appProxyTargetName(target.Type, target.Path)
	return target, nil
}

func sortStrings(items []string) []string {
	for i := 1; i < len(items); i++ {
		value := items[i]
		j := i - 1
		for j >= 0 && strings.ToLower(items[j]) > strings.ToLower(value) {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = value
	}
	return items
}
