package main

import (
	"flag"
	"go.mongodb.org/mongo-driver/bson"
	"log"
	"magnet-search/internal/database"
	"magnet-search/internal/model"
	"magnet-search/internal/server"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 命令行参数
	port := flag.String("port", "27777", "HTTP服务端口")
	dbURL := flag.String("db", "mongodb://root:123@mongo-1:27011,mongo-1:27012,mongo-1:27013/?replicaSet=rs", "mongo_db_url")
	flag.Parse()

	// 初始化数据库
	log.Printf("正在连接数据库: %s", *dbURL)
	db, err := database.InitDB(*dbURL)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()
	log.Println("数据库连接成功")

	// 添加测试数据
	if err := addTestData(db); err != nil {
		log.Printf("添加测试数据失败: %v", err)
	} else {
		log.Println("测试数据检查完成")
	}

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在单独的协程中启动服务器
	errChan := make(chan error)
	go func() {
		log.Printf("启动磁力搜索服务于 http://0.0.0.0:%s", *port)
		errChan <- server.Run(*port, db, nil) // 爬虫为nil，表示该服务不包含爬虫功能
	}()

	// 等待退出信号或错误
	select {
	case <-sigChan:
		log.Println("收到退出信号，正在关闭Web服务...")
		log.Println("Web服务已关闭")
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Web服务器运行错误: %v", err)
		}
	}
}

// 添加测试数据
func addTestData(db *database.DB) error {
	// 检查是否已有数据
	count, err := db.Torrents.CountDocuments(db.Ctx, bson.M{})
	if err != nil {
		return err
	}

	// 如果已有数据，跳过
	if count > 0 {
		log.Printf("数据库中已有 %d 条记录，跳过测试数据添加", count)
		return nil
	}

	log.Println("正在添加测试数据...")

	// 示例数据
	testTorrents := []model.Torrent{
		{
			Title:       "Ubuntu 22.04 Desktop (64bit)",
			InfoHash:    "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
			MagnetLink:  "magnet:?xt=urn:btih:a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
			Size:        3_500_000_000,
			FileCount:   1,
			Category:    "操作系统",
			UploadDate:  time.Now().Add(-24 * time.Hour),
			Seeds:       1500,
			Peers:       300,
			Downloads:   25000,
			Description: "Ubuntu 22.04 LTS 官方桌面版ISO",
			Source:      "官方网站",
			Heat:        100,
		},
		{
			Title:       "Debian 11 (64bit)",
			InfoHash:    "b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1",
			MagnetLink:  "magnet:?xt=urn:btih:b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1",
			Size:        2_800_000_000,
			FileCount:   1,
			Category:    "操作系统",
			UploadDate:  time.Now().Add(-48 * time.Hour),
			Seeds:       1200,
			Peers:       250,
			Downloads:   20000,
			Description: "Debian 11 稳定版ISO",
			Source:      "官方网站",
			Heat:        80,
		},
		{
			Title:       "Big Buck Bunny 4K",
			InfoHash:    "c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2",
			MagnetLink:  "magnet:?xt=urn:btih:c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2",
			Size:        750_000_000,
			FileCount:   1,
			Category:    "视频",
			UploadDate:  time.Now().Add(-72 * time.Hour),
			Seeds:       800,
			Peers:       150,
			Downloads:   15000,
			Description: "Big Buck Bunny 开源动画电影4K版本",
			Source:      "官方发布",
			Heat:        120,
		},
		{
			Title:       "Blender 3.4.1 Windows 64bit",
			InfoHash:    "d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3",
			MagnetLink:  "magnet:?xt=urn:btih:d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3",
			Size:        250_000_000,
			FileCount:   5,
			Category:    "软件",
			UploadDate:  time.Now().Add(-96 * time.Hour),
			Seeds:       950,
			Peers:       200,
			Downloads:   18000,
			Description: "Blender 3D建模软件 Windows版",
			Source:      "官方网站",
			Heat:        90,
		},
		{
			Title:       "GIMP 2.10.32 for Linux",
			InfoHash:    "e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4",
			MagnetLink:  "magnet:?xt=urn:btih:e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4",
			Size:        125_000_000,
			FileCount:   3,
			Category:    "软件",
			UploadDate:  time.Now().Add(-120 * time.Hour),
			Seeds:       600,
			Peers:       120,
			Downloads:   12000,
			Description: "GIMP 图像编辑软件 Linux版",
			Source:      "官方网站",
			Heat:        70,
		},
	}

	for _, torrent := range testTorrents {
		if err := database.AddTorrent(db, &torrent); err != nil {
			return err
		}
	}

	log.Printf("已成功添加 %d 条测试数据", len(testTorrents))
	return nil
}
