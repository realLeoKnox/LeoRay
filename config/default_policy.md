[Proxy Group]

兜底 = select,ALL_Filter
AI = select,US_Filter,JP_Filter,KR_Filter,SG_Filter,TW_Filter,EU_Filter
常用论坛 = select,ALL_Filter
Leo = select,DIRECT,ALL_Filter
谷歌服务 = select,ALL_Filter
微软服务 = select,DIRECT,ALL_Filter
PayPal = select,US_Filter
Spotify = select,ALL_Filter
Telegram = select,ALL_Filter
TikTok = select,ALL_Filter
X【Twitter】 = select,ALL_Filter
流媒体 = select,ALL_Filter
Apple = select,DIRECT,ALL_Filter
Speedtest = select,DIRECT,ALL_Filter
游戏平台 = select,DIRECT,ALL_Filter
Direct = select,DIRECT,ALL_Filter
BiliBili = select,DIRECT,ALL_Filter
抖音 = select,DIRECT,ALL_Filter
内网段【默认绕过】 = select,DIRECT,ALL_Filter

[Rule]
DOMAIN-KEYWORD,wechat,Direct
DOMAIN-KEYWORD,weixin,Direct
FINAL,兜底
#Type:DOMAIN-SUFFIX,DOMAIN,DOMAIN-KEYWORD,USER-AGENT,URL-REGEX,IP-CIDR
#Strategy:DIRECT,PROXY,REJECT
#Options:no-resolve(only for IP-CIDR,IP-CIDR2,GEOIP,IP-ASN)


[Remote Rule]
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Adblock.list, policy=REJECT-DROP, tag=拦截广告跟踪器, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/AI.list, policy=AI, tag=主流AI服务, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Apple.list, policy=Apple, tag=Apple服务, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/BiliBili.list, policy=BiliBili, tag=B站, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Direct.list, policy=Direct, tag=直连规则, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/DouYin.list, policy=抖音, tag=抖音分流, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Game.list, policy=游戏平台, tag=主流游戏平台, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Google.list, policy=谷歌服务, tag=谷歌服务, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/leosweb.list, policy=Leo, tag=Leo的自有服务, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Microsoft.list, policy=微软服务, tag=微软服务, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/nsweb.list, policy=常用论坛, tag=一些常用论坛, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/PayPal.list, policy=PayPal, tag=PayPal, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/special.list, policy=内网段【默认绕过】, tag=内网段, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Speedtest.list, policy=Speedtest, tag=测速分流, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Spotify.list, policy=Spotify, tag=Spotify, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Telegram.list, policy=Telegram, tag=Telegram, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/TikTok.list, policy=TikTok, tag=TikTok, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Twitter.list, policy=X【Twitter】, enabled=true
https://raw.githubusercontent.com/DonaldKing2022/Proxy_rule/refs/heads/main/rule/Video.list, policy=流媒体, tag=流媒体, enabled=true
