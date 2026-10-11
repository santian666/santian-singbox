package main

import "time"

type Node struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Address       string                 `json:"address"`
	Port          int                    `json:"port"`
	Country       string                 `json:"country"`
	GroupID       string                 `json:"groupId"`
	DelayMS       int                    `json:"delayMs"`
	LastTest      string                 `json:"lastTest"`
	Raw           string                 `json:"raw"`
	Outbound      map[string]interface{} `json:"outbound"`
	UploadRate    int64                  `json:"uploadRate"`
	DownloadRate  int64                  `json:"downloadRate"`
	UploadTotal   int64                  `json:"uploadTotal"`
	DownloadTotal int64                  `json:"downloadTotal"`
}

type NodeGroup struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        string `json:"source"`
	URL           string `json:"url"`
	UploadRate    int64  `json:"uploadRate"`
	DownloadRate  int64  `json:"downloadRate"`
	UploadTotal   int64  `json:"uploadTotal"`
	DownloadTotal int64  `json:"downloadTotal"`
}

type Settings struct {
	Mode                   string                  `json:"mode"`
	SystemProxy            bool                    `json:"systemProxy"`
	Tun                    bool                    `json:"tun"`
	MixedPort              int                     `json:"mixedPort"`
	ProxyMap               bool                    `json:"proxyMap"`
	ProxyMapStartPort      int                     `json:"proxyMapStartPort"`
	ProxyMapGroupID        string                  `json:"proxyMapGroupId"`
	ProxyMapGroupIDs       []string                `json:"proxyMapGroupIds"`
	ExternalProxyEndpoints []ExternalProxyEndpoint `json:"externalProxyEndpoints"`
	AppProxy               bool                    `json:"appProxy"`
	AppProxyTargets        []AppProxyTarget        `json:"appProxyTargets"`
	DirectDomains          string                  `json:"directDomains"`
	CoreLogLevel           string                  `json:"coreLogLevel"`
	RuleSource             string                  `json:"ruleSource"`
	AutoUpdateRules        bool                    `json:"autoUpdateRules"`
	RuleUpdateInterval     string                  `json:"ruleUpdateInterval"`
	FakeDNS                bool                    `json:"fakeDns"`
	AutoStart              bool                    `json:"autoStart"`
	CircuitBreaker         bool                    `json:"circuitBreaker"`
	AutoFailover           bool                    `json:"autoFailover"`
	Theme                  string                  `json:"theme"`
}

type ExternalProxyEndpoint struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Enabled     bool     `json:"enabled"`
	ResolvedIPs []string `json:"resolvedIps,omitempty"`
}

type ExternalProxyTestResult struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Auth      string `json:"auth"`
	LatencyMS int    `json:"latencyMs"`
	Message   string `json:"message"`
}

type AppProxyTarget struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

type PersistState struct {
	Nodes      []Node      `json:"nodes"`
	Groups     []NodeGroup `json:"groups"`
	SelectedID string      `json:"selectedId"`
	Settings   Settings    `json:"settings"`
}

type NodeBackupFile struct {
	App                    string                  `json:"app"`
	Version                int                     `json:"version"`
	ExportedAt             string                  `json:"exportedAt"`
	Nodes                  []Node                  `json:"nodes"`
	Groups                 []NodeGroup             `json:"groups"`
	SelectedID             string                  `json:"selectedId"`
	ExternalProxyEndpoints []ExternalProxyEndpoint `json:"externalProxyEndpoints"`
}

