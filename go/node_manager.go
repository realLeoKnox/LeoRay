package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

// testNodeTCPPing performs a raw TCP connection to the node's server address
// to test reachability. This works WITHOUT Xray running.
// Returns the TCP handshake latency or an error string.
func testNodeTCPPing(ob *OutboundObject) string {
	addr, port := extractAddressAndPort(ob)
	if addr == "" {
		return "无法解析"
	}
	target := net.JoinHostPort(addr, fmt.Sprintf("%d", port))

	start := time.Now()
	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return "超时"
	}
	defer conn.Close()
	return time.Since(start).Round(time.Millisecond).String()
}

// testNodeHTTPS performs a real HTTPS request through the running SOCKS5 proxy
// to cp.cloudflare.com/generate_204, measuring the full round-trip including
// DNS resolution (through proxy), TCP connect, TLS handshake, and HTTP response.
// This requires Xray to be running on the standard SOCKS5 port.
func testNodeHTTPS(ob *OutboundObject, socksPort int) (string, string) {
	if socksPort == 0 {
		socksPort = 1080
	}
	proxyAddr := fmt.Sprintf("socks5://127.0.0.1:%d", socksPort)
	proxyURL, _ := url.Parse(proxyAddr)

	// We use a custom transport with TLS to get real HTTPS latency
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	start := time.Now()
	resp, err := client.Get("https://cp.cloudflare.com/generate_204")
	if err != nil {
		return "超时", "HTTPS 握手失败"
	}
	defer resp.Body.Close()
	elapsed := time.Since(start).Round(time.Millisecond).String()

	if resp.StatusCode == 204 || resp.StatusCode == 200 {
		return elapsed, ""
	}
	return elapsed, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// testNodeLatency tests a node's connectivity and returns tcp_ping and connect results.
// When Xray is NOT running:   TCP ping directly to node server (tests reachability).
// When Xray IS running:       HTTPS request through proxy (tests real end-to-end latency).
func testNodeLatency(ob *OutboundObject, xrayRunning bool) (string, string, error) {
	addr, _ := extractAddressAndPort(ob)
	if addr == "" {
		return "无法解析", "无法解析", fmt.Errorf("无法解析节点地址")
	}

	if !xrayRunning {
		// Xray not running: do raw TCP ping to server
		tcpPing := testNodeTCPPing(ob)
		return tcpPing, "未启动", nil
	}

	// Xray running: TCP ping first (direct), then HTTPS through proxy
	tcpPing := testNodeTCPPing(ob)

	httpsLatency, httpsErr := testNodeHTTPS(ob, 1080)
	if httpsErr != "" {
		return tcpPing, httpsErr, nil
	}
	return tcpPing, httpsLatency, nil
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
