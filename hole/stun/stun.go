package stun

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/stun"
)

// STUNClient STUN客户端
type STUNClient struct {
	STUNServers  []string
	ExternalIP   net.IP
	ExternalPort int
}

// NewSTUNClient 创建一个新的STUN客户端
func NewSTUNClient() *STUNClient {
	return &STUNClient{
		STUNServers: []string{
			// 中国/亚洲优化的STUN服务器
			"stun.miwifi.com:3478", // 小米
			"stun.qq.com:3478",     // 腾讯

			// 你提供的额外服务器 (选择部分高可用性服务器)
			"stun3.l.google.com:19302",
			"stun4.l.google.com:19302",
			"stun.gmx.net:3478",
			"stun.antisip.com:3478",
			"stun.bluesip.net:3478",
			"stun.dus.net:3478",
			"stun.epygi.com:3478",
			"stun.sip.us:3478",
			"stun.uls.co.za:3478",
			"stun.voys.nl:3478",
			"stun.faktortel.com.au:3478",
			"stun.freecall.com:3478",
			"stun.freeswitch.org:3478",
			"stun.ipfire.org:3478",
			"stun.mit.de:3478",
			"stun.linphone.org:3478",
			"stun.services.mozilla.com:3478",
			"stunserver.org:3478",

			// 以下是为中国网络优化的STUN服务器，排在靠前位置提高成功率
			"stun.chat.bilibili.com:3478", // 哔哩哔哩
			"stun.voip.aebc.com:3478",     // 亚太地区
			"stun.mitake.com.tw:3478",     // 台湾区域
			"stun.zoiper.com:3478",        // 全球分布，亚太地区连接较好
		},
	}
}

// DiscoverExternalAddress 发现外部地址
func (s *STUNClient) DiscoverExternalAddress(localPort int) error {
	var lastErr error

	for _, serverAddr := range s.STUNServers {
		err := s.trySTUNServer(serverAddr, localPort)
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("STUN服务器 %s 失败: %v, 尝试下一个...", serverAddr, err)
	}

	return fmt.Errorf("所有STUN服务器都失败, 最后错误: %v", lastErr)
}

// 尝试单个STUN服务器
func (s *STUNClient) trySTUNServer(serverAddr string, localPort int) error {
	// 创建一个UDP连接
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: localPort})
	if err != nil {
		return fmt.Errorf("无法监听UDP端口 %d: %v", localPort, err)
	}
	defer conn.Close()

	// 设置读取超时
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		return fmt.Errorf("设置读取超时失败: %v", err)
	}

	// 创建STUN消息
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	// 解析STUN服务器地址
	serverUDPAddr, err := net.ResolveUDPAddr("udp4", serverAddr)
	if err != nil {
		return fmt.Errorf("解析STUN服务器地址失败: %v", err)
	}

	// 发送STUN请求
	_, err = conn.WriteToUDP(message.Raw, serverUDPAddr)
	if err != nil {
		return fmt.Errorf("发送STUN请求失败: %v", err)
	}

	// 接收缓冲区
	buffer := make([]byte, 1024)

	// 接收STUN响应
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return fmt.Errorf("接收STUN响应失败: %v", err)
	}

	// 解析STUN消息
	// 正确使用Decode方法
	message = &stun.Message{Raw: buffer[:n]}
	if err := message.Decode(); err != nil {
		return fmt.Errorf("解析STUN消息失败: %v", err)
	}

	// 获取XOR-MAPPED-ADDRESS属性
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(message); err != nil {
		return fmt.Errorf("从STUN响应中获取XOR-MAPPED-ADDRESS失败: %v", err)
	}

	s.ExternalIP = xorAddr.IP
	s.ExternalPort = xorAddr.Port

	log.Printf("STUN发现的外部地址: %s:%d", s.ExternalIP.String(), s.ExternalPort)
	return nil
}
