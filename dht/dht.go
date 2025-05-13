package dht

import (
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"log"
	"magnet-search/hole/nat"
	"magnet-search/hole/stun"
	"math"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	// StandardMode follows the standard protocol
	StandardMode = iota
	// CrawlMode for crawling the dht network.
	CrawlMode
)

var (
	// ErrNotReady is the error when DHT is not initialized.
	ErrNotReady = errors.New("dht is not ready")
	// ErrOnGetPeersResponseNotSet is the error that config
	// OnGetPeersResponseNotSet is not set when call dht.GetPeers.
	ErrOnGetPeersResponseNotSet = errors.New("OnGetPeersResponse is not set")
)

// Config represents the configure of dht.
type Config struct {
	// in mainline dht, k = 8
	K int
	// for crawling mode, we put all nodes in one bucket, so KBucketSize may
	// not be K
	KBucketSize int
	// candidates are udp, udp4, udp6
	Network string
	// format is `ip:port`
	Address string
	// the prime nodes through which we can join in dht network
	PrimeNodes []string
	// the kbucket expired duration
	KBucketExpiredAfter time.Duration
	// the node expired duration
	NodeExpriedAfter time.Duration
	// how long it checks whether the bucket is expired
	CheckKBucketPeriod time.Duration
	// peer token expired duration
	TokenExpiredAfter time.Duration
	// the max transaction id
	MaxTransactionCursor uint64
	// how many nodes routing table can hold
	MaxNodes int
	// callback when got get_peers request
	OnGetPeers func(string, string, int)
	// callback when receive get_peers response
	OnGetPeersResponse func(string, *Peer)
	// callback when got announce_peer request
	OnAnnouncePeer func(string, string, int)
	// blcoked ips
	BlockedIPs []string
	// blacklist size
	BlackListMaxSize int
	// StandardMode or CrawlMode
	Mode int
	// the times it tries when send fails
	Try int
	// the size of packet need to be dealt with
	PacketJobLimit int
	// the size of packet handler
	PacketWorkerLimit int
	// the nodes num to be fresh in a kbucket
	RefreshNodeNum int
}

// NewStandardConfig returns a Config pointer with default values.
func NewStandardConfig() *Config {
	return &Config{
		K:           8,
		KBucketSize: 8,
		Network:     "udp4",
		Address:     ":6881",
		PrimeNodes: []string{
			"router.bittorrent.com:6881",
			"dht.transmissionbt.com:6881",
			"router.utorrent.com:6881",
			"dht.libtorrent.org:25401",
			"dht.aelitis.com:6881",
			"router.silotis.us:6881",    // 新增的活跃节点
			"dht.monitorrent.com:6881",  // 新增的活跃节点
			"dht.bootjav.com:6881",      // 亚洲区域活跃节点
			"router.bitcomet.com:6881",  // BitComet客户端DHT节点，国内使用较多
			"dht.cyberyon.com:6881",     // 亚太地区活跃节点
			"dht.bluedot.org:6881",      // 多区域连通性良好
			"router.magnets.im:6881",    // 磁力链相关资源节点
			"dht.eastasia.one:6881",     // 东亚优化节点
			"router.c-base.org:6881",    // 稳定的欧洲节点，但对中国连通性较好
			"dht.pacific-node.com:6881", // 太平洋区域节点，对亚洲友好
			"dht.acgtracker.com:6881",   // 动漫资源相关，亚洲连通性好
			"router.asiandht.com:6881",  // 亚洲优化节点
			// 特殊活跃种子的节点
			"87.98.162.88:6881",   // 一个已知活跃的节点
			"82.221.103.244:6881", // 一个已知活跃的节点
			"213.239.217.10:6881", // 一个已知活跃的节点
		},
		NodeExpriedAfter:     time.Duration(time.Minute * 15),
		KBucketExpiredAfter:  time.Duration(time.Minute * 15),
		CheckKBucketPeriod:   time.Duration(time.Second * 30),
		TokenExpiredAfter:    time.Duration(time.Minute * 10),
		MaxTransactionCursor: math.MaxUint32,
		MaxNodes:             5000,
		BlockedIPs:           make([]string, 0),
		BlackListMaxSize:     65536,
		Try:                  2,
		Mode:                 StandardMode,
		PacketJobLimit:       1024,
		PacketWorkerLimit:    256,
		RefreshNodeNum:       16,
	}
}

// NewCrawlConfig returns a config in crawling mode.
func NewCrawlConfig() *Config {
	config := NewStandardConfig()
	config.NodeExpriedAfter = 0
	config.KBucketExpiredAfter = 0
	config.CheckKBucketPeriod = time.Second * 5
	config.KBucketSize = math.MaxInt32
	config.Mode = CrawlMode
	config.RefreshNodeNum = 512

	return config
}

