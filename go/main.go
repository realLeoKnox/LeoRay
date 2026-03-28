package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Path constants ────────────────────────────────────────────────────────────
const (
	// config/ — user-editable policy configuration
	pathPolicy        = "config/policy.json"
	pathDefaultPolicy = "config/default_policy.md"

	// data/ — runtime state, subscriptions, logs
	pathCustomNodes = "data/custom_nodes.json"
	pathSub         = "data/sub"
	pathAssetDir    = "./data" // geoip.dat / geosite.dat
	pathLogFile     = "data/xray.log"

	// core/ — xray binary & generated config
	pathXrayConfig = "core/config.json"
	pathXrayBin    = "core/xray"
)

// ─────────────────────────────────────────────────────────────────────────────

type XrayConfig struct {
	Log       map[string]interface{} `json:"log"`
	Dns       map[string]interface{} `json:"dns,omitempty"`
	Inbounds  []interface{}          `json:"inbounds"`
	Outbounds []interface{}          `json:"outbounds"`
	Routing   map[string]interface{} `json:"routing"`
}

// LogBuffer is a thread-safe circular log buffer that optionally mirrors
// all output to a persistent log file.
type LogBuffer struct {
	mu      sync.Mutex
	lines   []string
	max     int
	logFile *os.File // nil means no file logging
}

// SetLogFile enables persistent file logging. Call once after opening the file.
func (l *LogBuffer) SetLogFile(f *os.File) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logFile = f
}

func (l *LogBuffer) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		if strings.TrimSpace(line) != "" {
			l.lines = append(l.lines, line)
		}
	}
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
	os.Stdout.Write(p)
	if l.logFile != nil {
		l.logFile.Write(p)
	}
	return len(p), nil
}

func (l *LogBuffer) GetLines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// APIServer holds all runtime state.
type APIServer struct {
	Mu          sync.Mutex
	Cmd         *exec.Cmd
	Policy      *PolicyConfig
	Outbounds   []*OutboundObject
	Nodes       []string // Outbound tags + "direct" + "block"
	DefaultNode string
	Logs        *LogBuffer
	StartTime   time.Time
	TUNName     string   // currently active utun interface, e.g. "utun4"
	ProxyRoutes []string // per-proxy-IP /32 bypass routes added for TUN mode
}

func main() {
	if exePath, err := os.Executable(); err == nil {
		os.Chdir(filepath.Dir(exePath))
	}
	logs := &LogBuffer{max: 500}

	// Open persistent log file (append mode, created if absent)
	if err := os.MkdirAll("data", 0755); err == nil {
		if lf, err := os.OpenFile(pathLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err == nil {
			logs.SetLogFile(lf)
			fmt.Printf("[LOG] Writing to %s\n", pathLogFile)
		} else {
			fmt.Printf("[LOG] Cannot open log file: %v\n", err)
		}
	}

	server := &APIServer{
		Logs: logs,
	}

	server.reloadDiskData()
	// Xray is NOT started automatically. User controls it via /api/start and /api/stop.

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/api/config", server.handleConfig)
	http.HandleFunc("/api/status", server.handleStatus)
	http.HandleFunc("/api/logs", server.handleLogs)
	http.HandleFunc("/api/sub", server.handleSub)
	http.HandleFunc("/api/nodes", server.handleNodes)
	http.HandleFunc("/api/test_node", server.handleTestNode)
	http.HandleFunc("/api/policy", server.handlePolicy)
	http.HandleFunc("/api/policy/refresh", server.handlePolicyRefresh)
	http.HandleFunc("/api/start", server.handleStart)
	http.HandleFunc("/api/stop", server.handleStop)
	http.HandleFunc("/api/test_route", server.handleTestRoute)
	http.HandleFunc("/api/subscriptions", server.handleSubscriptions)

	// Install sudoers for TUN support in background after startup.
	// Running it here (not in the HTTP handler) prevents osascript from
	// blocking the /api/start request if a password dialog is needed.
	go func() {
		time.Sleep(2 * time.Second)
		xrayAbs, _ := filepath.Abs(pathXrayBin)
		ensureRouteSudoers(xrayAbs)
	}()

	fmt.Println("==================================================")
	fmt.Println("Web GUI + API 已启动： http://127.0.0.1:3406")
	fmt.Println("==================================================")
	if err := http.ListenAndServe(":3406", nil); err != nil {
		fmt.Println("Server error:", err)
	}
}

// reloadDiskDataLocked loads nodes and policy from disk.
// Caller MUST hold s.Mu.
func (s *APIServer) reloadDiskDataLocked() {
	s.Nodes = []string{}

	// 1. Load outbound nodes — try multiple sources in priority order:
	//    a) custom_nodes.json (user-edited)
	//    b) Merge from saved subscriptions
	//    c) Legacy data/sub file
	var outbounds []*OutboundObject
	customBytes, err := os.ReadFile(pathCustomNodes)
	if err == nil {
		json.Unmarshal(customBytes, &outbounds)
	}
	if len(outbounds) == 0 {
		// Try merging from subscription system
		outbounds = MergeAllSubscriptionNodes()
	}
	if len(outbounds) == 0 {
		// Legacy fallback: parse data/sub raw file
		outbounds, err = ParseSub(pathSub)
		if err == nil && len(outbounds) > 0 {
			b, _ := json.MarshalIndent(outbounds, "", "  ")
			os.WriteFile(pathCustomNodes, b, 0644)
		}
	}
	if len(outbounds) > 0 {
		s.Outbounds = outbounds
		for _, ob := range outbounds {
			s.Nodes = append(s.Nodes, ob.Tag)
		}
		s.DefaultNode = outbounds[0].Tag
	} else {
		fmt.Println("[Policy] 暂无节点，路由策略将全部走 direct")
		s.Outbounds = nil
		s.DefaultNode = "direct" // prevent invalid Xray config
	}
	s.Nodes = append(s.Nodes, "direct", "block")

	// 2. Load policy — migrate old mapping.json node assignments on first run
	existingMapping := s.loadOldMapping()
	p, loadErr := LoadPolicy(pathPolicy, pathDefaultPolicy, existingMapping)
	if loadErr != nil {
		fmt.Printf("[Policy] Load error: %v; using empty policy\n", loadErr)
		p = defaultEmptyPolicy()
	}
	s.Policy = p
	// Persist the (possibly newly-initialised) policy
	SavePolicy(p, pathPolicy)
}

// reloadDiskData is the public wrapper: acquires the lock then calls reloadDiskDataLocked.
func (s *APIServer) reloadDiskData() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.reloadDiskDataLocked()
}

