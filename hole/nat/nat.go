package nat

import (
	"context"
	"fmt"
	"github.com/huin/goupnp/dcps/internetgateway2"
	nat_pmp "github.com/jackpal/go-nat-pmp"
	"log"
	"magnet-search/hole/gateway"
	"net"
)

// PortMapping 表示一个端口映射
type PortMapping struct {
	Protocol string // "UDP" 或 "TCP"
	ExtPort  int    // 外部端口
	IntPort  int    // 内部端口
	Desc     string // 描述
}

// NATTraversal 处理NAT穿透
type NATTraversal struct {
	Mappings   []PortMapping
	upnpClient *internetgateway2.WANIPConnection1
	pmpClient  *nat_pmp.Client
	useUPnP    bool
	localIP    net.IP
}

// NewNATTraversal 创建一个新的NAT穿透实例
func NewNATTraversal() *NATTraversal {
	return &NATTraversal{
		Mappings: make([]PortMapping, 0),
	}
}

// AddPortMapping 添加一个端口映射
func (n *NATTraversal) AddPortMapping(protocol string, extPort, intPort int, desc string) {
	n.Mappings = append(n.Mappings, PortMapping{
		Protocol: protocol,
		ExtPort:  extPort,
		IntPort:  intPort,
		Desc:     desc,
	})
}

// 获取本地IP地址
func getLocalIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP, nil
			}
		}
	}

	return nil, fmt.Errorf("无法找到非回环IPv4地址")
}

// Setup 设置NAT穿透
func (n *NATTraversal) Setup(ctx context.Context) error {
	var err error
	n.localIP, err = getLocalIP()
	if err != nil {
		return err
	}

	log.Printf("本地IP地址: %s", n.localIP.String())

	// 首先尝试UPnP
	upnpErr := n.setupUPnP(ctx)
	if upnpErr == nil {
		n.useUPnP = true
		log.Println("成功使用UPnP设置NAT穿透")
		return nil
	}

	log.Printf("UPnP设置失败: %v, 尝试NAT-PMP...", upnpErr)

	// 如果UPnP失败，尝试NAT-PMP
	pmpErr := n.setupNATPMP()
	if pmpErr == nil {
		n.useUPnP = false
		log.Println("成功使用NAT-PMP设置NAT穿透")
		return nil
	}

	log.Printf("NAT-PMP设置也失败: %v", pmpErr)
	return fmt.Errorf("所有NAT穿透方法都失败: UPnP: %v, NAT-PMP: %v", upnpErr, pmpErr)
}

// 设置UPnP
func (n *NATTraversal) setupUPnP(ctx context.Context) error {
	clients, _, err := internetgateway2.NewWANIPConnection1Clients()
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("没有找到UPnP设备")
	}

	n.upnpClient = clients[0]

	// 测试映射一个端口
	return n.applyUPnPMappings(ctx)
}

// 应用UPnP映射
func (n *NATTraversal) applyUPnPMappings(ctx context.Context) error {
	for _, mapping := range n.Mappings {
		err := n.upnpClient.AddPortMapping(
			"",                      // 远程主机（留空表示任何主机）
			uint16(mapping.ExtPort), // 外部端口
			mapping.Protocol,        // 协议
			uint16(mapping.IntPort), // 内部端口
			n.localIP.String(),      // 内部客户端
			true,                    // 是否启用
			mapping.Desc,            // 描述
			86400,                   // 租约时间（秒）
		)
		if err != nil {
			return err
		}
		log.Printf("成功映射 %s 端口 %d -> %d (%s)",
			mapping.Protocol, mapping.ExtPort, mapping.IntPort, mapping.Desc)
	}
	return nil
}