// DHT represents a DHT node.
type DHT struct {
	*Config
	node               *node
	conn               *net.UDPConn
	routingTable       *routingTable
	transactionManager *transactionManager
	peersManager       *peersManager
	tokenManager       *tokenManager
	blackList          *blackList
	Ready              bool
	packets            chan packet
	workerTokens       chan struct{}
	//NAT 穿透相关字段
	natTraversal *nat.NATTraversal
	externalIP   net.IP
	externalPort int

	// 节点监控相关字段
	bootNodesMutex     sync.RWMutex
	totalBootNodes     int                      // 总引导节点数量
	connectedBootNodes int                      // 成功连接的引导节点数量
	bootNodeStatus     map[string]bool          // 每个引导节点的连接状态
	bootNodeLatency    map[string]time.Duration // 每个引导节点的延迟

	// 对等点统计
	peerMutex        sync.RWMutex
	totalPeersFound  int                 // 总发现的对等点数量
	uniquePeersCount int                 // 唯一对等点数量
	activeInfoHashes map[string]int      // 每个infoHash对应的对等点数量
	uniquePeerMap    map[string]struct{} // 用于跟踪唯一对等点

	// 关闭通道
	closing chan struct{}
}

// initNAT 初始化NAT穿透
func (dht *DHT) initNAT(ctx context.Context) error {
	// 解析本地地址，获取端口
	_, portStr, err := net.SplitHostPort(dht.Address)
	if err != nil {
		return fmt.Errorf("解析地址错误: %v", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("端口格式错误: %v", err)
	}

	// 创建NAT穿透实例
	dht.natTraversal = nat.NewNATTraversal()

	// 添加UDP端口映射
	dht.natTraversal.AddPortMapping("UDP", port, port, "DHT Protocol")

	// 设置NAT穿透
	err = dht.natTraversal.Setup(ctx)
	if err != nil {
		log.Printf("NAT穿透设置失败: %v, 但仍将继续...", err)
	}

	// 使用STUN探测外部IP和端口
	stunClient := stun.NewSTUNClient()
	err = stunClient.DiscoverExternalAddress(port)
	if err != nil {
		log.Printf("STUN探测失败: %v, 但仍将继续...", err)
	} else {
		dht.externalIP = stunClient.ExternalIP
		dht.externalPort = stunClient.ExternalPort
		log.Printf("DHT节点的外部地址: %s:%d", dht.externalIP.String(), dht.externalPort)
	}

	return nil
}

// New returns a DHT pointer. If config is nil, then config will be set to
// the default config.
func New(config *Config) *DHT {
	if config == nil {
		config = NewStandardConfig()
	}

	node, err := newNode(randomString(20), config.Network, config.Address)
	if err != nil {
		panic(err)
	}

	d := &DHT{
		Config:           config,
		node:             node,
		blackList:        newBlackList(config.BlackListMaxSize),
		packets:          make(chan packet, config.PacketJobLimit),
		workerTokens:     make(chan struct{}, config.PacketWorkerLimit),
		bootNodeStatus:   make(map[string]bool),
		bootNodeLatency:  make(map[string]time.Duration),
		activeInfoHashes: make(map[string]int),
		uniquePeerMap:    make(map[string]struct{}),
		closing:          make(chan struct{}),
	}

	// 记录总引导节点数量
	d.totalBootNodes = len(config.PrimeNodes)

	for _, ip := range config.BlockedIPs {
		d.blackList.insert(ip, -1)
	}

	go func() {
		for _, ip := range getLocalIPs() {
			d.blackList.insert(ip, -1)
		}

		ip, err := getRemoteIP()
		if err != nil {
			d.blackList.insert(ip, -1)
		}
	}()

	return d
}

// IsStandardMode returns whether mode is StandardMode.
func (dht *DHT) IsStandardMode() bool {
	return dht.Mode == StandardMode
}

// IsCrawlMode returns whether mode is CrawlMode.
func (dht *DHT) IsCrawlMode() bool {
	return dht.Mode == CrawlMode
}

// 记录收到节点响应的方法
func (dht *DHT) recordNodeResponse(addr string) {
	dht.bootNodesMutex.Lock()
	defer dht.bootNodesMutex.Unlock()

	// 检查是否是引导节点
	if _, exists := dht.bootNodeStatus[addr]; exists {
		// 如果状态从失败变为成功，增加计数
		if !dht.bootNodeStatus[addr] {
			dht.connectedBootNodes++
			log.Printf("成功连接到DHT引导节点: %s", addr)
		}
		dht.bootNodeStatus[addr] = true
	}
}

// 获取引导节点统计信息
func (dht *DHT) GetBootNodeStats() (total, connected int, nodeStatus map[string]bool, latencies map[string]time.Duration) {
	dht.bootNodesMutex.RLock()
	defer dht.bootNodesMutex.RUnlock()

	// 创建副本，避免并发修改
	statusCopy := make(map[string]bool, len(dht.bootNodeStatus))
	latencyCopy := make(map[string]time.Duration, len(dht.bootNodeLatency))

	for node, status := range dht.bootNodeStatus {
		statusCopy[node] = status
	}

	for node, latency := range dht.bootNodeLatency {
		latencyCopy[node] = latency
	}

	return dht.totalBootNodes, dht.connectedBootNodes, statusCopy, latencyCopy
}

// 更新对等点统计信息
func (dht *DHT) updatePeerStats(infoHash string, peer *Peer) {
	dht.peerMutex.Lock()
	defer dht.peerMutex.Unlock()

	dht.totalPeersFound++

	// 更新此infoHash的对等点计数
	count, exists := dht.activeInfoHashes[infoHash]
	if !exists {
		dht.activeInfoHashes[infoHash] = 1
	} else {
		dht.activeInfoHashes[infoHash] = count + 1
	}

	// 跟踪唯一对等点
	peerKey := fmt.Sprintf("%s:%d", peer.IP.String(), peer.Port)
	if _, exists := dht.uniquePeerMap[peerKey]; !exists {
		dht.uniquePeerMap[peerKey] = struct{}{}
		dht.uniquePeersCount++
	}
}

// 获取对等点统计信息
func (dht *DHT) GetPeerStats() (totalFound, uniqueCount int, activeHashes map[string]int) {
	dht.peerMutex.RLock()
	defer dht.peerMutex.RUnlock()

	// 创建活跃infoHash的副本
	activeHashesCopy := make(map[string]int, len(dht.activeInfoHashes))
	for hash, count := range dht.activeInfoHashes {
		activeHashesCopy[hash] = count
	}

	return dht.totalPeersFound, dht.uniquePeersCount, activeHashesCopy
}

// 周期性输出DHT统计信息
func (dht *DHT) startStatsMonitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dht.printStats()
		case <-dht.closing:
			return
		}
	}
}