// loadOldMapping reads the old mapping.json for backward-compat migration.
func (s *APIServer) loadOldMapping() map[string]string {
	data, err := os.ReadFile("config/mapping.json")
	if err != nil {
		return nil
	}
	var wrapper struct {
		Mapping     map[string]string `json:"mapping"`
		AllowLAN    bool              `json:"allow_lan"`
		EnableTUN   bool              `json:"enable_tun"`
		EnableFakeIP bool             `json:"enable_fakeip"`
		EnableSniff  bool             `json:"enable_sniff"`
	}
	if json.Unmarshal(data, &wrapper) != nil {
		return nil
	}
	m := wrapper.Mapping
	if m == nil {
		m = map[string]string{}
	}
	// Embed booleans as sentinels so initPolicyFromDefault can pick them up
	if wrapper.AllowLAN {
		m["__allow_lan"] = "true"
	}
	if wrapper.EnableTUN {
		m["__enable_tun"] = "true"
	}
	if wrapper.EnableFakeIP {
		m["__enable_fakeip"] = "true"
	}
	if wrapper.EnableSniff {
		m["__enable_sniff"] = "true"
	}
	return m
}

func getDefaultInterface() string {
	out, err := exec.Command("sh", "-c", "netstat -nr -f inet | grep -E '^default' | grep -v utun | awk '{print $NF}' | head -n 1").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// safeTag returns tag if it exists among loaded outbounds or is a built-in tag.
// Falls back to 'direct' to prevent Xray from rejecting an unknown outbound.
func (s *APIServer) safeTag(tag string) string {
	switch tag {
	case "direct", "direct-local", "block", "dns-out":
		return tag
	}
	for _, ob := range s.Outbounds {
		if ob.Tag == tag {
			return tag
		}
	}
	fmt.Printf("[Policy] 节点 '%s' 不存在，回落 direct\n", tag)
	return "direct"
}

func (s *APIServer) applyConfigAndRestartLocked() {

	p := s.Policy
	if p == nil {
		p = defaultEmptyPolicy()
	}

	// ── FIRST: Kill old Xray + clean up routes BEFORE doing any network work ──
	// This ensures DNS resolution (below) is not blocked by stale TUN routes.
	if s.TUNName != "" {
		removeTUNRoutes(s.TUNName)
		s.TUNName = ""
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
		s.Cmd = nil
	}
	// Also kill any orphaned xray from a previous crashed session
	exec.Command("sudo", "-n", "/usr/bin/pkill", "-x", "xray").Run()
	// Brief pause to let the system routing table settle after route deletion
	time.Sleep(150 * time.Millisecond)

	activeIf := ""
	activeGw := ""
	if p.EnableTUN {
		activeIf = getDefaultInterface()
		activeGw = getPhysicalGateway()
		fmt.Printf("[Routing] Active physical interface: %s, Gateway: %s\n", activeIf, activeGw)
	}

	// ── Build outbounds ──────────────────────────────────────────────────────
	// When TUN is active, pre-resolve all proxy server domains to real IPs using
	// rawDNSLookup (bypasses TUN/fakedns entirely). Xray will connect directly
	// to the IP, so its internal DNS module is never consulted for proxy domains.
	var addrMap map[string]string
	if p.EnableTUN && activeIf != "" {
		addrMap = resolveProxyHostnames(s.Outbounds)
		// ── Per-proxy-IP bypass routes (highest priority – overrides 0/1+128/1) ──
		// Without these, Xray's own connections to proxy servers loop back through TUN.
		s.ProxyRoutes = nil
		for _, ip := range addrMap {
			if out, err := exec.Command("sudo", "-n", "/sbin/route", "-n", "add", ip, activeGw).CombinedOutput(); err == nil {
				s.ProxyRoutes = append(s.ProxyRoutes, ip)
				fmt.Printf("[Routing] Added bypass route %s → %s (skips TUN)\n", ip, activeGw)
			} else {
				fmt.Printf("[Routing] Bypass route add failed for %s: %s\n", ip, strings.TrimSpace(string(out)))
			}
		}
		// Establish a scoped default route so sockopt.interface en0 packets know their gateway.
		if activeGw != "" {
			setupScopedDefaultRoute(activeGw, activeIf)
		}
	}

	var finalOutbounds []interface{}
	for _, ob := range s.Outbounds {
		if p.EnableTUN && activeIf != "" {
			obMap := map[string]interface{}{
				"tag": ob.Tag, "protocol": ob.Protocol, "settings": ob.Settings,
			}
			if ob.StreamSettings != nil {
				ss := *ob.StreamSettings
				ss.Sockopt = &SockoptSettings{Interface: activeIf}
				obMap["streamSettings"] = ss
			} else {
				obMap["streamSettings"] = map[string]interface{}{
					"sockopt": map[string]interface{}{"interface": activeIf},
				}
			}
			// Substitute domain with pre-resolved IP to bypass fakedns
			if addrMap != nil {
				patched := patchOutboundAddress(ob, addrMap)
				if patchedMap, ok := patched.(map[string]interface{}); ok {
					// Merge streamSettings into patched map (keep sockopt)
					patchedMap["streamSettings"] = obMap["streamSettings"]
					finalOutbounds = append(finalOutbounds, patchedMap)
					continue
				}
			}
			finalOutbounds = append(finalOutbounds, obMap)
		} else {
			finalOutbounds = append(finalOutbounds, ob)
		}
	}

	if p.EnableTUN && activeIf != "" {
		finalOutbounds = append(finalOutbounds, map[string]interface{}{
			"tag": "direct", "protocol": "freedom", 
			"settings": map[string]interface{}{
				"domainStrategy": "UseIPv4",
			},
			"streamSettings": map[string]interface{}{
				"sockopt": map[string]interface{}{"interface": activeIf},
			},
		})
	} else {
		finalOutbounds = append(finalOutbounds, map[string]interface{}{
			"tag": "direct", "protocol": "freedom", 
			"settings": map[string]interface{}{
				"domainStrategy": "UseIPv4",
			},
		})
	}
	finalOutbounds = append(finalOutbounds,
		map[string]interface{}{
			"tag": "direct-local", "protocol": "freedom", 
			"settings": map[string]interface{}{
				"domainStrategy": "UseIPv4",
			},
		},
		map[string]interface{}{
			"tag": "block", "protocol": "blackhole",
			"settings": map[string]interface{}{"response": map[string]interface{}{"type": "http"}},
		},
	)

	// ── Build inbounds ───────────────────────────────────────────────────────
	// Decide TUN interface name early so we can use it for both config and route setup.
	targetTun := ""
	if p.EnableTUN {
		targetTun = "utun234" // requested by user
	}

	listenAddr := "127.0.0.1"
	if p.AllowLAN {
		listenAddr = "0.0.0.0"
	}
	inbounds := []interface{}{
		map[string]interface{}{
			"tag": "socks-in", "port": 1080, "listen": listenAddr, "protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth", "udp": true},
		},
		map[string]interface{}{
			"tag": "http-in", "port": 1081, "listen": listenAddr, "protocol": "http",
			"settings": map[string]interface{}{},
		},
	}
	if p.EnableTUN {
		inbounds = append(inbounds, map[string]interface{}{
			"tag": "tun-in", "protocol": "tun",
			"settings": map[string]interface{}{
				"name":        targetTun,
				"network":     "tcp,udp",
				"mtu":         9000,
				"autoRoute":   true,
				"strictRoute": false, // prevent pf from hijacking 127.0.0.1
			},
		})
	}

	// ── Sniffing ─────────────────────────────────────────────────────────────
	if p.EnableSniff || p.EnableTUN {
		sniffOverrides := []string{"http", "tls", "quic"}
		if p.EnableFakeIP {
			sniffOverrides = append(sniffOverrides, "fakedns")
		}
		sniffing := map[string]interface{}{
			"enabled": true, "destOverride": sniffOverrides, "routeOnly": false,
		}
		for i, ib := range inbounds {
			ibMap := ib.(map[string]interface{})
			ibMap["sniffing"] = sniffing
			inbounds[i] = ibMap
		}
	}

	// ── DNS Anti-Pollution + FakeIP ──────────────────────────────────────────────
	// Build split DNS: DirectDNS for CN/direct traffic, ProxyDNS for proxied traffic.
	// Enabled for BOTH TUN and non-TUN modes to combat DNS pollution.
	dc := p.DnsConfig
	if len(dc.DirectDNS) == 0 {
		dc.DirectDNS = []string{"223.5.5.5", "123.123.123.124"}
	}
	if len(dc.ProxyDNS) == 0 {
		dc.ProxyDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// Determine the primary direct DNS address (for proxy hostname resolution, FakeIP bypass, etc.)
	primaryDirectDNS := dc.DirectDNS[0]
	if dc.DirectDOH != "" {
		primaryDirectDNS = dc.DirectDOH
	}

	var dnsConfig map[string]interface{}
	{
		dnsServers := []interface{}{}

		if p.EnableFakeIP && p.EnableTUN {
			// ── Critical FakeIP fix ────────────────────────────────────────────────
			// Insert a real-DNS rule for proxy server hostnames BEFORE fakedns
			// so they always resolve to real IPs, never fake ones.
			proxyHostnames := extractProxyHostnames(s.Outbounds)
			if len(proxyHostnames) > 0 {
				dnsServers = append(dnsServers, map[string]interface{}{
					"address": primaryDirectDNS,
					"domains": proxyHostnames,
				})
			}
			dnsServers = append(dnsServers, "fakedns")
		}

		// Proxy DNS: for non-CN domains (resolved via proxy to avoid pollution)
		proxyDNSAddr := dc.ProxyDNS[0]
		if dc.ProxyDOH != "" {
			proxyDNSAddr = dc.ProxyDOH
		}
		dnsServers = append(dnsServers, map[string]interface{}{
			"address": proxyDNSAddr,
			"domains": []string{"geosite:geolocation-!cn"},
		})

		// Direct DNS: for CN domains + fallback
		directDNSAddr := dc.DirectDNS[0]
		if dc.DirectDOH != "" {
			directDNSAddr = dc.DirectDOH
		}
		dnsServers = append(dnsServers, map[string]interface{}{
			"address": directDNSAddr,
			"domains": []string{"geosite:cn"},
		})
		// Add remaining direct DNS as plain fallback
		for _, dns := range dc.DirectDNS {
			dnsServers = append(dnsServers, dns)
		}

		dnsConfig = map[string]interface{}{
			"servers":       dnsServers,
			"queryStrategy": "UseIP",
		}
		if p.EnableFakeIP && p.EnableTUN {
			dnsConfig["fakedns"] = []map[string]interface{}{
				{"ipPool": "198.18.0.0/15", "poolSize": 65535},
			}
		}
	}

	if p.EnableTUN {
		// dns-out: bind to physical NIC so Xray's own DNS queries bypass TUN.
		dnsOutbound := map[string]interface{}{
			"tag": "dns-out", "protocol": "dns",
		}
		if activeIf != "" {
			dnsOutbound["streamSettings"] = map[string]interface{}{
				"sockopt": map[string]interface{}{"interface": activeIf},
			}
		}
		finalOutbounds = append(finalOutbounds, dnsOutbound)
	}

	// ── Routing rules ────────────────────────────────────────────────────────
	xrayRules := BuildXrayRulesFromPolicy(p, s.DefaultNode, false)

	// Collect all DNS IPs that need direct bypass routing
	var dnsIPs []string
	for _, ip := range dc.DirectDNS {
		dnsIPs = append(dnsIPs, ip)
	}
	for _, ip := range dc.ProxyDNS {
		dnsIPs = append(dnsIPs, ip)
	}

	// TUN pre-pend: LAN bypass + DNS intercept
	if p.EnableTUN {
		xrayRules = append([]XrayRouteRule{
			{Type: "field", IP: []string{"127.0.0.0/8", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "fc00::/7", "fe80::/10"}, OutboundTag: "direct-local"},
			{Type: "field", Domain: []string{"localhost"}, OutboundTag: "direct-local"},
			{Type: "field", InboundTag: []string{"tun-in", "socks-in"}, Port: "53", Network: "tcp,udp", OutboundTag: "dns-out"},
			{Type: "field", IP: dnsIPs, OutboundTag: "direct"},
		}, xrayRules...)
	}

	// Catch-all / FINAL rule — safeTag ensures the tag exists even when no nodes are loaded
	finalTag := s.safeTag(resolvePolicy(p.Final, p.Groups, s.DefaultNode))
	xrayRules = append(xrayRules, XrayRouteRule{
		Type: "field", Network: "tcp,udp", OutboundTag: finalTag,
	})

	// ── Write config.json & restart Xray ────────────────────────────────────
	config := XrayConfig{
		Log:      map[string]interface{}{"loglevel": "warning"},
		Dns:      dnsConfig,
		Inbounds: inbounds, Outbounds: finalOutbounds,
		Routing: map[string]interface{}{"domainStrategy": "IPIfNonMatch", "rules": xrayRules},
	}
	bytes, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(pathXrayConfig, bytes, 0644)

	// ── Start Xray ───────────────────────────────────────────────────────────
	s.Logs.Write([]byte("[SYSTEM] 正在重启 Xray...\n"))

	if p.EnableTUN {
		// TUN requires root to create utun device.
		// sudoers is set up at startup (main goroutine), not here, to avoid blocking.
		xrayAbs, _ := filepath.Abs(pathXrayBin)
		cfgAbs, _ := filepath.Abs(pathXrayConfig)
		assetAbs, _ := filepath.Abs(pathAssetDir)
		s.Cmd = exec.Command("sudo", "-n", xrayAbs, "run", "-c", cfgAbs)
		s.Cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+assetAbs)
	} else {
		s.Cmd = exec.Command(pathXrayBin, "run", "-c", pathXrayConfig)
		s.Cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+pathAssetDir)
	}
	s.Cmd.Stdout = s.Logs
	s.Cmd.Stderr = s.Logs


	if err := s.Cmd.Start(); err == nil {
		s.StartTime = time.Now()
		s.Logs.Write([]byte(fmt.Sprintf("[SYSTEM] Xray 启动成功，PID: %d\n", s.Cmd.Process.Pid)))
		// macOS: autoRoute creates the utun but does NOT add routing rules.
		// We must manually add 0/1 and 128/1 routes after the interface appears.
		if p.EnableTUN && targetTun != "" {
			go func(name string) {
				// Poll until utun interface is created by Xray
				for i := 0; i < 30; i++ {
					time.Sleep(300 * time.Millisecond)
					if exec.Command("ifconfig", name).Run() == nil {
						break
					}
				}
				addTUNRoutes(name)
				s.Mu.Lock()
				s.TUNName = name
				s.Mu.Unlock()
				s.Logs.Write([]byte(fmt.Sprintf("[TUN] 路由表配置完成 (%s)\n", name)))
			}(targetTun)
		}
	} else {
		s.Logs.Write([]byte(fmt.Sprintf("[SYSTEM] Xray 启动失败: %v\n", err)))
	}
}

