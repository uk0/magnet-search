package test

import (
	"log"
	"magnet-search/dht"
	"magnet-search/hole/stun"
	"time"
)

func testNATPunchthrough() {
	log.Println("NAT穿透测试开始...")

	// 创建STUN客户端
	stunClient := stun.NewSTUNClient()

	// 使用DHT端口
	port := 6881 // 使用你DHT的端口

	// 探测外部IP和端口
	err := stunClient.DiscoverExternalAddress(port)
	if err != nil {
		log.Printf("STUN探测失败: %v", err)
		return
	}

	// 输出结果
	log.Printf("外部IP: %s, 端口: %d",
		stunClient.ExternalIP.String(),
		stunClient.ExternalPort)

	// 验证端口映射是否成功
	if stunClient.ExternalPort == port {
		log.Println("✅ 端口映射成功，使用了与内部相同的端口")
	} else {
		log.Printf("⚠️ 端口映射使用了不同的端口: 内部=%d, 外部=%d",
			port, stunClient.ExternalPort)
	}
}

func verifyDHTPublicConnectivity() {
	// 创建一个磁力链接用于测试
	// 选择一个流行的种子，比如Ubuntu系统镜像
	testInfoHash := "08ada5a7a6183aae1e09d831df6748d566095a10" // Ubuntu镜像示例哈希

	// 创建DHT节点
	config := dht.NewStandardConfig()
	config.Address = ":6881"

	// 配置回调
	foundPeers := false

	config.OnGetPeersResponse = func(infoHash string, peer *dht.Peer) {
		log.Printf("找到节点响应我们的请求: %s:%d", peer.IP, peer.Port)
		foundPeers = true
	}

	// 创建DHT
	d := dht.New(config)

	// 启动DHT (在新的goroutine中)
	go d.Run()

	// 等待DHT准备就绪
	time.Sleep(30 * time.Second)

	// 执行GetPeers请求
	log.Printf("发送GetPeers请求，InfoHash: %s", testInfoHash)
	err := d.GetPeers(testInfoHash)
	if err != nil {
		log.Printf("GetPeers失败: %v", err)
		return
	}

	// 等待回复
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			if foundPeers {
				log.Println("✅ DHT节点成功收到GetPeers响应，NAT穿透工作正常！")
				return
			}
			log.Println("仍在等待DHT响应...")

		case <-timeout:
			log.Println("⚠️ 超时，未收到GetPeers响应，NAT穿透可能失败")
			return
		}
	}
}
