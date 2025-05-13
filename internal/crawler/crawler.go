package crawler

import (
	"encoding/hex"
	"fmt"
	"log"
	"magnet-search/internal/database"
	"magnet-search/internal/logger"
	"magnet-search/internal/models"
	"net/url"
	"strings"
	"sync"
	"time"

	"magnet-search/dht"
)

// Crawler 磁力链接爬虫管理器
type Crawler struct {
	db           *database.DB
	logger       *logger.Logger
	dhtCrawler   *dht.DHT
	dhtWire      *dht.Wire
	metadataChan chan *TorrentMetadata
	filter       *KeywordFilter
	running      bool
	wg           sync.WaitGroup
}

// NewCrawler 创建一个新的爬虫
func NewCrawler(db *database.DB, listenAddr string, metadataConcurrency int) (*Crawler, error) {
	// 创建日志记录器
	crawlerLogger, err := logger.NewLogger("logs")
	if err != nil {
		return nil, fmt.Errorf("创建日志记录器失败: %v", err)
	}

	// 创建元数据通道
	metadataChan := make(chan *TorrentMetadata, 100)

	// 创建过滤器并添加默认关键词
	filter := NewKeywordFilter()
	initDefaultKeywords(filter)

	// 创建 DHT Wire 组件，用于获取元数据
	// 参数: 下载缓冲区大小, 对等点数量限制, 每个 torrent 的并发下载数
	dhtWire := dht.NewWire(65536, 1024, metadataConcurrency)

	// 创建 DHT 爬虫配置
	dhtConfig := dht.NewCrawlConfig()
	dhtConfig.RefreshNodeNum = 512
	dhtConfig.CheckKBucketPeriod = time.Second * 30

	// 设置引导节点
	dhtConfig.PrimeNodes = []string{
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
	}

	// 解析监听地址获取端口
	host, port, err := parseListenAddr(listenAddr)
	if err != nil {
		return nil, fmt.Errorf("解析监听地址失败: %v", err)
	}

	// 设置DHT监听地址
	dhtConfig.Address = fmt.Sprintf("%s:%d", host, port)

	// 创建爬虫实例
	crawler := &Crawler{
		db:           db,
		logger:       crawlerLogger,
		dhtWire:      dhtWire,
		metadataChan: metadataChan,
		filter:       filter,
		running:      false,
	}

	// 设置 DHT 的回调函数
	dhtConfig.OnAnnouncePeer = func(infoHash, ip string, port int) {
		// 只有在爬虫运行中才请求元数据
		log.Println("发现对等点:", infoHash, "IP:", ip, "端口:", port)
		if crawler.running {
			log.Println("请求元数据:", infoHash, "IP:", ip, "端口:", port)
			// 请求获取元数据
			crawler.dhtWire.Request([]byte(infoHash), ip, port)
		}
	}

	// 创建 DHT 爬虫
	crawler.dhtCrawler = dht.New(dhtConfig)
	log.Println("[init] DHT 爬虫已创建....")
	return crawler, nil
}

// 解析监听地址
func parseListenAddr(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("无效的地址格式: %s", addr)
	}

	host := parts[0]
	if host == "" {
		host = "0.0.0.0"
	}

	var port int
	_, err := fmt.Sscanf(parts[1], "%d", &port)
	if err != nil {
		return "", 0, fmt.Errorf("无效的端口: %s", parts[1])
	}

	return host, port, nil
}

// GetKeywordCategory 获取关键词对应的分类
func (c *Crawler) GetKeywordCategory(keyword string) string {
	if c.filter == nil {
		return ""
	}
	return c.filter.GetCategory(keyword)
}

// AddKeyword 添加监控关键词
func (c *Crawler) AddKeyword(keyword, category string) {
	c.filter.AddKeyword(keyword, category)
	log.Printf("添加监控关键词: %s, 分类: %s", keyword, category)
}

// RemoveKeyword 移除监控关键词
func (c *Crawler) RemoveKeyword(keyword string) {
	c.filter.RemoveKeyword(keyword)
	log.Printf("移除监控关键词: %s", keyword)
}

