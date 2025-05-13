package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"magnet-search/internal/crawler"
	"magnet-search/internal/database"
	"magnet-search/internal/models"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Server 表示HTTP服务器
type Server struct {
	db         *database.DB
	crawler    *crawler.Crawler
	templates  *template.Template
	staticPath string
}

// 分页项类型
type PaginationItem struct {
	Type    string // "page" 或 "ellipsis"
	Number  int    // 页码
	Current bool   // 是否当前页
}

// formatDate 格式化日期为人类可读格式
func formatDate(t time.Time) string {
	if t.IsZero() {
		return "未知时间"
	}
	return t.Format("2006-01-02 15:04:05")
}

// formatSize 将字节大小格式化为人类可读的形式
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 2, 64) + " " + string("KMGTPE"[exp]) + "B"
}

// 生成分页数据
func generatePagination(currentPage, totalPages int) []PaginationItem {
	var items []PaginationItem

	// 显示第一页
	if currentPage > 3 {
		items = append(items, PaginationItem{Type: "page", Number: 1})

		// 如果当前页离第一页较远，添加省略号
		if currentPage > 4 {
			items = append(items, PaginationItem{Type: "ellipsis"})
		}
	}

	// 显示当前页前一页
	if currentPage > 1 {
		items = append(items, PaginationItem{Type: "page", Number: currentPage - 1})
	}

	// 显示当前页
	items = append(items, PaginationItem{Type: "page", Number: currentPage, Current: true})

	// 显示当前页后一页
	if currentPage < totalPages {
		items = append(items, PaginationItem{Type: "page", Number: currentPage + 1})
	}

	// 显示最后一页
	if currentPage < totalPages-2 {
		// 如果当前页离最后页较远，添加省略号
		if currentPage < totalPages-3 {
			items = append(items, PaginationItem{Type: "ellipsis"})
		}
		items = append(items, PaginationItem{Type: "page", Number: totalPages})
	}

	return items
}

// Run 运行服务器
func Run(port string, db *database.DB, crawler *crawler.Crawler) error {
	server := &Server{
		db:         db,
		crawler:    crawler,
		staticPath: "./static",
	}

	// 加载模板
	templates, err := template.New("").Funcs(template.FuncMap{
		"formatSize": formatSize,
		"formatDate": formatDate,
	}).ParseGlob("templates/*.html")
	if err != nil {
		return err
	}
	server.templates = templates

	// 设置路由
	http.HandleFunc("/", server.indexHandler)
	http.HandleFunc("/search", server.searchHandler)

	// 添加管理界面
	//http.HandleFunc("/admin", server.adminHandler)

	// 添加关键词管理API
	//http.HandleFunc("/api/keywords", server.keywordAPIHandler)
	//http.HandleFunc("/api/blacklist", server.blacklistAPIHandler)
	//http.HandleFunc("/api/stats", server.statsAPIHandler)

	// 添加日志API
	//http.HandleFunc("/api/logs", server.logsAPIHandler)
	//http.HandleFunc("/api/log-dates", server.logDatesAPIHandler)
	//http.HandleFunc("/api/daily-stats", server.dailyStatsAPIHandler)

	// 静态文件服务
	fs := http.FileServer(http.Dir(server.staticPath))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 启动服务器
	log.Printf("服务器启动在 http://localhost:%s", port)
	return http.ListenAndServe(":"+port, nil)
}

// logsAPIHandler 处理日志API请求
func (s *Server) logsAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	// 获取日期参数
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// 验证日期格式
	_, err := time.Parse("2006-01-02", date)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "无效的日期格式，请使用YYYY-MM-DD格式",
		})
		return
	}

	// 日志文件路径
	logFilePath := filepath.Join("logs", fmt.Sprintf("crawler-%s.log", date))

	// 检查文件是否存在
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("日期 %s 的日志文件不存在", date),
		})
		return
	}

	// 读取日志文件内容
	content, err := os.ReadFile(logFilePath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "读取日志文件失败: " + err.Error(),
		})
		return
	}

	// 返回日志内容
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"date":    date,
		"content": string(content),
	})
}

// logDatesAPIHandler 处理日志日期列表API请求
func (s *Server) logDatesAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	// 读取logs目录
	files, err := os.ReadDir("logs")
	if err != nil {
		// 如果目录不存在，返回空列表
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"dates":  []string{},
			})
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "读取日志目录失败: " + err.Error(),
		})
		return
	}

	// 提取日期
	dates := make([]string, 0)
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasPrefix(name, "crawler-") && strings.HasSuffix(name, ".log") {
			dateStr := strings.TrimPrefix(name, "crawler-")
			dateStr = strings.TrimSuffix(dateStr, ".log")

			// 验证日期格式
			if _, err := time.Parse("2006-01-02", dateStr); err == nil {
				dates = append(dates, dateStr)
			}
		}
	}

	// 按日期排序（降序）
	sort.Slice(dates, func(i, j int) bool {
		return dates[i] > dates[j]
	})

	// 返回日期列表
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"dates":  dates,
	})
}

