package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"magnet-search/dht"
	"magnet-search/hole/nat"
	"magnet-search/hole/stun"
)

type NATTestResult struct {
	ExternalIP      string
	ExternalPort    int
	NATType         string
	PortMappingOK   bool
	DHTPeersFound   bool
	DHTPeersCount   int
	TimeToFirstPeer time.Duration
}

func main() {
	// 设置日志
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("DHT NAT穿透验证程序启动...")

	// 创建结果对象
	result := &NATTestResult{}

	// 1. 执行NAT类型检测
	log.Println("1. 检测NAT类型...")
	natType, err := detectNATType()
	if err != nil {
		log.Printf("  NAT类型检测失败: %v", err)
	} else {
		log.Printf("  NAT类型: %s", natType)
		result.NATType = natType
	}

	// 2. 测试端口映射
	log.Println("2. 测试端口映射...")
	mappingOK, extIP, extPort := testPortMapping(6881)
	result.PortMappingOK = mappingOK
	result.ExternalIP = extIP
	result.ExternalPort = extPort

	// 3. 测试DHT连通性
	log.Println("3. 测试DHT网络连通性...")
	peersFound, peerCount, timeToFirst := testDHTConnectivity(result.ExternalIP, result.ExternalPort)
	result.DHTPeersFound = peersFound
	result.DHTPeersCount = peerCount
	result.TimeToFirstPeer = timeToFirst

	// 4. 输出综合报告
	printTestReport(result)

	// 5. 启动简单的HTTP服务，显示详细信息
	startReportServer(result)
}

// 检测NAT类型
func detectNATType() (string, error) {
	stunClient := stun.NewSTUNClient()

	// 测试端口1
	err1 := stunClient.DiscoverExternalAddress(6881)
	if err1 != nil {
		return "", err1
	}
	ip1 := stunClient.ExternalIP.String()
	port1 := stunClient.ExternalPort

	// 测试端口2 (不同的本地端口)
	err2 := stunClient.DiscoverExternalAddress(6882)
	if err2 != nil {
		return "端口受限型NAT", nil
	}
	ip2 := stunClient.ExternalIP.String()
	port2 := stunClient.ExternalPort

	if ip1 == ip2 {
		if port1 == 6881 && port2 == 6882 {
			return "完全锥形NAT (最佳)", nil
		} else if port1 != port1 && port2 != 6882 {
			return "端口受限锥形NAT (较好)", nil
		} else {
			return "地址受限锥形NAT (良好)", nil
		}
	} else {
		return "对称型NAT (最差，可能无法穿透)", nil
	}
}

// 测试端口映射
func testPortMapping(port int) (bool, string, int) {
	log.Printf("  尝试为端口 %d 创建映射...", port)

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建NAT穿透对象
	natTraversal := nat.NewNATTraversal()
	natTraversal.AddPortMapping("UDP", port, port, "DHT测试")

	// 设置NAT穿透
	err := natTraversal.Setup(ctx)
	if err != nil {
		log.Printf("  NAT穿透设置失败: %v", err)
	}

	// 使用STUN检测外部地址
	stunClient := stun.NewSTUNClient()
	err = stunClient.DiscoverExternalAddress(port)
	if err != nil {
		log.Printf("  STUN探测失败: %v", err)
		return false, "未知", 0
	}

	extIP := stunClient.ExternalIP.String()
	extPort := stunClient.ExternalPort

	log.Printf("  外部地址: %s:%d", extIP, extPort)

	// 验证端口映射是否如预期
	if extPort == port {
		log.Printf("  ✅ 端口映射成功 (保留了相同端口)")
		return true, extIP, extPort
	} else if extPort > 0 {
		log.Printf("  ⚠️ 端口映射变更: 内部=%d, 外部=%d", port, extPort)
		return true, extIP, extPort
	}

	log.Println("  ❌ 端口映射失败")
	return false, extIP, extPort
}