// applyConfigAndRestart acquires the lock and calls applyConfigAndRestartLocked.
func (s *APIServer) applyConfigAndRestart() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.applyConfigAndRestartLocked()
}

// reloadAndRestart atomically reloads disk data and restarts Xray under a single lock,
// eliminating the race window between the two operations.
func (s *APIServer) reloadAndRestart() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.reloadDiskDataLocked()
	s.applyConfigAndRestartLocked()
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP Handlers
// ─────────────────────────────────────────────────────────────────────────────

// handleConfig: compatibility endpoint → returns node list for legacy UI elements
func (s *APIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": s.Nodes,
	})
}

// handlePolicy: GET returns full policy; POST saves and restarts Xray only if it was already running.
func (s *APIServer) handlePolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.Mu.Lock()
		p := s.Policy
		s.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		var p PolicyConfig
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), 400)
			return
		}
		s.Mu.Lock()
		s.Policy = &p
		SavePolicy(&p, pathPolicy)
		running := s.isXrayRunningLocked()
		s.Mu.Unlock()
		if running {
			s.applyConfigAndRestart()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}
}

// handleStart: starts the Xray proxy core.
func (s *APIServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.applyConfigAndRestart()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// handleStop: stops the Xray proxy core, leaving xray_controller running.
func (s *APIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.Mu.Lock()
	s.stopXrayLocked()
	s.Mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// stopXrayLocked kills Xray (both the sudo wrapper and real xray child) and removes TUN routes.
// Caller MUST hold s.Mu.
func (s *APIServer) stopXrayLocked() {
	// Remove per-proxy-IP bypass routes first
	for _, ip := range s.ProxyRoutes {
		exec.Command("sudo", "-n", "/sbin/route", "-n", "delete", ip).Run()
	}
	s.ProxyRoutes = nil

	if s.TUNName != "" {
		removeTUNRoutes(s.TUNName)
		s.TUNName = ""
	}

	if s.Cmd != nil && s.Cmd.Process != nil {
		pid := s.Cmd.Process.Pid
		// Xray runs as root (via sudo). We cannot kill a root process from non-root
		// with os.Process.Kill() — it returns EPERM and Wait() blocks forever.
		// Use 'sudo -n kill -9' instead.
		exec.Command("sudo", "-n", "/bin/kill", "-9", strconv.Itoa(pid)).Run()
		// Wait with a timeout so stopXrayLocked never hangs indefinitely.
		done := make(chan struct{})
		go func() { s.Cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			fmt.Println("[SYSTEM] Wait timeout — xray may still be alive")
		}
		s.Cmd = nil
	}
	// Kill any orphaned xray processes (in case sudo didn't relay the signal)
	exec.Command("sudo", "-n", "/usr/bin/pkill", "-x", "xray").Run()
}

// isXrayRunningLocked returns true if Xray is running. Caller MUST hold s.Mu.
func (s *APIServer) isXrayRunningLocked() bool {
	return s.Cmd != nil && s.Cmd.Process != nil && s.Cmd.ProcessState == nil
}

// handlePolicyRefresh: fetches all remote rule-sets in background, saves cache, then restarts.
func (s *APIServer) handlePolicyRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	// Respond immediately; do the heavy lifting in background
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","message":"正在后台拉取规则并重启..."}`))

	go func() {
		s.Mu.Lock()
		p := s.Policy
		s.Mu.Unlock()
		if p == nil {
			return
		}
		// Force-fetch all enabled rule sets (saves to local cache automatically)
		for _, rs := range p.RuleSets {
			if !rs.Enabled || rs.URL == "" {
				continue
			}
			fetchRuleLines(rs, true) // side-effect: saves to rs.Local
		}
		// Restart with fresh cache only if Xray is already running
		s.Mu.Lock()
		running := s.isXrayRunningLocked()
		s.Mu.Unlock()
		if running {
			s.applyConfigAndRestart()
		}
	}()
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	running := s.Cmd != nil && s.Cmd.Process != nil && s.Cmd.ProcessState == nil
	pid := 0
	uptime := "N/A"
	if running {
		pid = s.Cmd.Process.Pid
		uptime = time.Since(s.StartTime).Round(time.Second).String()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running": running, "pid": pid, "uptime": uptime,
	})
}

