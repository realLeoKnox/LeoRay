package main

import (
	"fmt"
	"net/url"
)

// OutboundObject 表示 Xray 的出站配置外层结构
type OutboundObject struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       interface{}     `json:"settings"`
	StreamSettings *StreamSettings `json:"streamSettings,omitempty"`
}

// VnextSettings 适用于 vmess 和 vless 协议的 settings
type VnextSettings struct {
	Vnext []VnextServer `json:"vnext"`
}

type VnextServer struct {
	Address string      `json:"address"`
	Port    uint16      `json:"port"`
	Users   []VnextUser `json:"users"`
}

type VnextUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption,omitempty"` // VLESS 协议通常需要这行配置
	Flow       string `json:"flow,omitempty"`       // XTLS / Reality 特性需要
}

// ServerSettings 适用于 trojan 和 shadowsocks 协议的 settings
type ServerSettings struct {
	Servers []Server `json:"servers"`
}

type Server struct {
	Address  string `json:"address"`
	Port     uint16 `json:"port"`
	Password string `json:"password"`
	Method   string `json:"method,omitempty"` // Shadowsocks 通常会用到 Method 这里保留字段扩展性
}

// CreateXrayOutbound 根据传入的参数生成 Xray出站对象
func CreateXrayOutbound(protocol string, address string, port uint16, userID string, tag string, query url.Values) (*OutboundObject, error) {
	if query == nil {
		query = url.Values{}
	}
	if tag == "" {
		tag = "proxy"
	}

	outbound := &OutboundObject{
		Tag:      tag,
		Protocol: protocol,
	}

	switch protocol {
	case "vless", "vmess":
		user := VnextUser{
			ID: userID,
		}
		if protocol == "vless" {
			user.Encryption = "none"
			if enc := query.Get("encryption"); enc != "" {
				user.Encryption = enc
			}
			if flow := query.Get("flow"); flow != "" {
				user.Flow = flow
			}
		}

		outbound.Settings = VnextSettings{
			Vnext: []VnextServer{
				{
					Address: address,
					Port:    port,
					Users:   []VnextUser{user},
				},
			},
		}

	case "trojan", "shadowsocks":
		outbound.Settings = ServerSettings{
			Servers: []Server{
				{
					Address:  address,
					Port:     port,
					Password: userID,
				},
			},
		}

	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return outbound, nil
}

// ----------- StreamSettings Structures -----------
type StreamSettings struct {
	Network         string           `json:"network,omitempty"`
	Security        string           `json:"security,omitempty"`
	Sockopt         *SockoptSettings `json:"sockopt,omitempty"`
	TLSSettings     *TLSSettings     `json:"tlsSettings,omitempty"`
	RealitySettings *RealitySettings `json:"realitySettings,omitempty"`
	TCPSettings     *TCPSettings     `json:"tcpSettings,omitempty"`
	WSSettings      *WSSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *GRPCSettings    `json:"grpcSettings,omitempty"`
	XHTTPSettings   *XHTTPSettings   `json:"xhttpSettings,omitempty"`
}

type SockoptSettings struct {
	Interface string `json:"interface,omitempty"`
}

type TLSSettings struct {
	ServerName  string   `json:"serverName,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Alpn        []string `json:"alpn,omitempty"`
}

type RealitySettings struct {
	ServerName  string `json:"serverName,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	ShortId     string `json:"shortId,omitempty"`
	SpiderX     string `json:"spiderX,omitempty"`
}

type TCPSettings struct {
	Header *TCPHeader `json:"header,omitempty"`
}

type TCPHeader struct {
	Type    string      `json:"type,omitempty"`
	Request interface{} `json:"request,omitempty"` // map[string]interface{}
}

type WSSettings struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type GRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

type XHTTPSettings struct {
	Path   string            `json:"path,omitempty"`
	Mode   string            `json:"mode,omitempty"`
	Header map[string]string `json:"header,omitempty"`
}


