package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultGroupID = "default"

func normalizeGroupID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return defaultGroupID
	}
	return id
}

type Store struct {
	mu            sync.RWMutex
	path          string
	selectionPath string
	state         PersistState
	trafficDirty  bool
}

func NewStore(path string) (*Store, error) {
	store := &Store{path: path, selectionPath: filepath.Join(filepath.Dir(path), "selected.dat")}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func defaultSettings() Settings {
	return Settings{
		Mode:               "global",
		MixedPort:          20808,
		ProxyMapStartPort:  52001,
		RuleSource:         ruleSourceLoyalsoldier,
		AutoUpdateRules:    false,
		RuleUpdateInterval: "168h",
		AutoStart:          false,
		CircuitBreaker:     true,
		AutoFailover:       false,
	}
}

func defaultGroups() []NodeGroup {
	return []NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}}
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = PersistState{Settings: defaultSettings(), Groups: defaultGroups()}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s.saveLocked()
	}
	if err != nil {
		return err
	}
	data, err = decryptBytes(data)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return s.saveLocked()
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return err
	}
	if s.state.Settings.Mode != "rule" && s.state.Settings.Mode != "global" {
		s.state.Settings.Mode = "global"
	}
	if s.state.Settings.MixedPort == 0 {
		s.state.Settings.MixedPort = 20808
	}
	s.state.Settings.AutoStart = false
	normalizeSettings(&s.state.Settings)
	s.normalizeLocked()
	s.loadSelectionLocked()
	return nil
}

func (s *Store) loadSelectionLocked() {
	data, err := os.ReadFile(s.selectionPath)
	if err != nil {
		return
	}
	data, err = decryptBytes(data)
	if err != nil {
		return
	}
	selectedID := strings.TrimSpace(string(data))
	if selectedID == "" {
		s.state.SelectedID = ""
		return
	}
	for _, node := range s.state.Nodes {
		if node.ID == selectedID {
			s.state.SelectedID = selectedID
			return
		}
	}
}

func (s *Store) Snapshot() PersistState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *Store) TrafficTarget() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	selectedID := s.state.SelectedID
	if selectedID == "" {
		return "", ""
	}
	for _, node := range s.state.Nodes {
		if node.ID != selectedID {
			continue
		}
		groupID := node.GroupID
		if groupID == "" {
			groupID = defaultGroupID
		}
		return selectedID, groupID
	}
	return "", ""
}

func (s *Store) ReplaceNodes(nodes []Node, selectedID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Nodes = nodes
	s.state.SelectedID = selectedID
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) ReplaceGroupsAndNodes(groups []NodeGroup, nodes []Node, selectedID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Groups = groups
	s.state.Nodes = nodes
	s.state.SelectedID = selectedID
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) UpdateSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	theme := s.state.Settings.Theme
	if settings.Mode == "" {
		settings.Mode = s.state.Settings.Mode
	}
	if settings.MixedPort == 0 {
		settings.MixedPort = s.state.Settings.MixedPort
	}
	if strings.TrimSpace(settings.DirectDomains) == "" && settings.Mode != "rule" && strings.TrimSpace(s.state.Settings.DirectDomains) != "" {
		settings.DirectDomains = s.state.Settings.DirectDomains
	}
	normalizeSettings(&settings)
	settings.Theme = theme
	s.state.Settings = settings
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) UpdateTheme(theme string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if theme != "dark" {
		theme = "light"
	}
	s.state.Settings.Theme = theme
	return s.saveLocked()
}

func (s *Store) SelectNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, node := range s.state.Nodes {
		if node.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("节点不存在")
	}
	if s.state.SelectedID == id {
		return nil
	}
	previousID := s.state.SelectedID
	s.state.SelectedID = id
	if err := s.saveSelectionLocked(); err != nil {
		s.state.SelectedID = previousID
		return err
	}
	return nil
}