// 测试DHT连通性
func testDHTConnectivity(extIP string, extPort int) (bool, int, time.Duration) {
	// 流行的Ubuntu种子InfoHash

	// 热门种子的InfoHash列表
	var popularInfoHashes = []string{
		"08ada5a7a6183aae1e09d831df6748d566095a10", // Sintel

		"dd5600473d34ffa3bdbe0f25800ec3981752ac60", // Ubuntu 25.04
	}

	// 记录找到的对等点数量
	peerCount := 0
	foundPeers := false
	startTime := time.Now()
	var timeToFirst time.Duration

	// 创建DHT节点
	config := dht.NewStandardConfig()
	config.Address = ":6881"

	// 设置回调
	config.OnGetPeersResponse = func(infoHash string, peer *dht.Peer) {
		if !foundPeers {
			timeToFirst = time.Since(startTime)
			foundPeers = true
		}
		peerCount++
		log.Printf("  找到对等点 #%d: %s:%d", peerCount, peer.IP, peer.Port)
	}

	// 创建DHT
	d := dht.New(config)

	// 启动DHT
	go d.Run()

	// 等待DHT准备就绪
	log.Println("  DHT启动中，等待30秒让路由表填充...")
	time.Sleep(30 * time.Second)

	for _, testInfoHash := range popularInfoHashes {
		// 执行GetPeers请求
		log.Printf("  发送GetPeers请求，InfoHash: %s", testInfoHash)
		err := d.GetPeers(testInfoHash)
		if err != nil {
			log.Printf("  GetPeers失败: %v", err)
			return false, 0, 0
		}
	}

	// 等待结果
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-ticker.C:
			if foundPeers {
				log.Printf("  已找到 %d 个对等点", peerCount)
			} else {
				log.Println("  仍在搜索对等点...")
			}

			// 如果找到足够多的对等点，提前结束
			if peerCount >= 6 {
				log.Println("  ✅ 找到足够多的对等点，测试成功")
				return true, peerCount, timeToFirst
			}

		case <-timeout:
			if foundPeers {
				log.Printf("  ✅ 测试完成，找到 %d 个对等点", peerCount)
				return true, peerCount, timeToFirst
			} else {
				log.Println("  ❌ 测试超时，未找到任何对等点")
				return false, 0, 0
			}
		}
	}
}

// 输出测试报告
func printTestReport(result *NATTestResult) {
	fmt.Println("\n--------- DHT NAT穿透测试报告 ---------")
	fmt.Printf("外部IP地址:      %s\n", result.ExternalIP)
	fmt.Printf("外部端口:        %d\n", result.ExternalPort)
	fmt.Printf("NAT类型:         %s\n", result.NATType)
	fmt.Printf("端口映射:        %s\n", boolToStatus(result.PortMappingOK))
	fmt.Printf("DHT对等点:       %s (%d个)\n", boolToStatus(result.DHTPeersFound), result.DHTPeersCount)

	if result.DHTPeersFound {
		fmt.Printf("首个对等点响应:  %.2f秒\n", result.TimeToFirstPeer.Seconds())
	}

	fmt.Println("\n综合评估:")

	if result.DHTPeersFound && result.PortMappingOK {
		fmt.Println("✅ DHT NAT穿透测试成功！节点可以正常参与DHT网络。")
	} else if result.PortMappingOK && !result.DHTPeersFound {
		fmt.Println("⚠️ 端口映射成功，但未找到DHT对等点。可能需要更长时间加入网络。")
	} else if !result.PortMappingOK {
		fmt.Println("❌ NAT穿透失败。节点可能无法正常参与DHT网络。")
	}

	fmt.Println("---------------------------------------")
}

// 辅助函数：布尔值转状态文本
func boolToStatus(ok bool) string {
	if ok {
		return "✅ 成功"
	}
	return "❌ 失败"
}