func (s *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"logs": s.Logs.GetLines()})
}

func (s *APIServer) handleSub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]string
	json.Unmarshal(body, &req)

	content := req["content"]
	if urlStr := req["url"]; urlStr != "" {
		resp, err := http.Get(urlStr)

		if err != nil {
			http.Error(w, "Download failed", 500)
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		content = string(b)
	}
	if content == "" {
		http.Error(w, "Empty params", 400)
		return
	}
	os.WriteFile(pathSub, []byte(content), 0644)
	os.Remove(pathCustomNodes)
	// Reload disk data; restart Xray only if it was already running
	s.Mu.Lock()
	running := s.isXrayRunningLocked()
	s.Mu.Unlock()
	if running {
		s.reloadAndRestart()
	} else {
		s.reloadDiskData()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// handleSubscriptions: GET returns subscription list, POST adds/refreshes/deletes.
func (s *APIServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		subs, _ := LoadSubscriptions()
		json.NewEncoder(w).Encode(subs)

	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Action string `json:"action"` // "add", "refresh", "refresh_all", "delete"
			ID     string `json:"id,omitempty"`
			Name   string `json:"name,omitempty"`
			URL    string `json:"url,omitempty"`
		}
		json.Unmarshal(body, &req)

		switch req.Action {
		case "add":
			if req.URL == "" {
				http.Error(w, "URL required", 400)
				return
			}
			name := req.Name
			if name == "" {
				name = "订阅 " + time.Now().Format("01-02 15:04")
			}
			sub, _, err := AddSubscription(name, req.URL)
			if err != nil {
				http.Error(w, "Add failed: "+err.Error(), 500)
				return
			}
			// Rebuild merged node list
			allNodes := MergeAllSubscriptionNodes()
			if len(allNodes) > 0 {
				nb, _ := json.MarshalIndent(allNodes, "", "  ")
				os.WriteFile(pathCustomNodes, nb, 0644)
			}
			s.reloadDiskData()
			json.NewEncoder(w).Encode(sub)

		case "refresh":
			if req.ID == "" {
				http.Error(w, "ID required", 400)
				return
			}
			_, err := RefreshSubscription(req.ID)
			if err != nil {
				http.Error(w, "Refresh failed: "+err.Error(), 500)
				return
			}
			allNodes := MergeAllSubscriptionNodes()
			if len(allNodes) > 0 {
				nb, _ := json.MarshalIndent(allNodes, "", "  ")
				os.WriteFile(pathCustomNodes, nb, 0644)
			}
			s.Mu.Lock()
			running := s.isXrayRunningLocked()
			s.Mu.Unlock()
			if running {
				s.reloadAndRestart()
			} else {
				s.reloadDiskData()
			}
			w.Write([]byte(`{"status":"ok"}`))

		case "refresh_all":
			_, err := RefreshAllSubscriptions()
			if err != nil {
				http.Error(w, "Refresh all failed: "+err.Error(), 500)
				return
			}
			allNodes := MergeAllSubscriptionNodes()
			if len(allNodes) > 0 {
				nb, _ := json.MarshalIndent(allNodes, "", "  ")
				os.WriteFile(pathCustomNodes, nb, 0644)
			}
			s.Mu.Lock()
			running := s.isXrayRunningLocked()
			s.Mu.Unlock()
			if running {
				s.reloadAndRestart()
			} else {
				s.reloadDiskData()
			}
			w.Write([]byte(`{"status":"ok"}`))

		case "delete":
			if req.ID == "" {
				http.Error(w, "ID required", 400)
				return
			}
			DeleteSubscription(req.ID)
			allNodes := MergeAllSubscriptionNodes()
			nb, _ := json.MarshalIndent(allNodes, "", "  ")
			os.WriteFile(pathCustomNodes, nb, 0644)
			s.reloadDiskData()
			w.Write([]byte(`{"status":"ok"}`))

		default:
			http.Error(w, "Unknown action", 400)
		}
	}
}

