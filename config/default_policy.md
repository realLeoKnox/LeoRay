# LeoRay 默认策略参考

本文件仅作参考，系统默认策略组已硬编码在 Go 后端中。

## 默认 GEO 策略组

| 策略组 | GEO 规则 | 说明 |
|--------|----------|------|
| Final | (catch-all) | 所有未匹配规则流量指向 |
| Google | geosite:google | 谷歌服务 |
| DIRECT | geosite:cn | 中国大陆直连 |
| Microsoft | geosite:microsoft | 微软服务 |
| COIN | geosite:category-cryptocurrency | 加密货币 |
| AI | geosite:category-ai-!cn | 中国大陆以外的 AI 服务 |

## 可用 GEO 规则

常用 GeoSite:
- geosite:google
- geosite:cn
- geosite:microsoft
- geosite:apple
- geosite:category-ai-!cn
- geosite:category-cryptocurrency
- geosite:telegram
- geosite:youtube
- geosite:netflix
- geosite:spotify
- geosite:twitter
- geosite:tiktok
- geosite:bilibili
- geosite:category-games

常用 GeoIP:
- geoip:cn
- geoip:private
- geoip:us
- geoip:jp
