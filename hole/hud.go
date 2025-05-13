package hole

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"magnet-search/hole/nat"
	"magnet-search/hole/stun"
)

// PeerInfo 存储对等点信息
type PeerInfo struct {
	ID       string
	IP       net.IP
	Port     int
	LastSeen time.Time
}

// HolePuncher 处理UDP打洞
type HolePuncher struct {
	LocalPort        int
	Conn             *net.UDPConn
	NAT              *nat.NATTraversal
	STUN             *stun.STUNClient
	KnownPeers       map[string]*PeerInfo
	OnPeerDiscovered func(peer *PeerInfo)
}

// NewHolePuncher 创建一个新的打洞器
func NewHolePuncher(localPort int) *HolePuncher {
	return &HolePuncher{
		LocalPort:  localPort,
		KnownPeers: make(map[string]*PeerInfo),
	}
}

// Start 启动打洞器
func (h *HolePuncher) Start(ctx context.Context) error {
	// 设置NAT穿透
	h.NAT = nat.NewNATTraversal()
	h.NAT.AddPortMapping("UDP", h.LocalPort, h.LocalPort, "DHT App")

	if err := h.NAT.Setup(ctx); err != nil {
		log.Printf("NAT穿透设置失败: %v, 但仍会继续...", err)
	}

	// 发现外部地址
	h.STUN = stun.NewSTUNClient()
	if err := h.STUN.DiscoverExternalAddress(h.LocalPort); err != nil {
		log.Printf("STUN发现失败: %v, 但仍会继续...", err)
	}

	// 创建UDP监听
	var err error
	h.Conn, err = net.ListenUDP("udp4", &net.UDPAddr{Port: h.LocalPort})
	if err != nil {
		return fmt.Errorf("UDP监听失败: %v", err)
	}

	// 启动接收循环
	go h.receiveLoop(ctx)

	// 启动NAT刷新循环
	go h.refreshLoop(ctx)

	return nil
}

// 接收循环
func (h *HolePuncher) receiveLoop(ctx context.Context) {
	buffer := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 设置读取超时以便定期检查ctx.Done()
			h.Conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, addr, err := h.Conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 超时，继续循环
					continue
				}
				log.Printf("UDP读取错误: %v", err)
				continue
			}

			// 处理接收到的数据
			h.handlePacket(buffer[:n], addr)
		}
	}
}

// 处理接收到的数据包
func (h *HolePuncher) handlePacket(data []byte, addr *net.UDPAddr) {
	// 这里应该实现你的协议解析逻辑
	// 简单示例: 期望"HELLO:{PEER_ID}"格式的消息

	// 假设前5个字节是"HELLO"，然后是一个冒号，后面是对等点ID
	if len(data) < 7 || string(data[:5]) != "HELLO" || data[5] != ':' {
		return
	}

	peerID := string(data[6:])

	// 记录或更新对等点
	peer, exists := h.KnownPeers[peerID]
	if !exists {
		peer = &PeerInfo{
			ID:   peerID,
			IP:   addr.IP,
			Port: addr.Port,
		}
		h.KnownPeers[peerID] = peer

		if h.OnPeerDiscovered != nil {
			h.OnPeerDiscovered(peer)
		}
	}

	peer.LastSeen = time.Now()

	// 回复打洞尝试
	h.sendHolePunchingPacket(addr, peerID)
}

// 发送打洞尝试包
func (h *HolePuncher) sendHolePunchingPacket(addr *net.UDPAddr, peerID string) {
	// 发送"PUNCH:{OUR_ID}"格式的消息
	message := fmt.Sprintf("PUNCH:%s", "our_node_id_here")
	_, err := h.Conn.WriteToUDP([]byte(message), addr)
	if err != nil {
		log.Printf("发送打洞包失败: %v", err)
	}
}

// 主动尝试连接一个对等点
func (h *HolePuncher) ConnectToPeer(ip net.IP, port int, peerID string) {
	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	// 发送打洞包
	message := fmt.Sprintf("HELLO:%s", "our_node_id_here")

	// 发送多个打洞包，增加成功率
	for i := 0; i < 5; i++ {
		_, err := h.Conn.WriteToUDP([]byte(message), addr)
		if err != nil {
			log.Printf("发送打洞包失败: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// 刷新循环 - 保持NAT映射活跃
func (h *HolePuncher) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if h.NAT != nil {
				if err := h.NAT.Refresh(ctx); err != nil {
					log.Printf("刷新NAT映射失败: %v", err)
				}
			}
		}
	}
}

// Stop 停止打洞器
func (h *HolePuncher) Stop(ctx context.Context) {
	if h.Conn != nil {
		h.Conn.Close()
	}

	if h.NAT != nil {
		h.NAT.Shutdown(ctx)
	}
}