// AddBlacklistKeyword 添加黑名单关键词
func (c *Crawler) AddBlacklistKeyword(keyword string) {
	c.filter.AddToBlacklist(keyword)
	log.Printf("添加黑名单关键词: %s", keyword)
}

// RemoveBlacklistKeyword 移除黑名单关键词
func (c *Crawler) RemoveBlacklistKeyword(keyword string) {
	c.filter.RemoveFromBlacklist(keyword)
	log.Printf("移除黑名单关键词: %s", keyword)
}

// GetKeywords 获取所有监控关键词
func (c *Crawler) GetKeywords() []string {
	return c.filter.GetKeywords()
}

// GetBlacklist 获取所有黑名单关键词
func (c *Crawler) GetBlacklist() []string {
	return c.filter.GetBlacklist()
}

// Start 启动爬虫
func (c *Crawler) Start() {
	if c.running {
		return
	}
	c.running = true

	// 启动 DHT Wire 组件
	go c.dhtWire.Run()
	c.logger.Info("DHT Wire 组件已启动")

	// 启动元数据处理器
	c.wg.Add(1)
	go c.processMetadata()
	c.logger.Info("元数据处理器已启动")

	// 启动 DHT 爬虫
	go c.dhtCrawler.Run()
	c.logger.Info("DHT 爬虫已启动")

	log.Println("爬虫已启动")
	c.logger.Info("爬虫已启动")
}

// Stop 停止爬虫
func (c *Crawler) Stop() {
	if !c.running {
		return
	}
	c.running = false

	// DHT 爬虫没有提供 Stop 方法，我们只能停止使用它
	c.logger.Info("DHT 爬虫已停止")

	// 等待处理结束
	c.wg.Wait()

	// 关闭元数据通道
	close(c.metadataChan)

	log.Println("爬虫已停止")
	c.logger.Info("爬虫已停止")
}

// processMetadata 处理元数据
func (c *Crawler) processMetadata() {
	defer c.wg.Done()

	// 处理从 DHT Wire 接收到的元数据
	for resp := range c.dhtWire.Response() {
		// 检查是否需要停止
		if !c.running {
			break
		}

		// 解码元数据
		metadata, err := dht.Decode(resp.MetadataInfo)
		if err != nil {
			c.logger.Debug(fmt.Sprintf("解码元数据失败: %v", err))
			continue
		}

		// 转换为元数据对象
		torrentMetadata, err := c.convertToTorrentMetadata(resp.InfoHash, metadata)
		if err != nil {
			c.logger.Debug(fmt.Sprintf("转换元数据失败: %v", err))
			continue
		}

		// 如果名称为空，跳过
		if torrentMetadata.Name == "" {
			continue
		}

		// 检查数据库中是否已存在
		exists, err := database.InfoHashExists(c.db, torrentMetadata.InfoHash)
		if err != nil {
			log.Printf("检查InfoHash存在失败: %v", err)
			continue
		}

		if exists {
			// 更新种子热度
			err = database.IncrementTorrentHeat(c.db, torrentMetadata.InfoHash)
			if err != nil {
				log.Printf("更新种子热度失败: %v", err)
			}
			continue
		}

		// 使用关键词过滤器匹配名称
		matched, keyword := c.filter.MatchContent(torrentMetadata.Name)
		if !matched {
			// 不匹配任何关键词，跳过
			log.Printf("不匹配任何关键词: %s", torrentMetadata.Name)
			continue
		}

		// 获取匹配关键词的分类
		category := c.filter.GetCategory(keyword)
		if category == "" {
			// 使用默认分类方法
			category = categorizeContent(torrentMetadata.Name, len(torrentMetadata.Files), torrentMetadata.Length)
		}
		log.Println("匹配关键词:", keyword, "分类:", category)

		// 转换为种子模型
		torrent := convertMetadataToTorrent(torrentMetadata, category)

		// 保存到数据库
		err = database.AddTorrent(c.db, torrent)
		if err != nil {
			log.Printf("保存种子失败: %v", err)
			continue
		}

		log.Printf("添加新种子: %s, 关键词: %s, 分类: %s, InfoHash: %s",
			torrent.Title, keyword, torrent.Category, torrent.InfoHash)
		c.logger.Info(fmt.Sprintf("添加新种子: %s [%s]", torrent.Title, torrent.InfoHash))
	}
}

