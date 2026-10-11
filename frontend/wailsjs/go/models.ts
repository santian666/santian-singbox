export namespace main {
	
	export class AppProxyTarget {
	    id: string;
	    type: string;
	    name: string;
	    path: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppProxyTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.enabled = source["enabled"];
	    }
	}
	export class ConfigMode {
	    mode: string;
	    final: string;
	    ruleCount: number;
	    ruleSets: string[];
	    hasDirect: boolean;
	    directHits: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigMode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.final = source["final"];
	        this.ruleCount = source["ruleCount"];
	        this.ruleSets = source["ruleSets"];
	        this.hasDirect = source["hasDirect"];
	        this.directHits = source["directHits"];
	    }
	}
	export class ExternalProxyEndpoint {
	    host: string;
	    port: number;
	    enabled: boolean;
	    resolvedIps?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExternalProxyEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.enabled = source["enabled"];
	        this.resolvedIps = source["resolvedIps"];
	    }
	}
	export class ExternalProxyTestResult {
	    host: string;
	    port: number;
	    path: string;
	    auth: string;
	    latencyMs: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ExternalProxyTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.path = source["path"];
	        this.auth = source["auth"];
	        this.latencyMs = source["latencyMs"];
	        this.message = source["message"];
	    }
	}
	export class IperfStartOptions {
	    target: string;
	    parallel: number;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new IperfStartOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.parallel = source["parallel"];
	        this.mode = source["mode"];
	    }
	}
	export class IperfStartResult {
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new IperfStartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	    }
	}
	export class LocalSpeedtestServer {
	    id: string;
	    name: string;
	    location: string;
	    country: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalSpeedtestServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.location = source["location"];
	        this.country = source["country"];
	    }
	}
	export class LocalSpeedtestServerListOptions {
	    networkMode: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalSpeedtestServerListOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.networkMode = source["networkMode"];
	    }
	}
	export class LocalSpeedtestStartOptions {
	    serverId: string;
	    networkMode: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalSpeedtestStartOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.networkMode = source["networkMode"];
	    }
	}
	export class LocalSpeedtestStartResult {
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalSpeedtestStartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	    }
	}
	export class LogMessage {
	    time: string;
	    level: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.text = source["text"];
	    }
	}
	export class MTRStartOptions {
	    target: string;
	    networkMode: string;
	
	    static createFrom(source: any = {}) {
	        return new MTRStartOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.networkMode = source["networkMode"];
	    }
	}
	export class MTRStartResult {
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new MTRStartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	    }
	}
	export class NetworkToolResult {
	    name: string;
	    command: string;
	    output: string;
	    exitCode: number;
	    success: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NetworkToolResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.output = source["output"];
	        this.exitCode = source["exitCode"];
	        this.success = source["success"];
	    }
	}
	export class Node {
	    id: string;
	    name: string;
	    type: string;
	    address: string;
	    port: number;
	    country: string;
	    groupId: string;
	    delayMs: number;
	    lastTest: string;
	    raw: string;
	    outbound: Record<string, any>;
	    uploadRate: number;
	    downloadRate: number;
	    uploadTotal: number;
	    downloadTotal: number;
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.country = source["country"];
	        this.groupId = source["groupId"];
	        this.delayMs = source["delayMs"];
	        this.lastTest = source["lastTest"];
	        this.raw = source["raw"];
	        this.outbound = source["outbound"];
	        this.uploadRate = source["uploadRate"];
	        this.downloadRate = source["downloadRate"];
	        this.uploadTotal = source["uploadTotal"];
	        this.downloadTotal = source["downloadTotal"];
	    }
	}
	export class NodeGroup {
	    id: string;
	    name: string;
	    source: string;
	    url: string;
	    uploadRate: number;
	    downloadRate: number;
	    uploadTotal: number;
	    downloadTotal: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.url = source["url"];
	        this.uploadRate = source["uploadRate"];
	        this.downloadRate = source["downloadRate"];
	        this.uploadTotal = source["uploadTotal"];
	        this.downloadTotal = source["downloadTotal"];
	    }
	}
	export class SpeedResult {
	    id: string;
	    name: string;
	    type: string;
	    address: string;
	    port: number;
	    country: string;
	    pingAddress: string;
	    pingMs: number;
	    urlDelayMs: number;
	    outboundIp: string;
	    delayMs: number;
	    downloadMbps: number;
	    uploadMbps: number;
	
	    static createFrom(source: any = {}) {
	        return new SpeedResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.country = source["country"];
	        this.pingAddress = source["pingAddress"];
	        this.pingMs = source["pingMs"];
	        this.urlDelayMs = source["urlDelayMs"];
	        this.outboundIp = source["outboundIp"];
	        this.delayMs = source["delayMs"];
	        this.downloadMbps = source["downloadMbps"];
	        this.uploadMbps = source["uploadMbps"];
	    }
	}
	export class RawNodeTestResult {
	    index: number;
	    raw: string;
	    result: SpeedResult;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new RawNodeTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.raw = source["raw"];
	        this.result = this.convertValues(source["result"], SpeedResult);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RulePreset {
	    id: string;
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new RulePreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class Settings {
	    mode: string;
	    systemProxy: boolean;
	    tun: boolean;
	    mixedPort: number;
	    proxyMap: boolean;
	    proxyMapStartPort: number;
	    proxyMapGroupId: string;
	    proxyMapGroupIds: string[];
	    externalProxyEndpoints: ExternalProxyEndpoint[];
	    appProxy: boolean;
	    appProxyTargets: AppProxyTarget[];
	    directDomains: string;
	    coreLogLevel: string;
	    ruleSource: string;
	    autoUpdateRules: boolean;
	    ruleUpdateInterval: string;
	    fakeDns: boolean;
	    autoStart: boolean;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.systemProxy = source["systemProxy"];
	        this.tun = source["tun"];
	        this.mixedPort = source["mixedPort"];
	        this.proxyMap = source["proxyMap"];
	        this.proxyMapStartPort = source["proxyMapStartPort"];
	        this.proxyMapGroupId = source["proxyMapGroupId"];
	        this.proxyMapGroupIds = source["proxyMapGroupIds"];
	        this.externalProxyEndpoints = this.convertValues(source["externalProxyEndpoints"], ExternalProxyEndpoint);
	        this.appProxy = source["appProxy"];
	        this.appProxyTargets = this.convertValues(source["appProxyTargets"], AppProxyTarget);
	        this.directDomains = source["directDomains"];
	        this.coreLogLevel = source["coreLogLevel"];
	        this.ruleSource = source["ruleSource"];
	        this.autoUpdateRules = source["autoUpdateRules"];
	        this.ruleUpdateInterval = source["ruleUpdateInterval"];
	        this.fakeDns = source["fakeDns"];
	        this.autoStart = source["autoStart"];
	        this.theme = source["theme"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeState {
	    running: boolean;
	    appVersion: string;
	    coreVersion: string;
	    coreLatestVersion: string;
	    coreUpdateAvailable: boolean;
	    listenUrl: string;
	    selectedId: string;
	    uploadRate: number;
	    downloadRate: number;
	    settings: Settings;
	    rulePresets: RulePreset[];
	    nodes: Node[];
	    groups: NodeGroup[];
	    logs: LogMessage[];
	    configMode: ConfigMode;
	    tunActive: boolean;
	    tunStatus: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.appVersion = source["appVersion"];
	        this.coreVersion = source["coreVersion"];
	        this.coreLatestVersion = source["coreLatestVersion"];
	        this.coreUpdateAvailable = source["coreUpdateAvailable"];
	        this.listenUrl = source["listenUrl"];
	        this.selectedId = source["selectedId"];
	        this.uploadRate = source["uploadRate"];
	        this.downloadRate = source["downloadRate"];
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.rulePresets = this.convertValues(source["rulePresets"], RulePreset);
	        this.nodes = this.convertValues(source["nodes"], Node);
	        this.groups = this.convertValues(source["groups"], NodeGroup);
	        this.logs = this.convertValues(source["logs"], LogMessage);
	        this.configMode = this.convertValues(source["configMode"], ConfigMode);
	        this.tunActive = source["tunActive"];
	        this.tunStatus = source["tunStatus"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class SpeedTestOptions {
	    trafficMB: number;
	    durationSec: number;
	    useSystemProxy: boolean;
	    directOnly: boolean;
	    fastDelay: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SpeedTestOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.trafficMB = source["trafficMB"];
	        this.durationSec = source["durationSec"];
	        this.useSystemProxy = source["useSystemProxy"];
	        this.directOnly = source["directOnly"];
	        this.fastDelay = source["fastDelay"];
	    }
	}

}