func (s *Store) RestoreSelectedNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id != "" {
		found := false
		for _, node := range s.state.Nodes {
			if node.ID == id {
				found = true
				break
			}
		}
		if !found {
			return errors.New("节点不存在")
		}
	}
	previousID := s.state.SelectedID
	s.state.SelectedID = id
	if err := s.saveSelectionLocked(); err != nil {
		s.state.SelectedID = previousID
		return err
	}
	return nil
}

func (s *Store) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes := make([]Node, 0, len(s.state.Nodes))
	for _, node := range s.state.Nodes {
		if node.ID != id {
			nodes = append(nodes, node)
		}
	}
	s.state.Nodes = nodes
	if s.state.SelectedID == id {
		s.state.SelectedID = ""
	}
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) DeleteNodes(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) == 0 {
		return errors.New("节点 ID 无效")
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return errors.New("节点 ID 无效")
	}
	nodes := make([]Node, 0, len(s.state.Nodes))
	for _, node := range s.state.Nodes {
		if _, ok := idSet[node.ID]; ok {
			continue
		}
		nodes = append(nodes, node)
	}
	s.state.Nodes = nodes
	if _, ok := idSet[s.state.SelectedID]; ok {
		s.state.SelectedID = ""
	}
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) MoveNodesToGroup(ids []string, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		groupID = defaultGroupID
	}
	groupExists := false
	for _, group := range s.state.Groups {
		if group.ID == groupID {
			groupExists = true
			break
		}
	}
	if !groupExists {
		return errors.New("目标分组不存在")
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return errors.New("节点 ID 无效")
	}
	moved := 0
	for i := range s.state.Nodes {
		if _, ok := idSet[s.state.Nodes[i].ID]; !ok {
			continue
		}
		s.state.Nodes[i].GroupID = groupID
		moved++
	}
	if moved == 0 {
		return errors.New("节点不存在")
	}
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == defaultGroupID {
		return errors.New("默认组不能删除")
	}
	groups := make([]NodeGroup, 0, len(s.state.Groups))
	for _, group := range s.state.Groups {
		if group.ID != id {
			groups = append(groups, group)
		}
	}
	nodes := make([]Node, 0, len(s.state.Nodes))
	for _, node := range s.state.Nodes {
		if node.GroupID != id {
			nodes = append(nodes, node)
		}
	}
	s.state.Groups = groups
	s.state.Nodes = nodes
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) RenameGroup(id string, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		return errors.New("group does not exist")
	}
	if name == "" {
		return errors.New("group name is required")
	}
	for _, group := range s.state.Groups {
		if group.ID != id && group.Name == name {
			return errors.New("group name already exists")
		}
	}
	for i := range s.state.Groups {
		if s.state.Groups[i].ID == id {
			if id == defaultGroupID {
				s.state.Groups[i].Name = "默认"
			} else {
				s.state.Groups[i].Name = name
			}
			s.normalizeLocked()
			return s.saveLocked()
		}
	}
	return errors.New("group does not exist")
}