// convertToTorrentMetadata 将 DHT 库的元数据转换为我们的 TorrentMetadata 结构
func (c *Crawler) convertToTorrentMetadata(infoHash []byte, metadata interface{}) (*TorrentMetadata, error) {
	info, ok := metadata.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("无效的元数据格式")
	}

	// 检查是否包含 info 字典
	log.Println("元数据:", info)

	// 创建元数据对象
	result := &TorrentMetadata{
		InfoHash: infoHash,
	}

	// 提取名称
	if name, ok := info["name"].(string); ok {
		result.Name = name
	} else {
		return nil, fmt.Errorf("元数据中没有名称")
	}

	// 提取注释
	if comment, ok := info["comment"].(string); ok {
		result.Comment = comment
	}

	// 提取创建时间
	if creation, ok := info["creation date"].(int64); ok {
		result.Creation = time.Unix(creation, 0)
	}

	// 提取宣告地址
	if announce, ok := info["announce"].(string); ok {
		result.Announce = announce
	}

	// 提取piece长度
	if pieceLength, ok := info["piece length"].(int64); ok {
		result.PieceLength = pieceLength
	} else if pieceLength, ok := info["piece length"].(int); ok {
		result.PieceLength = int64(pieceLength)
	}

	// 提取私有标志
	if private, ok := info["private"].(int64); ok {
		result.Private = int(private)
	} else if private, ok := info["private"].(int); ok {
		result.Private = private
	}

	// 处理单文件或多文件情况
	if length, ok := info["length"].(int64); ok {
		// 单文件情况
		result.Length = length
	} else if length, ok := info["length"].(int); ok {
		// 单文件情况 (int类型)
		result.Length = int64(length)
	} else if files, ok := info["files"].([]interface{}); ok {
		// 多文件情况
		result.Files = make([]TorrentFile, 0, len(files))
		var totalLength int64

		for _, file := range files {
			if fileDict, ok := file.(map[string]interface{}); ok {
				var tf TorrentFile

				// 处理长度
				if length, ok := fileDict["length"].(int64); ok {
					tf.Length = length
					totalLength += length
				} else if length, ok := fileDict["length"].(int); ok {
					tf.Length = int64(length)
					totalLength += int64(length)
				}

				// 处理路径
				if path, ok := fileDict["path"].([]interface{}); ok {
					tf.Path = make([]string, 0, len(path))
					for _, p := range path {
						if ps, ok := p.(string); ok {
							tf.Path = append(tf.Path, ps)
						}
					}
				}

				result.Files = append(result.Files, tf)
			}
		}

		result.Length = totalLength
	}

	return result, nil
}

// convertMetadataToTorrent 将元数据转换为种子模型
func convertMetadataToTorrent(metadata *TorrentMetadata, category string) *models.Torrent {
	// 构造磁力链接
	magnetLink := "magnet:?xt=urn:btih:" + hex.EncodeToString(metadata.InfoHash)

	// 添加跟踪器
	if metadata.Announce != "" {
		magnetLink += "&tr=" + url.QueryEscape(metadata.Announce)
	}

	// 添加名称
	if metadata.Name != "" {
		magnetLink += "&dn=" + url.QueryEscape(metadata.Name)
	}

	// 计算文件数
	fileCount := len(metadata.Files)
	if fileCount == 0 {
		fileCount = 1
	}

	// 如果未提供分类，自动分类
	if category == "" {
		category = categorizeContent(metadata.Name, fileCount, metadata.Length)
	}

	// 创建种子记录
	torrent := &models.Torrent{
		Title:       metadata.Name,
		InfoHash:    hex.EncodeToString(metadata.InfoHash),
		MagnetLink:  magnetLink,
		Size:        metadata.Length,
		FileCount:   fileCount,
		Category:    category,
		UploadDate:  metadata.Creation,
		Seeds:       0, // 未知
		Peers:       0, // 未知
		Downloads:   0, // 未知
		Description: metadata.Comment,
		Source:      "DHT",
		Heat:        1, // 初始热度
	}

	// 如果上传日期无效，使用当前时间
	if torrent.UploadDate.IsZero() {
		torrent.UploadDate = time.Now()
	}

	return torrent
}