func (s *APIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.Mu.Lock()
		defer s.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.Outbounds)
	case http.MethodPost:
		var newOutbounds []*OutboundObject
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &newOutbounds); err != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		s.Mu.Lock()
		s.Outbounds = newOutbounds
		b, _ := json.MarshalIndent(newOutbounds, "", "  ")
		os.WriteFile(pathCustomNodes, b, 0644)
		running := s.isXrayRunningLocked()
		s.Mu.Unlock()
		if running {
			s.reloadAndRestart()
		}
		w.Write([]byte(`{"status":"ok"}`))
	}
}

func (s *APIServer) handleTestNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Index int `json:"index"`
	}
	json.Unmarshal(body, &req)

	s.Mu.Lock()
	if req.Index < 0 || req.Index >= len(s.Outbounds) {
		s.Mu.Unlock()
		http.Error(w, "Invalid index", 400)
		return
	}
	targetOb := s.Outbounds[req.Index]
	running := s.isXrayRunningLocked()
	s.Mu.Unlock()

	tcpPing, connectPing, err := testNodeLatency(targetOb, running)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "tcp_ping": tcpPing, "connect": connectPing})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tcp_ping": tcpPing, "connect": connectPing})
}

func getLowestUnusedUtun() string {
	for i := 0; i < 255; i++ {
		name := fmt.Sprintf("utun%d", i)
		if err := exec.Command("ifconfig", name).Run(); err != nil {
			return name // Error implies it does not exist, safe to claim
		}
	}
	return "utun9" // Fallback
}