// dailyStatsAPIHandler 处理每日统计API请求
func (s *Server) dailyStatsAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	// 获取30天前的日期
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// 创建MongoDB聚合管道
	pipeline := mongo.Pipeline{
		{
			{"$match", bson.M{
				"upload_date": bson.M{"$gte": thirtyDaysAgo},
			}},
		},
		{
			{"$project", bson.M{
				"date": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d",
						"date":   "$upload_date",
					},
				},
			}},
		},
		{
			{"$group", bson.M{
				"_id":   "$date",
				"count": bson.M{"$sum": 1},
			}},
		},
		{
			{"$sort", bson.M{"_id": 1}},
		},
	}

	// 执行聚合
	cursor, err := s.db.Torrents.Aggregate(s.db.Ctx, pipeline)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "查询数据失败: " + err.Error(),
		})
		return
	}
	defer cursor.Close(s.db.Ctx)

	// 提取数据
	type DailyCount struct {
		Date  string `json:"date" bson:"_id"`
		Count int    `json:"count" bson:"count"`
	}

	var results []DailyCount
	if err := cursor.All(s.db.Ctx, &results); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "解析数据失败: " + err.Error(),
		})
		return
	}

	// 返回数据
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   results,
	})
}

// indexHandler 处理首页请求
func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 获取最近添加的种子
	recentTorrents, err := database.GetLatestTorrents(s.db, 20)
	if err != nil {
		log.Printf("获取最近种子失败: %v", err)
		http.Error(w, "获取最近种子失败", http.StatusInternalServerError)
		return
	}

	// 获取热门种子
	hotTorrents, err := database.GetPopularTorrents(s.db, 20)
	if err != nil {
		log.Printf("获取热门种子失败: %v", err)
		http.Error(w, "获取热门种子失败", http.StatusInternalServerError)
		return
	}

	// 获取分类统计
	categories, err := database.GetCategories(s.db)
	if err != nil {
		log.Printf("获取分类统计失败: %v", err)
		http.Error(w, "获取分类统计失败", http.StatusInternalServerError)
		return
	}

	log.Printf("首页数据统计: 分类=%d, 热门=%d, 最新=%d",
		len(categories), len(hotTorrents), len(recentTorrents))

	data := map[string]interface{}{
		"Title":          "磁力搜索引擎",
		"RecentTorrents": recentTorrents,
		"HotTorrents":    hotTorrents,
		"Categories":     categories,
	}

	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// searchHandler 处理搜索页面请求
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("pageSize")

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 20
	}

	// 执行搜索
	result, err := database.SearchTorrents(s.db, query, category, sort, page, pageSize)
	if err != nil {
		http.Error(w, "搜索失败", http.StatusInternalServerError)
		return
	}

	// 获取分类统计
	categories, err := database.GetCategories(s.db)
	if err != nil {
		http.Error(w, "获取分类统计失败", http.StatusInternalServerError)
		return
	}

	// 计算上一页、下一页
	prevPage := result.Page - 1
	if prevPage < 1 {
		prevPage = 1
	}

	nextPage := result.Page + 1
	if nextPage > result.TotalPage {
		nextPage = result.TotalPage
	}

	// 生成分页数据
	pages := generatePagination(result.Page, result.TotalPage)

	data := map[string]interface{}{
		"Title":      "搜索结果 - " + query,
		"Query":      query,
		"Category":   category,
		"Sort":       sort,
		"Order":      order,
		"Result":     result,
		"Categories": categories,
		// 分页数据
		"Page":       result.Page,
		"TotalPages": result.TotalPage,
		"Prev":       prevPage,
		"Next":       nextPage,
		"Pages":      pages,
	}

	if err := s.templates.ExecuteTemplate(w, "search.html", data); err != nil {
		// 只记录错误，不尝试再写入响应
		log.Printf("模板渲染错误: %v", err)
		return
	}
}

// apiSearchHandler 处理API搜索请求
func (s *Server) apiSearchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sort := r.URL.Query().Get("sort")
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("pageSize")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	// 执行搜索
	result, err := database.SearchTorrents(s.db, query, category, sort, page, pageSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "搜索失败"})
		return
	}

	json.NewEncoder(w).Encode(result)
}