// categorizeContent 根据名称和大小对内容进行分类
func categorizeContent(name string, fileCount int, size int64) string {
	// 原有的分类逻辑保持不变
	lowerName := strings.ToLower(name)

	// 视频文件
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".ts", ".m4v", ".3gp"}
	for _, ext := range videoExtensions {
		if strings.HasSuffix(lowerName, ext) {
			if size > 1024*1024*1024 {
				return "电影"
			}
			return "视频"
		}
	}

	// 音频文件
	audioExtensions := []string{".mp3", ".flac", ".aac", ".wav", ".wma", ".m4a", ".ogg"}
	for _, ext := range audioExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return "音乐"
		}
	}

	// 图片文件
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff"}
	for _, ext := range imageExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return "图片"
		}
	}

	// 文档文件
	docExtensions := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".epub"}
	for _, ext := range docExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return "文档"
		}
	}

	// 压缩文件
	archiveExtensions := []string{".zip", ".rar", ".7z", ".tar", ".gz", ".iso"}
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return "压缩包"
		}
	}

	// 根据关键词分类
	keywords := map[string][]string{
		"电影":  {"movie", "film", "bluray", "bdrip", "dvdrip", "1080p", "720p", "4k"},
		"电视剧": {"tv", "series", "season", "episode", "s01", "s02", "e01", "e02"},
		"音乐":  {"album", "discography", "soundtrack", "ost", "music"},
		"游戏":  {"game", "xbox", "ps4", "ps5", "nintendo", "switch"},
		"软件":  {"software", "app", "windows", "macos", "linux"},
		"动漫":  {"anime", "cartoon", "animation"},
		"电子书": {"ebook", "books", "novel", "comics", "manga"},
	}

	for category, words := range keywords {
		for _, word := range words {
			if strings.Contains(lowerName, word) {
				return category
			}
		}
	}

	// 根据文件数和大小进行猜测
	if fileCount > 50 && size > 10*1024*1024*1024 {
		return "合集"
	} else if size > 4*1024*1024*1024 {
		return "电影"
	} else if fileCount > 10 {
		return "其他"
	}

	return "未知"
}

// 初始化默认关键词
func initDefaultKeywords(filter *KeywordFilter) {
	// 电影类
	filter.AddKeywords([]string{"movie", "film", "bluray", "bdrip", "1080p", "720p", "4k", "uhd", "av", "jav", "sex"}, "电影")

	// 电视剧类
	filter.AddKeywords([]string{"tv series", "season", "episode", "s01", "s02", "e01", "e02", "tv"}, "电视剧")

	// 动漫类
	filter.AddKeywords([]string{"anime", "animation", "cartoon", "animated"}, "动漫")

	// 音乐类
	filter.AddKeywords([]string{"ost", "soundtrack", "album", "discography", "concert", "music", "mp3", "flac"}, "音乐")

	// 软件类
	filter.AddKeywords([]string{"software", "application", "app", "windows", "macos", "linux", "android", "ios"}, "软件")

	// 游戏类
	filter.AddKeywords([]string{"game", "pc game", "xbox", "playstation", "ps4", "ps5", "nintendo", "switch"}, "游戏")

	// 文档/电子书
	filter.AddKeywords([]string{"ebook", "pdf", "epub", "mobi", "azw3", "textbook", "book"}, "电子书")

	// 黑名单关键词(敏感词汇)
	filter.AddBlacklist([]string{"child", "teen", "underage"})
}