const sudoersFile = "/etc/sudoers.d/leoray-route"

// ensureRouteSudoers installs (or upgrades) the sudoers rule covering:
//   /sbin/route       — adding/removing TUN routes
//   xrayPath          — running xray as root for TUN device creation
//   /usr/bin/pkill    — killing orphaned xray children
// Shows one password dialog on first use; re-prompts only when upgrading.
func ensureRouteSudoers(xrayPath string) {
	content := fmt.Sprintf(
		"%%admin ALL=(ALL) NOPASSWD: /sbin/route\n"+
			"%%admin ALL=(ALL) NOPASSWD: %s\n"+
			"%%admin ALL=(ALL) NOPASSWD: /usr/bin/pkill\n"+
			"%%admin ALL=(ALL) NOPASSWD: /bin/kill\n",
		xrayPath)

	// Skip if existing file is already up-to-date
	if data, err := os.ReadFile(sudoersFile); err == nil {
		if strings.Contains(string(data), xrayPath) &&
			strings.Contains(string(data), "/usr/bin/pkill") &&
			strings.Contains(string(data), "/bin/kill") {
			return
		}
		fmt.Println("[TUN] sudoers 需要更新…")
	} else {
		fmt.Println("[TUN] 首次配置权限，请在弹出框中输入管理员密码…")
	}

	tmpPath := "/tmp/leoray-sudoers-tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		fmt.Printf("[TUN] sudoers 写入临时文件失败: %v\n", err)
		return
	}
	defer os.Remove(tmpPath)
	script := fmt.Sprintf(`do shell script "cp %s %s && chmod 440 %s" with administrator privileges`,
		tmpPath, sudoersFile, sudoersFile)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		fmt.Printf("[TUN] sudoers 安装失败: %v\n", err)
	} else {
		fmt.Printf("[TUN] sudoers 已安装: %s\n", sudoersFile)
	}
}