// 启动报告HTTP服务器
func startReportServer(result *NATTestResult) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		html := `
        <!DOCTYPE html>
        <html>
        <head>
    		<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
            <title>DHT NAT穿透测试报告</title>
            <style>
                body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
                h1 { color: #333; }
                .success { color: green; }
                .warning { color: orange; }
                .error { color: red; }
                .report { background: #f5f5f5; padding: 20px; border-radius: 5px; }
                table { width: 100%; border-collapse: collapse; }
                td, th { padding: 8px; border-bottom: 1px solid #ddd; }
                th { text-align: left; }
            </style>
        </head>
        <body>
            <h1>DHT NAT穿透测试报告</h1>
            <div class="report">
                <table>
                    <tr>
                        <th>外部IP地址</th>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <th>外部端口</th>
                        <td>%d</td>
                    </tr>
                    <tr>
                        <th>NAT类型</th>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <th>端口映射</th>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <th>DHT对等点</th>
                        <td>%s (%d个)</td>
                    </tr>
        `

		if result.DHTPeersFound {
			html += fmt.Sprintf(`
                    <tr>
                        <th>首个对等点响应</th>
                        <td>%.2f秒</td>
                    </tr>
            `, result.TimeToFirstPeer.Seconds())
		}

		html += `
                </table>
                
                <h2>综合评估</h2>
                <p class="%s">
        `

		var assessment string
		var cssClass string

		if result.DHTPeersFound && result.PortMappingOK {
			assessment = "✅ DHT NAT穿透测试成功！节点可以正常参与DHT网络。"
			cssClass = "success"
		} else if result.PortMappingOK && !result.DHTPeersFound {
			assessment = "⚠️ 端口映射成功，但未找到DHT对等点。可能需要更长时间加入网络。"
			cssClass = "warning"
		} else if !result.PortMappingOK {
			assessment = "❌ NAT穿透失败。节点可能无法正常参与DHT网络。"
			cssClass = "error"
		}

		html += assessment + `
                </p>
                
                <h2>NAT类型解释</h2>
                <ul>
                    <li><strong>完全锥形NAT</strong>: 最佳，任何外部主机都可以通过映射的端口联系内部主机</li>
                    <li><strong>地址受限锥形NAT</strong>: 良好，只有内部主机联系过的外部IP可以通过映射端口联系内部主机</li>
                    <li><strong>端口受限锥形NAT</strong>: 较好，只有内部主机联系过的特定IP:端口可以通过映射端口联系内部主机</li>
                    <li><strong>对称型NAT</strong>: 最差，为每个外部连接分配不同的映射端口，很难穿透</li>
                </ul>
                
                <h2>下一步</h2>
                <ul>
                    <li>如果测试失败，尝试在路由器上手动设置端口转发</li>
                    <li>检查防火墙设置是否阻止了UDP流量</li>
                    <li>尝试使用不同的端口（如6882、6889等）</li>
                    <li>如果使用VPN，可能需要关闭VPN或配置VPN允许UDP流量</li>
                </ul>
            </div>
            
            <p><small>测试时间: %s</small></p>
        </body>
        </html>
        `

		statusStr := boolToStatus(result.PortMappingOK)
		peerStatusStr := boolToStatus(result.DHTPeersFound)

		fmt.Fprintf(w, html,
			result.ExternalIP,
			result.ExternalPort,
			result.NATType,
			statusStr,
			peerStatusStr, result.DHTPeersCount,
			cssClass,
			time.Now().Format("2006-01-02 15:04:05"))
	})

	// 获取本地IP，以便用户知道在哪里访问报告
	localIP, err := getLocalIP()
	if err == nil {
		log.Printf("测试报告可在浏览器中访问: http://%s:8080", localIP)
	} else {
		log.Println("测试报告可在浏览器中访问: http://localhost:8080")
	}

	// 设置退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动HTTP服务器
	server := &http.Server{Addr: ":8080"}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP服务器错误: %v", err)
		}
	}()

	// 等待退出信号
	<-sigCh
	log.Println("收到退出信号，关闭服务...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭错误: %v", err)
	}
}

// 获取本地IP
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("无法找到合适的本地IP地址")
}