// 输出DHT统计信息
func (dht *DHT) printStats() {
	// 获取DHT节点统计
	total, connected, _, _ := dht.GetBootNodeStats()
	routingTableSize := dht.routingTable.Len()

	// 获取对等点统计
	totalPeers, uniquePeers, activeHashes := dht.GetPeerStats()

	// 当前活跃的infoHash数量
	activeInfoHashCount := len(activeHashes)

	// 输出统计信息
	log.Printf("DHT状态: 路由表=%d节点 | 引导节点=%d/%d (%.1f%%) | 发现对等点=%d | 唯一对等点=%d | 活跃资源=%d",
		routingTableSize,
		connected,
		total,
		float64(connected)/float64(total)*100,
		totalPeers,
		uniquePeers,
		activeInfoHashCount)

	// 如果活跃资源很少，可以列出它们
	if activeInfoHashCount > 0 && activeInfoHashCount <= 5 {
		log.Printf("活跃资源InfoHash:")
		for hash, count := range activeHashes {
			if len(hash) == 20 {
				hashHex := hex.EncodeToString([]byte(hash))
				log.Printf("  %s: %d个对等点", hashHex, count)
			}
		}
	}
}

// 初始化引导节点状态
func (dht *DHT) initBootNodeStatus() {
	// 初始化所有引导节点状态为未连接
	dht.bootNodesMutex.Lock()
	for _, addr := range dht.PrimeNodes {
		dht.bootNodeStatus[addr] = false
	}
	dht.bootNodesMutex.Unlock()
}

// init initializes global varables.
func (dht *DHT) init() {
	// 创建上下文，用于NAT穿透的生命周期管理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化NAT穿透
	err := dht.initNAT(ctx)
	if err != nil {
		log.Printf("NAT穿透初始化失败: %v", err)
		// 即使NAT穿透失败，我们仍然继续，以防在没有NAT的环境下
	}

	listener, err := net.ListenPacket(dht.Network, dht.Address)
	if err != nil {
		panic(err)
	}

	dht.conn = listener.(*net.UDPConn)
	dht.routingTable = newRoutingTable(dht.KBucketSize, dht)
	dht.peersManager = newPeersManager(dht)
	dht.tokenManager = newTokenManager(dht.TokenExpiredAfter, dht)
	dht.transactionManager = newTransactionManager(
		dht.MaxTransactionCursor, dht)

	// 初始化引导节点状态
	dht.initBootNodeStatus()

	go dht.transactionManager.run()
	go dht.tokenManager.clear()
	go dht.blackList.clear()

	// 启动统计监控
	go dht.startStatsMonitor()

	// 重新包装OnGetPeersResponse回调以收集统计信息
	originalCallback := dht.OnGetPeersResponse
	if originalCallback != nil {
		dht.OnGetPeersResponse = func(infoHash string, peer *Peer) {
			// 更新统计信息
			dht.updatePeerStats(infoHash, peer)
			// 调用原始回调
			originalCallback(infoHash, peer)
		}
	}
}