// addTUNRoutes adds the routes needed to capture all traffic via the TUN interface.
// sudoers must already be installed (done in applyConfigAndRestartLocked before xray starts).
func addTUNRoutes(name string) {
	for _, args := range [][]string{
		{"sudo", "-n", "/sbin/route", "-n", "add", "-net", "0.0.0.0/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "add", "-net", "128.0.0.0/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "add", "-inet6", "::/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "add", "-inet6", "8000::/1", "-interface", name},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			fmt.Printf("[TUN] route add failed (%v): %s\n", err, strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("[TUN] route add 0/1, 128/1 → %s\n", name)
}

// removeTUNRoutes removes the routes added by addTUNRoutes before Xray is killed.
// sudoers is already installed when TUN was first enabled.
func removeTUNRoutes(name string) {
	for _, args := range [][]string{
		{"sudo", "-n", "/sbin/route", "-n", "delete", "-net", "0.0.0.0/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "delete", "-net", "128.0.0.0/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "delete", "-inet6", "::/1", "-interface", name},
		{"sudo", "-n", "/sbin/route", "-n", "delete", "-inet6", "8000::/1", "-interface", name},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			fmt.Printf("[TUN] route delete failed (%v): %s\n", err, strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("[TUN] route delete 0/1, 128/1 from %s\n", name)
}

// skipDNSName advances offset past a DNS name (label sequence or compressed pointer)
// in the given buffer. Handles both uncompressed labels and RFC 1035 pointer compression.
func skipDNSName(buf []byte, offset int) int {
	for offset < len(buf) {
		length := buf[offset]
		if length == 0 {
			return offset + 1 // end of name
		}
		if length&0xC0 == 0xC0 { // compressed pointer: 2-byte field
			return offset + 2
		}
		offset += int(length) + 1
	}
	return offset
}

// rawDNSLookup resolves a hostname by sending a raw UDP A-record query directly
// to dnsServer (e.g. "223.5.5.5:53"), completely bypassing the OS resolver and
// therefore the TUN interface. Safe to call before or after TUN is active.
func rawDNSLookup(host, dnsServer string) []string {
	// Build a minimal DNS query for the A record
	txID := uint16(rand.Intn(65535) + 1)
	query := make([]byte, 12)
	binary.BigEndian.PutUint16(query[0:2], txID)   // Transaction ID
	binary.BigEndian.PutUint16(query[2:4], 0x0100) // Flags: standard query, recursion desired
	binary.BigEndian.PutUint16(query[4:6], 1)      // QDCOUNT = 1
	// Encode the QNAME
	for _, label := range strings.Split(host, ".") {
		query = append(query, byte(len(label)))
		query = append(query, []byte(label)...)
	}
	query = append(query, 0x00)       // end of QNAME
	query = append(query, 0x00, 0x01) // QTYPE = A
	query = append(query, 0x00, 0x01) // QCLASS = IN

	conn, err := net.DialTimeout("udp", dnsServer, 3*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = conn.Write(query); err != nil {
		return nil
	}
	// 4096 bytes: safely covers large multi-IP responses (CDN, etc.)
	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil || n < 12 {
		return nil
	}
	// Skip question section
	offset := 12
	qCount := int(binary.BigEndian.Uint16(resp[4:6]))
	for i := 0; i < qCount && offset < n; i++ {
		offset = skipDNSName(resp[:n], offset)
		offset += 4 // QTYPE + QCLASS
	}
	// Parse answer section
	var ips []string
	anCount := int(binary.BigEndian.Uint16(resp[6:8]))
	for i := 0; i < anCount && offset+10 <= n; i++ {
		offset = skipDNSName(resp[:n], offset) // skip name
		if offset+10 > n {
			break
		}
		rrType := binary.BigEndian.Uint16(resp[offset : offset+2])
		rdLen := int(binary.BigEndian.Uint16(resp[offset+8 : offset+10]))
		offset += 10
		if rrType == 1 && rdLen == 4 && offset+4 <= n { // A record
			ip := net.IP(resp[offset : offset+4]).String()
			ips = append(ips, ip)
		}
		offset += rdLen
	}
	return ips
}

// extractProxyHostnames returns the unique set of domain-name (non-IP) addresses
// used by all outbound nodes. Used to build a pre-fakedns DNS rule that ensures
// proxy server domains always resolve to real IPs instead of FakeIPs.
func extractProxyHostnames(outbounds []*OutboundObject) []string {
	seen := map[string]bool{}
	var hostnames []string
	for _, ob := range outbounds {
		addr, _ := extractAddressAndPort(ob)
		if addr == "" {
			continue
		}
		// Only add hostname-style addresses (not bare IPs)
		if net.ParseIP(addr) != nil {
			continue
		}
		if !seen[addr] {
			seen[addr] = true
			hostnames = append(hostnames, "full:"+addr)
		}
	}
	return hostnames
}

// resolveProxyHostnames resolves all proxy domain names using rawDNSLookup
// (raw UDP, bypasses TUN and OS resolver entirely).
// Returns a hostname → first-resolved-IPv4 map.
// This map is used to SUBSTITUTE domain names in the Xray config with real IPs
// so Xray's outbound DNS resolution never touches fakedns.
func resolveProxyHostnames(outbounds []*OutboundObject) map[string]string {
	result := map[string]string{}
	for _, ob := range outbounds {
		addr, _ := extractAddressAndPort(ob)
		if addr == "" || net.ParseIP(addr) != nil {
			continue // already an IP, no substitution needed
		}
		if _, already := result[addr]; already {
			continue // already resolved
		}
		ips := rawDNSLookup(addr, "223.5.5.5:53")
		if len(ips) == 0 {
			ips = rawDNSLookup(addr, "114.114.114.114:53")
		}
		for _, ip := range ips {
			if net.ParseIP(ip) != nil && !strings.HasPrefix(ip, "198.18.") && !strings.HasPrefix(ip, "198.19.") {
				result[addr] = ip
				fmt.Printf("[DNS] Resolved proxy domain %s → %s\n", addr, ip)
				break
			}
		}
		if _, found := result[addr]; !found {
			fmt.Printf("[DNS] Could not resolve proxy domain %s (will keep domain)\n", addr)
		}
	}
	return result
}

// patchOutboundAddress replaces the proxy server address in an outbound object
// with a pre-resolved IP (if available). This prevents Xray from using its
// internal DNS (which could hit fakedns) to resolve proxy server addresses.
// Returns a map[string]interface{} (JSON-friendly) if patched, else the original ob.
func patchOutboundAddress(ob *OutboundObject, addrMap map[string]string) interface{} {
	addr, _ := extractAddressAndPort(ob)
	if addr == "" {
		return ob
	}
	resolvedIP, ok := addrMap[addr]
	if !ok {
		return ob // domain not in map (already IP or lookup failed)
	}
	b, err := json.Marshal(ob)
	if err != nil {
		return ob
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return ob
	}
	settings, _ := m["settings"].(map[string]interface{})
	if settings == nil {
		return ob
	}
	switch ob.Protocol {
	case "vless", "vmess":
		if vnext, ok2 := settings["vnext"].([]interface{}); ok2 && len(vnext) > 0 {
			if server, ok3 := vnext[0].(map[string]interface{}); ok3 {
				server["address"] = resolvedIP
				vnext[0] = server
				settings["vnext"] = vnext
			}
		}
	case "trojan", "shadowsocks":
		if servers, ok2 := settings["servers"].([]interface{}); ok2 && len(servers) > 0 {
			if server, ok3 := servers[0].(map[string]interface{}); ok3 {
				server["address"] = resolvedIP
				servers[0] = server
				settings["servers"] = servers
			}
		}
	default:
		return ob
	}
	m["settings"] = settings
	return m
}

// getPhysicalGateway retrieves the IP of the default physical gateway
func getPhysicalGateway() string {
	out, err := exec.Command("sh", "-c", "netstat -nr -f inet | awk '/^default/ && $NF != \"utun\" && !/utun/ {print $2}' | head -n 1").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// setupScopedDefaultRoute solves the macOS network unreachable bug for TUN bypass.
// macOS drops packets strictly bound to `en0` via SO_BINDTODEVICE if the 0/1
// route hides the default gateway. A scoped route (-ifscope) forces macOS to
// resolve the MAC address locally for all packets explicitly bound to that interface.
func setupScopedDefaultRoute(gatewayIP, iface string) {
	exec.Command("sudo", "-n", "/sbin/route", "delete", "default", "-ifscope", iface).Run()
	err := exec.Command("sudo", "-n", "/sbin/route", "add", "default", gatewayIP, "-ifscope", iface).Run()
	if err == nil {
		fmt.Printf("[Routing] Scoped default route: %s → %s via %s\n", iface, "default", gatewayIP)
	} else {
		fmt.Printf("[Routing] Scoped default route failed: %v\n", err)
	}
}

func (s *APIServer) handleTestRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Target string `json:"target"`
		Method string `json:"method"` // "xray" or "go"
	}
	json.Unmarshal(body, &req)

	if req.Target == "" {
		http.Error(w, "Missing target", 400)
		return
	}
	if req.Method == "" {
		req.Method = "xray"
	}

	w.Header().Set("Content-Type", "application/json")

	if req.Method == "go" {
		s.Mu.Lock()
		p := s.Policy
		defaultNode := s.DefaultNode
		s.Mu.Unlock()
		if p == nil {
			json.NewEncoder(w).Encode(map[string]string{"outbound": "unknown", "rule": "未加载策略"})
			return
		}
		fallbackTag := s.safeTag(resolvePolicy(p.Final, p.Groups, defaultNode))
		rules := BuildXrayRulesFromPolicy(p, fallbackTag, false)
		tag, matchedRule := simulateRouteMatch(req.Target, rules, fallbackTag)

		json.NewEncoder(w).Encode(map[string]string{
			"outbound": s.safeTag(tag),
			"rule":     matchedRule,
		})
		return
	}

	s.Mu.Lock()
	running := s.isXrayRunningLocked()
	s.Mu.Unlock()

	if !running {
		json.NewEncoder(w).Encode(map[string]string{
			"outbound": "error",
			"error":    "使用 Xray 内核测试，请先在 Dashboard 启动核心",
		})
		return
	}

	s.Logs.mu.Lock()
	startIndex := len(s.Logs.lines)
	s.Logs.mu.Unlock()

	go func() {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:1080", 2*time.Second)
		if err == nil {
			defer conn.Close()
			conn.Write([]byte{0x05, 0x01, 0x00})
			buf := make([]byte, 2)
			conn.Read(buf)
			cmd := []byte{0x05, 0x01, 0x00, 0x03, byte(len(req.Target))}
			cmd = append(cmd, []byte(req.Target)...)
			cmd = append(cmd, 0x00, 0x50)
			conn.Write(cmd)
		}
	}()

	outbound := "unknown"
	for i := 0; i < 25; i++ {
		time.Sleep(100 * time.Millisecond)
		s.Logs.mu.Lock()
		var newLines []string
		if len(s.Logs.lines) > startIndex {
			newLines = append([]string(nil), s.Logs.lines[startIndex:]...)
		}
		s.Logs.mu.Unlock()

		for _, line := range newLines {
			if strings.Contains(line, "accepted tcp:"+req.Target) || strings.Contains(line, "accepted udp:"+req.Target) {
				idx := strings.LastIndex(line, " -> ")
				if idx != -1 {
					endIdx := strings.Index(line[idx+4:], "]")
					if endIdx != -1 {
						outbound = line[idx+4 : idx+4+endIdx]
						break
					}
				}
			}
		}
		if outbound != "unknown" {
			break
		}
	}

	if outbound == "unknown" {
		json.NewEncoder(w).Encode(map[string]string{
			"outbound": "timeout",
			"error":    "请求已发送，但在日志中未命中路由出站记录",
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{
		"outbound": outbound,
		"rule":     "Xray 分配 [真实探针日志提取]",
	})
}

func simulateRouteMatch(target string, rules []XrayRouteRule, finalTag string) (string, string) {
	host := target
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	isIP := net.ParseIP(host) != nil

	for _, rule := range rules {
		if !isIP && len(rule.Domain) > 0 {
			for _, d := range rule.Domain {
				if strings.HasPrefix(d, "full:") {
					if host == strings.TrimPrefix(d, "full:") {
						return rule.OutboundTag, d
					}
				} else if strings.HasPrefix(d, "domain:") {
					suffix := strings.TrimPrefix(d, "domain:")
					if host == suffix || strings.HasSuffix(host, "."+suffix) {
						return rule.OutboundTag, d
					}
				} else if strings.HasPrefix(d, "keyword:") {
					kw := strings.TrimPrefix(d, "keyword:")
					if strings.Contains(host, kw) {
						return rule.OutboundTag, d
					}
				} else if strings.HasPrefix(d, "geosite:") {
					kw := strings.TrimPrefix(d, "geosite:")
					if strings.Contains(host, kw) {
						return rule.OutboundTag, d + " [注: Go近似推断]"
					}
				}
			}
		}
		if isIP && len(rule.IP) > 0 {
			for _, ipCidr := range rule.IP {
				if strings.HasPrefix(ipCidr, "geoip:") {
					continue
				}
				if strings.Contains(ipCidr, "/") {
					_, ipCidrNet, err := net.ParseCIDR(ipCidr)
					if err == nil && ipCidrNet.Contains(net.ParseIP(host)) {
						return rule.OutboundTag, ipCidr
					}
				} else if ipCidr == host {
					return rule.OutboundTag, ipCidr
				}
			}
		}
	}
	return finalTag, "默认/兜底规则"
}
