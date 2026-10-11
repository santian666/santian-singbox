package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureRuleSetsUsesLocalCacheOnly(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.db")
	ruleDir := ruleSetDirFromCachePath(cachePath)
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatalf("创建规则缓存目录失败：%v", err)
	}
	for _, def := range rulePresetDefs[ruleSourceLoyalsoldier] {
		target := filepath.Join(ruleDir, def.FileName)
		if err := os.WriteFile(target, []byte(`{"version":1,"rules":[]}`), 0644); err != nil {
			t.Fatalf("写入规则缓存失败：%v", err)
		}
		oldTime := time.Now().Add(-30 * 24 * time.Hour)
		if err := os.Chtimes(target, oldTime, oldTime); err != nil {
			t.Fatalf("修改规则缓存时间失败：%v", err)
		}
	}
	manager := NewSingBoxManager(t.TempDir(), "sing-box.exe", "sing-box.json", cachePath, nil)
	settings := defaultSettings()
	settings.AutoUpdateRules = false
	settings.RuleUpdateInterval = "6h"
	if err := manager.ensureRuleSets(settings); err != nil {
		t.Fatalf("代理启动应直接使用本地规则缓存，不应联网更新：%v", err)
	}
}

func TestEnsureRuleSetsReportsMissingCache(t *testing.T) {
	manager := NewSingBoxManager(t.TempDir(), "sing-box.exe", "sing-box.json", filepath.Join(t.TempDir(), "cache.db"), nil)
	err := manager.ensureRuleSets(defaultSettings())
	if err == nil {
		t.Fatal("规则缓存缺失时应提示先更新规则")
	}
}

func TestUpdateRuleSetsIfNeededReturnsLatestForLocalCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.db")
	ruleDir := ruleSetDirFromCachePath(cachePath)
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatalf("创建规则缓存目录失败：%v", err)
	}
	for _, def := range rulePresetDefs[ruleSourceLoyalsoldier] {
		target := filepath.Join(ruleDir, def.FileName)
		if err := os.WriteFile(target, []byte(`{"version":1,"rules":[]}`), 0644); err != nil {
			t.Fatalf("写入规则缓存失败：%v", err)
		}
	}
	manager := NewSingBoxManager(t.TempDir(), "sing-box.exe", "sing-box.json", cachePath, nil)
	manager.ruleVersionChecker = func(source string) (string, error) {
		if source != ruleSourceLoyalsoldier {
			t.Fatalf("规则源错误：%s", source)
		}
		return loyalsoldierRuleVersion, nil
	}
	result, err := manager.UpdateRuleSetsIfNeeded(defaultSettings())
	if err != nil {
		t.Fatalf("本地规则齐全时不应联网更新：%v", err)
	}
	if !result.Latest || result.Count != 0 || result.Version != loyalsoldierRuleVersion {
		t.Fatalf("应返回已是最新版本，实际：%+v", result)
	}
	if got := manager.readRuleSetVersion(defaultSettings()); got != loyalsoldierRuleVersion {
		t.Fatalf("应写入规则版本标记，实际：%s", got)
	}
}

func TestCommitStagedRuleSetsReplacesAllFiles(t *testing.T) {
	dir := t.TempDir()
	files := make([]stagedRuleSetFile, 0, 3)
	for index := 0; index < 3; index++ {
		target := filepath.Join(dir, "rule-"+string(rune('a'+index))+".json")
		if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		temp, err := writeRuleSetStage(dir, filepath.Base(target), []byte("new"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, stagedRuleSetFile{targetPath: target, tempPath: temp})
	}
	if err := commitStagedRuleSets(files); err != nil {
		t.Fatal(err)
	}
	for _, item := range files {
		data, err := os.ReadFile(item.targetPath)
		if err != nil || string(data) != "new" {
			t.Fatalf("规则文件未完整提交：%s %q %v", item.targetPath, data, err)
		}
		if _, err := os.Stat(item.targetPath + ".update.bak"); !os.IsNotExist(err) {
			t.Fatalf("提交成功后残留备份：%v", err)
		}
	}
}

func TestCommitStagedRuleSetsRollsBackPartialCommit(t *testing.T) {
	dir := t.TempDir()
	firstTarget := filepath.Join(dir, "first.json")
	secondTarget := filepath.Join(dir, "second.json")
	for _, target := range []string{firstTarget, secondTarget} {
		if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	firstTemp, err := writeRuleSetStage(dir, "first.json", []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	files := []stagedRuleSetFile{
		{targetPath: firstTarget, tempPath: firstTemp},
		{targetPath: secondTarget, tempPath: filepath.Join(dir, "missing-stage.json")},
	}
	if err := commitStagedRuleSets(files); err == nil || !strings.Contains(err.Error(), "旧规则已恢复") {
		t.Fatalf("部分提交失败未返回回滚结果：%v", err)
	}
	for _, target := range []string{firstTarget, secondTarget} {
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "old" {
			t.Fatalf("部分提交失败后旧规则未恢复：%s %q %v", target, data, err)
		}
	}
}

func TestRuleUpdateTargetVersionChecksLatestOnlyOnManualUpdate(t *testing.T) {
	manager := NewSingBoxManager(t.TempDir(), "sing-box.exe", "sing-box.json", filepath.Join(t.TempDir(), "cache.db"), nil)
	called := 0
	manager.ruleVersionChecker = func(source string) (string, error) {
		called++
		if source != ruleSourceLoyalsoldier {
			t.Fatalf("规则源错误：%s", source)
		}
		return "v202606050001", nil
	}

	got, err := manager.ruleUpdateTargetVersion(defaultSettings())
	if err != nil {
		t.Fatalf("手动检查规则版本失败：%v", err)
	}
	if got != "202606050001" {
		t.Fatalf("手动更新应使用已检测到的新版本，实际：%s", got)
	}
	if called != 1 {
		t.Fatalf("手动更新应检查一次规则源版本，实际：%d", called)
	}
}

func TestNormalizeRuleVersionTag(t *testing.T) {
	cases := map[string]string{
		"v202606042304":        "202606042304",
		"tag/202606042304":     "202606042304",
		" 202606042304 \r\n":   "202606042304",
		"202606042304 release": "202606042304",
	}
	for input, want := range cases {
		if got := normalizeRuleVersionTag(input); got != want {
			t.Fatalf("normalizeRuleVersionTag(%q)=%q，期望 %q", input, got, want)
		}
	}
}