// join makes current node join the dht network.
func (dht *DHT) join() {
	// 如果我们有外部IP地址信息，添加到节点的地址信息中
	if dht.externalIP != nil && dht.externalPort > 0 {
		// 这里假设节点结构有合适的更新机制，否则可能需要修改节点结构
		log.Printf("使用外部地址 %s:%d 加入DHT网络", dht.externalIP.String(), dht.externalPort)
	}

	log.Printf("正在连接到 %d 个DHT引导节点...", dht.totalBootNodes)

	for _, addr := range dht.PrimeNodes {
		raddr, err := net.ResolveUDPAddr(dht.Network, addr)
		if err != nil {
			continue
		}

		// 发送find_node请求到引导节点
		dht.transactionManager.findNode(
			&node{addr: raddr},
			dht.node.id.RawString(),
		)
	}
}

// listen receives message from udp.
func (dht *DHT) listen() {
	go func() {
		buff := make([]byte, 8192)
		for {
			select {
			case <-dht.closing:
				return
			default:
				n, raddr, err := dht.conn.ReadFromUDP(buff)
				if err != nil {
					continue
				}

				dht.packets <- packet{buff[:n], raddr}
			}
		}
	}()
}

// id returns a id near to target if target is not null, otherwise it returns
// the dht's node id.
func (dht *DHT) id(target string) string {
	if dht.IsStandardMode() || target == "" {
		return dht.node.id.RawString()
	}
	return target[:15] + dht.node.id.RawString()[15:]
}

// GetPeers returns peers who have announced having infoHash.
func (dht *DHT) GetPeers(infoHash string) error {
	if !dht.Ready {
		return ErrNotReady
	}

	if dht.OnGetPeersResponse == nil {
		return ErrOnGetPeersResponseNotSet
	}

	if len(infoHash) == 40 {
		data, err := hex.DecodeString(infoHash)
		if err != nil {
			return err
		}
		infoHash = string(data)
	}

	neighbors := dht.routingTable.GetNeighbors(
		newBitmapFromString(infoHash), dht.routingTable.Len())

	for _, no := range neighbors {
		dht.transactionManager.getPeers(no, infoHash)
	}

	return nil
}

// Run starts the dht.
func (dht *DHT) Run() {
	// 创建上下文，用于NAT穿透的生命周期管理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dht.init()
	dht.listen()
	dht.join()

	dht.Ready = true

	// 开始监控后立即输出初始状态
	go func() {
		// 稍等片刻让初始连接有机会建立
		time.Sleep(3 * time.Second)
		dht.printStats()
	}()

	var pkt packet
	tick := time.Tick(dht.CheckKBucketPeriod)
	// 每10分钟刷新一次NAT映射，保持映射活跃
	natRefreshTick := time.Tick(10 * time.Minute)

	for {
		select {
		case pkt = <-dht.packets:
			// 当收到数据包时，可能需要更新引导节点状态
			if pkt.raddr != nil {
				addrStr := pkt.raddr.String()
				// 检查是否是来自引导节点的响应
				dht.recordNodeResponse(addrStr)
			}
			handle(dht, pkt)
		case <-tick:
			if dht.routingTable.Len() == 0 {
				dht.join()
			} else if dht.transactionManager.len() == 0 {
				go dht.routingTable.Fresh()
			}
		case <-natRefreshTick:
			// 刷新NAT映射
			if dht.natTraversal != nil {
				err := dht.natTraversal.Refresh(ctx)
				if err != nil {
					log.Printf("刷新NAT映射失败: %v", err)
				}
			}
		case <-dht.closing:
			return
		}
	}
}

// Stop stops the DHT and cleans up NAT mappings
func (dht *DHT) Stop() {
	// 通知所有协程关闭
	close(dht.closing)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭NAT映射
	if dht.natTraversal != nil {
		dht.natTraversal.Shutdown(ctx)
	}

	// 关闭UDP连接
	if dht.conn != nil {
		dht.conn.Close()
	}
	// 打印最终统计
	log.Println("DHT节点关闭, 最终统计:")
	dht.printStats()
}
