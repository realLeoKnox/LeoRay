package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type SubData struct {
	Links []string `json:"links"`
}

// extractLinks 智能判断文本是一般 Base64 还是定制的 JSON，最后切分出所有的协议链接
func extractLinks(data []byte) []string {
	// 1. 尝试作为定制 JSON (如本例用户自传的 sub 文件格式)
	var jData SubData
	if err := json.Unmarshal(data, &jData); err == nil && len(jData.Links) > 0 {
		return jData.Links
	}

	strData := string(data)
	
	// 2. 尝试解析为标准的 base64/base64URL 订阅信息（包含多行 vmesses）
	// 通常 sub 节点是用 std/url encoder 的，处理 padding。
	decoded, err := base64.StdEncoding.DecodeString(strData)
	if err != nil {
		// 很多 sub 会省略末尾的等号，或者使用 url encoding
		strData = strings.ReplaceAll(strData, "-", "+")
		strData = strings.ReplaceAll(strData, "_", "/")
		if m := len(strData) % 4; m != 0 {
			strData += strings.Repeat("=", 4-m)
		}
		decoded, err = base64.StdEncoding.DecodeString(strData)
	}
	if err == nil {
		strData = string(decoded)
	}

	// 3. 按行分割
	var links []string
	for _, l := range strings.Split(strData, "\n") {
		l = strings.TrimSpace(l)
		// 针对常见分享链接打开头过滤
		if l != "" && (strings.HasPrefix(l, "vless://") || strings.HasPrefix(l, "vmess://") || strings.HasPrefix(l, "trojan://") || strings.HasPrefix(l, "ss://")) {
			links = append(links, l)
		}
	}
	return links
}

// ParseSubReads from subfile and returns the parsed outbounds
func ParseSub(filename string) ([]*OutboundObject, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	links := extractLinks(bytes)
	var outbounds []*OutboundObject

	for i, link := range links {
		u, err := url.Parse(link)
		if err != nil {
			fmt.Printf("链接 %d 解析失败: %v\n", i+1, err)
			continue
		}

		protocol := u.Scheme
		userID := u.User.Username()
		address := u.Hostname()
		portStr := u.Port()
		portInt, _ := strconv.ParseUint(portStr, 10, 16)

		tag := u.Fragment
		if tag == "" {
			tag = fmt.Sprintf("proxy-%d", i+1)
		} else {
			decodedTag, err := url.QueryUnescape(tag)
			if err == nil {
				tag = decodedTag
			}
		}

		outbound, err := CreateXrayOutbound(protocol, address, uint16(portInt), userID, tag, u.Query())
		if err != nil {
			fmt.Printf("链接 %d 生成 outbound 失败: %v\n", i+1, err)
			continue
		}

		// 解析 streamSettings
		q := u.Query()
		stream := &StreamSettings{
			Network:  q.Get("type"),
			Security: q.Get("security"),
		}
		if stream.Network == "" {
			stream.Network = "tcp"
		}

		switch stream.Network {
		case "ws":
			stream.WSSettings = &WSSettings{
				Path: q.Get("path"),
			}
			if host := q.Get("host"); host != "" {
				stream.WSSettings.Headers = map[string]string{"Host": host}
			}
		case "xhttp":
			stream.XHTTPSettings = &XHTTPSettings{
				Path: q.Get("path"),
				Mode: q.Get("mode"),
			}
			if host := q.Get("host"); host != "" {
				stream.XHTTPSettings.Header = map[string]string{"Host": host}
			}
		case "grpc":
			stream.GRPCSettings = &GRPCSettings{
				ServiceName: q.Get("serviceName"),
			}
			if q.Get("mode") == "multi" {
				stream.GRPCSettings.MultiMode = true
			}
		case "tcp":
			if q.Get("headerType") == "http" {
				stream.TCPSettings = &TCPSettings{
					Header: &TCPHeader{
						Type: "http",
						Request: map[string]interface{}{
							"path": []string{q.Get("path")},
							"headers": map[string][]string{
								"Host": {q.Get("host")},
							},
						},
					},
				}
			}
		}

		switch stream.Security {
		case "tls":
			stream.TLSSettings = &TLSSettings{
				ServerName:  q.Get("sni"),
				Fingerprint: q.Get("fp"),
			}
			if alpn := q.Get("alpn"); alpn != "" {
				stream.TLSSettings.Alpn = []string{alpn}
			}
		case "reality":
			stream.RealitySettings = &RealitySettings{
				ServerName:  q.Get("sni"),
				Fingerprint: q.Get("fp"),
				PublicKey:   q.Get("pbk"),
				ShortId:     q.Get("sid"),
				SpiderX:     q.Get("spx"),
			}
		}

		outbound.StreamSettings = stream
		outbounds = append(outbounds, outbound)
	}
	return outbounds, nil
}