type RuntimeState struct {
	Running             bool         `json:"running"`
	AppVersion          string       `json:"appVersion"`
	AppLatestVersion    string       `json:"appLatestVersion,omitempty"`
	AppUpdateAvailable  bool         `json:"appUpdateAvailable,omitempty"`
	AppReleaseNotes     string       `json:"appReleaseNotes,omitempty"`
	CoreVersion         string       `json:"coreVersion"`
	CoreLatestVersion   string       `json:"coreLatestVersion"`
	CoreUpdateAvailable bool         `json:"coreUpdateAvailable"`
	ListenURL           string       `json:"listenUrl"`
	SelectedID          string       `json:"selectedId"`
	UploadRate          int64        `json:"uploadRate"`
	DownloadRate        int64        `json:"downloadRate"`
	Settings            Settings     `json:"settings"`
	RulePresets         []RulePreset `json:"rulePresets"`
	Nodes               []Node       `json:"nodes"`
	Groups              []NodeGroup  `json:"groups"`
	Logs                []LogMessage `json:"logs"`
	ConfigMode          ConfigMode   `json:"configMode"`
	TunActive            bool         `json:"tunActive"`
	TunStatus            string       `json:"tunStatus"`
	CircuitBreakerActive bool         `json:"circuitBreakerActive"`
	CircuitBreakerReason string       `json:"circuitBreakerReason,omitempty"`
}

type RulePreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RuleUpdateResult struct {
	Count   int    `json:"count"`
	Latest  bool   `json:"latest"`
	Version string `json:"version"`
}

type LogMessage struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

type SpeedResult struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Address      string  `json:"address"`
	Port         int     `json:"port"`
	Country      string  `json:"country"`
	PingAddress  string  `json:"pingAddress"`
	PingMS       int     `json:"pingMs"`
	URLDelayMS   int     `json:"urlDelayMs"`
	OutboundIP   string  `json:"outboundIp"`
	DelayMS      int     `json:"delayMs"`
	DownloadMbps float64 `json:"downloadMbps"`
	UploadMbps   float64 `json:"uploadMbps"`
}

type RawNodeTestResult struct {
	Index  int         `json:"index"`
	Raw    string      `json:"raw"`
	Result SpeedResult `json:"result"`
	Error  string      `json:"error"`
}

type SpeedTestOptions struct {
	TrafficMB      int           `json:"trafficMB"`
	DurationSec    int           `json:"durationSec"`
	UseSystemProxy bool          `json:"useSystemProxy"`
	DirectOnly     bool          `json:"directOnly"`
	FastDelay      bool          `json:"fastDelay"`
	CustomTimeout  time.Duration `json:"customTimeout,omitempty"`
	PreferCore     bool          `json:"-"`
}

type ToolStatus struct {
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	ErrorText string `json:"errorText"`
}

type IperfStartOptions struct {
	Target      string `json:"target"`
	Parallel    int    `json:"parallel"`
	Mode        string `json:"mode"`
	NetworkMode string `json:"networkMode,omitempty"`
}

type IperfStartResult struct {
	Command string `json:"command"`
}

type MTRStartResult struct {
	Command string `json:"command"`
}

type MTRStartOptions struct {
	Target      string `json:"target"`
	NetworkMode string `json:"networkMode"`
	Protocol    string `json:"protocol,omitempty"`
	Port        int    `json:"port,omitempty"`
}

type LocalSpeedtestStartOptions struct {
	ServerID    string `json:"serverId"`
	NetworkMode string `json:"networkMode"`
}

type LocalSpeedtestServerListOptions struct {
	NetworkMode string `json:"networkMode"`
}

type LocalSpeedtestStartResult struct {
	Command string `json:"command"`
}

type LocalSpeedtestServer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Country  string `json:"country"`
}

type NetworkToolResult struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
	Success  bool   `json:"success"`
}

type SpeedProgress struct {
	ID           string  `json:"id"`
	Raw          string  `json:"raw"`
	Name         string  `json:"name"`
	Phase        string  `json:"phase"`
	Status       string  `json:"status"`
	DownloadMbps float64 `json:"downloadMbps"`
	UploadMbps   float64 `json:"uploadMbps"`
}

type ConfigMode struct {
	Mode       string   `json:"mode"`
	Final      string   `json:"final"`
	RuleCount  int      `json:"ruleCount"`
	RuleSets   []string `json:"ruleSets"`
	HasDirect  bool     `json:"hasDirect"`
	DirectHits []string `json:"directHits"`
}