// 设置NAT-PMP
func (n *NATTraversal) setupNATPMP() error {
	// 获取默认网关
	gateway, err := getDefaultGateway()
	if err != nil {
		return err
	}

	n.pmpClient = nat_pmp.NewClient(gateway)

	// 获取外部IP地址
	resp, err := n.pmpClient.GetExternalAddress()
	if err != nil {
		return err
	}

	// 使用正确的方式处理外部IP
	externalIP := net.IPv4(
		resp.ExternalIPAddress[0],
		resp.ExternalIPAddress[1],
		resp.ExternalIPAddress[2],
		resp.ExternalIPAddress[3],
	)

	log.Printf("NAT-PMP 外部IP地址: %s", externalIP.String())

	// 应用端口映射
	return n.applyNATPMPMappings()
}

// 应用NAT-PMP映射
func (n *NATTraversal) applyNATPMPMappings() error {
	for _, mapping := range n.Mappings {
		// 使用uint8替代byte，并使用硬编码的协议值
		var protocol string
		if mapping.Protocol == "TCP" {
			protocol = "tcp" // TCP在NAT-PMP中的值
		} else {
			protocol = "udp" // UDP在NAT-PMP中的值
		}

		// 添加映射
		resp, err := n.pmpClient.AddPortMapping(
			protocol,
			mapping.IntPort,
			mapping.ExtPort,
			3600, // 租约时间（秒）
		)
		if err != nil {
			return err
		}

		log.Printf("成功添加NAT-PMP映射 %s: %d -> %d, 有效期至 %d秒",
			mapping.Protocol, mapping.IntPort, int(resp.MappedExternalPort), 3600)
	}
	return nil
}

// 获取默认网关IP
func getDefaultGateway() (net.IP, error) {
	gateway_ip, err := gateway.GetDefaultGateway()
	if err != nil {
		log.Fatalf("获取网关失败: %v", err)
	}

	fmt.Printf("检测到默认网关: %s\n", gateway_ip.String())

	// 获取所有可能的网关
	allGateways := gateway.GetAllPossibleGateways()
	fmt.Println("\n可能的网关列表:")

	for i, gw := range allGateways {
		isGateway := gateway.CheckIfGateway(gw)
		status := "❌ 可能不是网关"
		if isGateway {
			status = "✅ 可能是网关"
		}
		fmt.Printf("%d. %s  %s\n", i+1, gw.String(), status)
	}
	return gateway_ip, nil
}

// Refresh 刷新端口映射
func (n *NATTraversal) Refresh(ctx context.Context) error {
	if n.useUPnP {
		return n.applyUPnPMappings(ctx)
	}
	return n.applyNATPMPMappings()
}

// Shutdown 关闭所有端口映射
func (n *NATTraversal) Shutdown(ctx context.Context) {
	if n.useUPnP && n.upnpClient != nil {
		for _, mapping := range n.Mappings {
			err := n.upnpClient.DeletePortMapping(
				"",
				uint16(mapping.ExtPort),
				mapping.Protocol,
			)
			if err != nil {
				log.Printf("删除UPnP映射失败 %s:%d: %v",
					mapping.Protocol, mapping.ExtPort, err)
			} else {
				log.Printf("成功删除UPnP映射 %s:%d",
					mapping.Protocol, mapping.ExtPort)
			}
		}
	} else if n.pmpClient != nil {
		for _, mapping := range n.Mappings {
			var protocol string
			if mapping.Protocol == "TCP" {
				protocol = "tcp" // TCP在NAT-PMP中的值
			} else {
				protocol = "udp" // UDP在NAT-PMP中的值
			}

			// 删除映射（设置0秒租约）
			// 仔细检查NAT-PMP库中AddPortMapping的函数签名
			_, err := n.pmpClient.AddPortMapping(
				protocol, // 使用uint8类型的protocol
				mapping.IntPort,
				0, // 外部端口0，请求删除
				0, // 租约时间0秒
			)
			if err != nil {
				log.Printf("删除NAT-PMP映射失败 %s:%d: %v",
					mapping.Protocol, mapping.IntPort, err)
			} else {
				log.Printf("成功删除NAT-PMP映射 %s:%d",
					mapping.Protocol, mapping.IntPort)
			}
		}
	}
}