func (s *Store) ReorderGroups(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[string]NodeGroup, len(s.state.Groups))
	seen := make(map[string]bool, len(s.state.Groups))
	for _, group := range s.state.Groups {
		byID[group.ID] = group
	}
	ordered := make([]NodeGroup, 0, len(s.state.Groups))
	if group, ok := byID[defaultGroupID]; ok {
		ordered = append(ordered, group)
		seen[defaultGroupID] = true
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == defaultGroupID || seen[id] {
			continue
		}
		group, ok := byID[id]
		if !ok {
			return errors.New("节点组不存在")
		}
		ordered = append(ordered, group)
		seen[id] = true
	}
	for _, group := range s.state.Groups {
		if seen[group.ID] {
			continue
		}
		ordered = append(ordered, group)
		seen[group.ID] = true
	}
	s.state.Groups = ordered
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) ReorderNodes(groupID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		groupID = defaultGroupID
	}
	groupExists := false
	for _, group := range s.state.Groups {
		if group.ID == groupID {
			groupExists = true
			break
		}
	}
	if !groupExists {
		return errors.New("节点组不存在")
	}
	byID := make(map[string]Node, len(s.state.Nodes))
	groupNodeIDs := make(map[string]bool, len(s.state.Nodes))
	for _, node := range s.state.Nodes {
		byID[node.ID] = node
		if node.GroupID == groupID || (node.GroupID == "" && groupID == defaultGroupID) {
			groupNodeIDs[node.ID] = true
		}
	}
	seen := make(map[string]bool, len(ids))
	orderedGroupNodes := make([]Node, 0, len(groupNodeIDs))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		node, ok := byID[id]
		if !ok {
			return errors.New("节点不存在")
		}
		if !groupNodeIDs[id] {
			return errors.New("节点不属于当前分组")
		}
		orderedGroupNodes = append(orderedGroupNodes, node)
		seen[id] = true
	}
	for _, node := range s.state.Nodes {
		if !groupNodeIDs[node.ID] || seen[node.ID] {
			continue
		}
		orderedGroupNodes = append(orderedGroupNodes, node)
		seen[node.ID] = true
	}
	next := append([]Node{}, s.state.Nodes...)
	groupIndex := 0
	for i := range next {
		if next[i].GroupID != groupID && !(next[i].GroupID == "" && groupID == defaultGroupID) {
			continue
		}
		next[i] = orderedGroupNodes[groupIndex]
		groupIndex++
	}
	s.state.Nodes = next
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Store) UpdateNodeDelay(id string, delayMS int, lastTest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Nodes {
		if s.state.Nodes[i].ID == id {
			s.state.Nodes[i].DelayMS = delayMS
			s.state.Nodes[i].LastTest = lastTest
			return s.saveLocked()
		}
	}
	return errors.New("节点不存在")
}

type NodeDelayUpdate struct {
	DelayMS  int
	LastTest string
}

func (s *Store) UpdateNodeDelays(updates map[string]NodeDelayUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.state.Nodes {
		update, ok := updates[s.state.Nodes[i].ID]
		if !ok {
			continue
		}
		s.state.Nodes[i].DelayMS = update.DelayMS
		s.state.Nodes[i].LastTest = update.LastTest
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) AddTraffic(nodeID string, groupID string, uploadDelta int64, downloadDelta int64) error {
	if uploadDelta <= 0 && downloadDelta <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if groupID == "" {
		groupID = defaultGroupID
	}
	for i := range s.state.Nodes {
		if s.state.Nodes[i].ID == nodeID {
			s.state.Nodes[i].UploadTotal += uploadDelta
			s.state.Nodes[i].DownloadTotal += downloadDelta
			break
		}
	}
	for i := range s.state.Groups {
		if s.state.Groups[i].ID == groupID {
			s.state.Groups[i].UploadTotal += uploadDelta
			s.state.Groups[i].DownloadTotal += downloadDelta
			break
		}
	}
	s.trafficDirty = true
	return nil
}

func (s *Store) FlushTraffic() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.trafficDirty {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	encrypted, err := encryptBytes(data)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, encrypted, 0644); err != nil {
		return err
	}
	if err := s.saveSelectionLocked(); err != nil {
		return err
	}
	s.trafficDirty = false
	return nil
}