// apiAddTorrentHandler 处理添加种子的API请求(仅用于测试)
func (s *Server) apiAddTorrentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var torrent models.Torrent
	if err := json.NewDecoder(r.Body).Decode(&torrent); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "无效的请求数据"})
		return
	}

	if torrent.Title == "" || torrent.InfoHash == "" || torrent.MagnetLink == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少必要字段"})
		return
	}

	// 设置默认值
	if torrent.UploadDate.IsZero() {
		torrent.UploadDate = time.Now()
	}

	// 添加到数据库
	if err := database.AddTorrent(s.db, &torrent); err != nil {
		log.Printf("添加种子失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "添加种子失败"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "种子添加成功"})
}

// keywordAPIHandler 处理关键词API请求
func (s *Server) keywordAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// 获取所有关键词
		keywords := s.crawler.GetKeywords()

		// 将关键词转换为更详细的格式
		type KeywordInfo struct {
			Keyword  string `json:"keyword"`
			Category string `json:"category"`
		}

		keywordInfos := make([]KeywordInfo, 0, len(keywords))
		for _, keyword := range keywords {
			category := s.crawler.GetKeywordCategory(keyword)
			keywordInfos = append(keywordInfos, KeywordInfo{
				Keyword:  keyword,
				Category: category,
			})
		}

		// 输出JSON
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "success",
			"keywords": keywordInfos,
		})

	case http.MethodPost:
		// 添加新关键词
		var data struct {
			Keyword  string `json:"keyword"`
			Category string `json:"category"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "无效的请求数据"})
			return
		}

		if data.Keyword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "关键词不能为空"})
			return
		}

		s.crawler.AddKeyword(data.Keyword, data.Category)

		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "关键词添加成功"})

	case http.MethodDelete:
		// 删除关键词
		keyword := r.URL.Query().Get("keyword")
		if keyword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "必须指定要删除的关键词"})
			return
		}

		s.crawler.RemoveKeyword(keyword)

		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "关键词删除成功"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "不支持的HTTP方法"})
	}
}

// blacklistAPIHandler 处理黑名单API请求
func (s *Server) blacklistAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// 获取所有黑名单关键词
		blacklist := s.crawler.GetBlacklist()

		// 输出JSON
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "success",
			"blacklist": blacklist,
		})

	case http.MethodPost:
		// 添加新的黑名单关键词
		var data struct {
			Keyword string `json:"keyword"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "无效的请求数据"})
			return
		}

		if data.Keyword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "关键词不能为空"})
			return
		}

		s.crawler.AddBlacklistKeyword(data.Keyword)

		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "黑名单关键词添加成功"})

	case http.MethodDelete:
		// 删除黑名单关键词
		keyword := r.URL.Query().Get("keyword")
		if keyword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "必须指定要删除的关键词"})
			return
		}

		s.crawler.RemoveBlacklistKeyword(keyword)

		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "黑名单关键词删除成功"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "不支持的HTTP方法"})
	}
}

// adminHandler 处理管理界面请求
func (s *Server) adminHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":     "管理界面 - 磁力搜索引擎",
		"Keywords":  s.crawler.GetKeywords(),
		"Blacklist": s.crawler.GetBlacklist(),
	}

	if err := s.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// statsAPIHandler 处理统计数据API请求
func (s *Server) statsAPIHandler(w http.ResponseWriter, r *http.Request) {
	// 设置JSON响应头
	w.Header().Set("Content-Type", "application/json")

	// 获取今日日期
	today := time.Now()
	startOfDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	// 获取今日嗅探量
	todayCount, err := s.db.Torrents.CountDocuments(s.db.Ctx, bson.M{
		"upload_date": bson.M{
			"$gte": startOfDay,
			"$lt":  endOfDay,
		},
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "获取今日数据失败"})
		return
	}

	// 获取总嗅探量
	totalCount, err := s.db.Torrents.CountDocuments(s.db.Ctx, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "获取总数据失败"})
		return
	}

	// 获取已保存种子数（使用去重操作）
	pipeline := mongo.Pipeline{
		{
			{"$group", bson.M{
				"_id": "$info_hash",
			}},
		},
		{
			{"$count", "count"},
		},
	}

	var savedCountResult []bson.M
	cursor, err := s.db.Torrents.Aggregate(s.db.Ctx, pipeline)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "获取已保存数据失败"})
		return
	}
	defer cursor.Close(s.db.Ctx)

	if err := cursor.All(s.db.Ctx, &savedCountResult); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "处理已保存数据失败"})
		return
	}

	savedCount := int64(0)
	if len(savedCountResult) > 0 {
		savedCount = savedCountResult[0]["count"].(int64)
	}

	// 返回统计数据
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"todayCount": todayCount,
		"totalCount": totalCount,
		"savedCount": savedCount,
	})
}
