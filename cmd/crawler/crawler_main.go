package main

import (
	"flag"
	"log"
	"magnet-search/internal/crawler"
	"magnet-search/internal/database"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	// 命令行参数
	dbURL := flag.String("db", "mongodb://root:123@mongo-1:27011,mongo-1:27012,mongo-1:27013/?replicaSet=rs", "mongo_db_url")
	dhtAddr := flag.String("dht", ":26881", "DHT监听地址")
	concurrency := flag.Int("concurrency", 10, "元数据获取并发数")
	maxProcs := flag.Int("max-procs", 0, "最大处理器核心数，0表示使用所有可用核心")
	flag.Parse()

	// 设置最大使用的CPU核心数
	if *maxProcs > 0 {
		runtime.GOMAXPROCS(*maxProcs)
		log.Printf("设置处理器核心数为 %d", *maxProcs)
	} else {
		cpuNum := runtime.NumCPU()
		log.Printf("使用所有可用处理器核心: %d", cpuNum)
	}

	// 初始化数据库
	log.Printf("正在连接数据库: %s", *dbURL)
	db, err := database.InitDB(*dbURL)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()
	log.Println("数据库连接成功")

	// 创建并启动DHT爬虫
	dhtCrawler, err := crawler.NewCrawler(db, *dhtAddr, *concurrency)
	if err != nil {
		log.Fatalf("创建爬虫失败: %v", err)
	}

	// 启动爬虫
	dhtCrawler.Start()
	log.Printf("DHT爬虫已启动于 %s (并发: %d)", *dhtAddr, *concurrency)

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待退出信号
	<-sigChan
	log.Println("收到退出信号，正在关闭爬虫...")
	dhtCrawler.Stop()
	log.Println("爬虫已停止，程序退出")
}
