package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// extractAddressAndPort 从不同协议的出站对象里解析出底层地址和端口
func extractAddressAndPort(ob *OutboundObject) (string, uint16) {
	b, _ := json.Marshal(ob.Settings)
	if ob.Protocol == "vless" || ob.Protocol == "vmess" {
		var set VnextSettings
		json.Unmarshal(b, &set)
		if len(set.Vnext) > 0 {
			return set.Vnext[0].Address, set.Vnext[0].Port
		}
	} else if ob.Protocol == "trojan" || ob.Protocol == "shadowsocks" {
		var set ServerSettings
		json.Unmarshal(b, &set)
		if len(set.Servers) > 0 {
			return set.Servers[0].Address, set.Servers[0].Port
		}
	}
	return "", 0
}

// injectSockoptIntoOutbound serialises ob to a map and sets (or creates)
// streamSettings.sockopt.interface = iface. This ensures the temporary test
// Xray process binds to the physical NIC so its outbound traffic never enters
// the TUN interface.
func injectSockoptIntoOutbound(ob *OutboundObject, iface string) (map[string]interface{}, error) {
	b, err := json.Marshal(ob)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	ss, _ := m["streamSettings"].(map[string]interface{})
	if ss == nil {
		ss = map[string]interface{}{}
	}
	sockopt, _ := ss["sockopt"].(map[string]interface{})
	if sockopt == nil {
		sockopt = map[string]interface{}{}
	}
	sockopt["interface"] = iface
	ss["sockopt"] = sockopt
	m["streamSettings"] = ss
	return m, nil
}

// testNodeLatency 测试 TCP Ping 与代理握手 Connect.
// activeIf is the physical NIC name (e.g. "en0") when TUN is enabled;
// pass "" when TUN is off. The sub-Xray process will be bound to that
// interface so its outbound traffic bypasses the TUN.
func testNodeLatency(ob *OutboundObject, activeIf string) (string, string, error) {
	addr, port := extractAddressAndPort(ob)
	if addr == "" {
		return "-1ms", "-1ms", fmt.Errorf("无法解析节点地址")
	}

	tcpPing := "-1ms"
	connectPing := "-1ms"

	// 1. TCP Ping via local SOCKS5 proxy
	// Loopback (127.0.0.1) never enters TUN, so this is always safe.
	start := time.Now()
	conn, err := dialViaSocks5("127.0.0.1:1080", addr, port, 4*time.Second)
	if err != nil {
		return "超时", "不通", nil
	}
	tcpPing = time.Since(start).Round(time.Millisecond).String()
	conn.Close()

	// 2. HTTP Connect (验证代理能否正常穿透)
	// Build a minimal Xray config for this node only.
	// When TUN is active, inject sockopt.interface so the sub-process binds to
	// the physical NIC and its traffic never enters the TUN → avoids loop/timeout.
	var testOutbound interface{} = ob
	if activeIf != "" {
		injected, injErr := injectSockoptIntoOutbound(ob, activeIf)
		if injErr == nil {
			testOutbound = injected
		} else {
			fmt.Printf("[Test] sockopt inject failed: %v, falling back to unbound outbound\n", injErr)
		}
	}
	testConfig := XrayConfig{
		Inbounds: []interface{}{
			map[string]interface{}{
				"tag": "socks-test", "port": 10080, "listen": "127.0.0.1", "protocol": "socks",
				"settings": map[string]interface{}{"auth": "noauth", "udp": false},
			},
		},
		Outbounds: []interface{}{testOutbound},
	}
	b, _ := json.Marshal(testConfig)
	tmp, err := os.CreateTemp("", "xray-test-*.json")
	if err != nil {
		return tcpPing, "临时文件创建失败", err
	}
	tmpPath := tmp.Name()
	tmp.Write(b)
	tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command(pathXrayBin, "run", "-c", tmpPath)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+pathAssetDir)
	if err := cmd.Start(); err != nil {
		return tcpPing, "Xray启动失败", err
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// 给予子进程启动与端口绑定短暂喘息时间
	time.Sleep(600 * time.Millisecond)

	// 使用 Go 原生 HTTP Client 发起 SOCKS5 代理请求
	proxyURL, _ := url.Parse("socks5://127.0.0.1:10080")
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 3 * time.Second,
	}

	startHTTP := time.Now()
	resp, err := client.Get("http://cp.cloudflare.com/generate_204")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 204 || resp.StatusCode == 200 {
			connectPing = time.Since(startHTTP).Round(time.Millisecond).String()
		} else {
			connectPing = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	} else {
		connectPing = "代理超时/失败"
	}

	return tcpPing, connectPing, nil
}

// dialViaSocks5 opens a TCP connection to targetHost:targetPort through a
// SOCKS5 proxy at proxyAddr, using the SOCKS5 CONNECT command.
// This avoids using the system resolver or the TUN interface directly.
func dialViaSocks5(proxyAddr, targetHost string, targetPort uint16, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("socks5 connect to proxy: %w", err)
	}
	conn.SetDeadline(time.Now().Add(timeout))

	// SOCKS5 handshake: no auth
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		conn.Close()
		return nil, err
	}
	buf := make([]byte, 2)
	if _, err = conn.Read(buf); err != nil || buf[0] != 0x05 || buf[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 auth failed")
	}

	// SOCKS5 CONNECT request with domain name
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(targetHost))}
	req = append(req, []byte(targetHost)...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err = conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	// Read SOCKS5 reply (minimum 10 bytes)
	reply := make([]byte, 10)
	if _, err = conn.Read(reply); err != nil || reply[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 CONNECT failed: rep=%d", reply[1])
	}
	conn.SetDeadline(time.Time{}) // clear deadline
	return conn, nil
}