func (s *Store) saveSelectionLocked() error {
	encrypted, err := encryptBytes([]byte(s.state.SelectedID))
	if err != nil {
		return err
	}
	return writeFileAtomic(s.selectionPath, encrypted, 0644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		_ = os.Chmod(dir, 0777)
	}

	// 1. 若目标文件已存在，先清除可能的只读属性，防止因只读报错 Access is denied
	if fi, err := os.Stat(path); err == nil && fi.Mode()&0200 == 0 {
		_ = os.Chmod(path, 0666)
	}

	// 2. 优先尝试无 "." 前缀的临时文件原子写入（避免被杀毒软件/360防勒索拦截）
	prefix := filepath.Base(path) + "-tmp-*"
	tmp, err := os.CreateTemp(dir, prefix)
	if err == nil {
		tmpPath := tmp.Name()
		cleanup := true
		defer func() {
			if cleanup {
				_ = os.Remove(tmpPath)
			}
		}()

		if _, err := tmp.Write(data); err == nil {
			_ = tmp.Sync()
			if err := tmp.Close(); err == nil {
				_ = os.Chmod(tmpPath, perm)
				if err := replaceFileWithRetry(tmpPath, path, 3); err == nil {
					cleanup = false
					return nil
				}
			}
		} else {
			_ = tmp.Close()
		}
	}

	// 3. 降级方案：若创建临时文件被系统/杀毒软件拦截（Access is denied）或重命名失败，直接覆写目标文件
	_ = os.Chmod(path, 0666)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		_ = os.Chmod(path, 0666)
		return os.WriteFile(path, data, perm)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func replaceFileWithRetry(src, dst string, retries int) error {
	var lastErr error
	for i := 0; i < retries; i++ {
		if fi, err := os.Stat(dst); err == nil && fi.Mode()&0200 == 0 {
			_ = os.Chmod(dst, 0666)
		}
		if err := os.Rename(src, dst); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
		}
	}
	return lastErr
}


func cloneState(state PersistState) PersistState {
	nodes := make([]Node, len(state.Nodes))
	copy(nodes, state.Nodes)
	groups := make([]NodeGroup, len(state.Groups))
	copy(groups, state.Groups)
	appProxyTargets := make([]AppProxyTarget, len(state.Settings.AppProxyTargets))
	copy(appProxyTargets, state.Settings.AppProxyTargets)
	settings := state.Settings
	settings.AppProxyTargets = appProxyTargets
	return PersistState{
		Nodes:      nodes,
		Groups:     groups,
		SelectedID: state.SelectedID,
		Settings:   settings,
	}
}

func (s *Store) normalizeLocked() {
	if len(s.state.Groups) == 0 {
		s.state.Groups = defaultGroups()
	}
	groups := make([]NodeGroup, 0, len(s.state.Groups)+1)
	groupIDs := map[string]bool{}
	hasDefault := false
	for _, group := range s.state.Groups {
		if group.ID == "" {
			continue
		}
		if group.Name == "" {
			group.Name = "未命名组"
		}
		if group.Source == "" {
			group.Source = "manual"
		}
		if groupIDs[group.ID] {
			continue
		}
		if group.ID == defaultGroupID {
			group.Name = "默认"
			group.Source = "manual"
			hasDefault = true
		}
		groupIDs[group.ID] = true
		groups = append(groups, group)
	}
	if !hasDefault {
		groups = append([]NodeGroup{{ID: defaultGroupID, Name: "默认", Source: "manual"}}, groups...)
		groupIDs[defaultGroupID] = true
	}
	s.state.Groups = groups
	proxyMapGroupIDs := make([]string, 0, len(s.state.Settings.ProxyMapGroupIDs))
	for _, groupID := range s.state.Settings.ProxyMapGroupIDs {
		if groupIDs[groupID] {
			proxyMapGroupIDs = append(proxyMapGroupIDs, groupID)
		}
	}
	s.state.Settings.ProxyMapGroupIDs = proxyMapGroupIDs
	if len(proxyMapGroupIDs) > 0 {
		s.state.Settings.ProxyMapGroupID = proxyMapGroupIDs[0]
	} else if s.state.Settings.ProxyMapGroupID != "" && !groupIDs[s.state.Settings.ProxyMapGroupID] {
		s.state.Settings.ProxyMapGroupID = ""
	}

	selectedExists := false
	for i := range s.state.Nodes {
		if s.state.Nodes[i].GroupID == "" || !groupIDs[s.state.Nodes[i].GroupID] {
			s.state.Nodes[i].GroupID = defaultGroupID
		}
		if s.state.Nodes[i].ID == s.state.SelectedID {
			selectedExists = true
		}
		s.state.Nodes[i].Outbound = sanitizeOutboundForSingBox(s.state.Nodes[i].Outbound)
	}
	if !selectedExists {
		s.state.SelectedID = ""
	}
}

func settingsKeyExists(data []byte, key string) bool {
	var raw struct {
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, ok := raw.Settings[key]
	return ok
}
