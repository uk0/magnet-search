package database

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"magnet-search/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// DB 结构体用于封装MongoDB客户端和集合
type DB struct {
	client     *mongo.Client
	Torrents   *mongo.Collection
	keywords   *mongo.Collection
	statistics *mongo.Collection
	Ctx        context.Context
	cancel     context.CancelFunc
}

// InitDB 初始化MongoDB连接
func InitDB(mongoURL string) (*DB, error) {
	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// 创建客户端连接选项
	clientOptions := options.Client().
		ApplyURI(mongoURL).
		SetMaxPoolSize(100).                // 设置最大连接池大小
		SetMinPoolSize(10).                 // 设置最小连接池大小
		SetMaxConnIdleTime(5 * time.Minute) // 空闲连接最大存活时间

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("连接MongoDB失败: %v", err)
	}

	// 测试连接
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("MongoDB Ping失败: %v", err)
	}

	// 获取数据库和集合
	database := client.Database("magnet_search")
	torrentsCollection := database.Collection("torrents")
	keywordsCollection := database.Collection("keywords")
	statisticsCollection := database.Collection("statistics")

	// 创建索引
	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "info_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "title", Value: "text"}},
		},
		{
			Keys: bson.D{{Key: "description", Value: "text"}},
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "upload_date", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "heat", Value: -1}},
		},
	}

	// 创建索引
	for _, indexModel := range indexModels {
		_, err := torrentsCollection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			log.Printf("创建索引失败: %v", err)
		}
	}

	log.Println("MongoDB 连接成功")

	return &DB{
		client:     client,
		Torrents:   torrentsCollection,
		keywords:   keywordsCollection,
		statistics: statisticsCollection,
		Ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Close 关闭MongoDB连接
func (db *DB) Close() error {
	if db.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return db.client.Disconnect(ctx)
	}
	return nil
}

// 为每个数据库操作创建新的上下文
func createContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// UpdateTorrentsTable 更新集合结构 (MongoDB 中不需要)
func UpdateTorrentsTable(db *DB) error {
	// MongoDB 是无模式的，不需要明确更新表结构
	return nil
}

// AddTorrent 添加新种子
func AddTorrent(db *DB, torrent *models.Torrent) error {
	// 先检查是否已存在
	ctx, cancel := createContext()
	defer cancel()
	var existingTorrent models.Torrent
	err := db.Torrents.FindOne(ctx, bson.M{"info_hash": torrent.InfoHash}).Decode(&existingTorrent)

	// 如果已存在，则更新热度
	if err == nil {
		update := bson.M{
			"$inc": bson.M{"heat": 1},
		}
		_, err := db.Torrents.UpdateOne(ctx, bson.M{"info_hash": torrent.InfoHash}, update)
		return err
	}

	// 如果不存在或发生其他错误，则插入新记录
	if err == mongo.ErrNoDocuments {
		_, err := db.Torrents.InsertOne(ctx, torrent)
		return err
	}

	return err
}

// IncrementTorrentHeat 增加种子热度
func IncrementTorrentHeat(db *DB, infoHash []byte) error {
	ctx, cancel := createContext()
	defer cancel()
	hexInfoHash := fmt.Sprintf("%x", infoHash)
	update := bson.M{
		"$inc": bson.M{"heat": 1},
	}
	_, err := db.Torrents.UpdateOne(ctx, bson.M{"info_hash": hexInfoHash}, update)
	return err
}

// InfoHashExists 检查InfoHash是否存在
func InfoHashExists(db *DB, infoHash []byte) (bool, error) {
	ctx, cancel := createContext()
	defer cancel()
	hexInfoHash := fmt.Sprintf("%x", infoHash)
	count, err := db.Torrents.CountDocuments(ctx, bson.M{"info_hash": hexInfoHash})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SearchTorrents 搜索种子
func SearchTorrents(db *DB, keyword string, category string, sortBy string, page, pageSize int) (*models.SearchResult, error) {
	// 构建查询条件
	ctx, cancel := createContext()
	defer cancel()
	filter := bson.M{}

	// 添加关键词搜索 - 同时搜索标题和描述
	if keyword != "" {
		// 两种搜索策略:
		// 1. 如果集合已创建文本索引（推荐用于生产环境）
		// filter["$text"] = bson.M{"$search": keyword}

		// 2. 使用正则表达式（更灵活但性能较差）
		keywordRegex := primitive.Regex{
			Pattern: keyword,
			Options: "i", // 不区分大小写
		}
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": keywordRegex}},
			{"description": bson.M{"$regex": keywordRegex}},
		}
	}

	// 添加分类过滤
	if category != "" && category != "全部" {
		filter["category"] = category
	}

	// 排序选项
	sortOpt := bson.D{}
	switch sortBy {
	case "heat":
		sortOpt = bson.D{{Key: "heat", Value: -1}}
	case "size":
		sortOpt = bson.D{{Key: "size", Value: -1}}
	case "time":
		sortOpt = bson.D{{Key: "upload_date", Value: -1}}
	default:
		sortOpt = bson.D{{Key: "upload_date", Value: -1}}
	}

	// 计算总数
	total, err := db.Torrents.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 设置分页
	skip := (page - 1) * pageSize
	limit := int64(pageSize)

	// 查询选项
	options := options.Find().
		SetSort(sortOpt).
		SetSkip(int64(skip)).
		SetLimit(limit)

	// 执行查询
	cursor, err := db.Torrents.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}

	// 解析结果
	var torrents []models.Torrent
	if err := cursor.All(ctx, &torrents); err != nil {
		return nil, err
	}

	// 修改这一行
	return &models.SearchResult{
		torrents,
		int(total),
		page,
		pageSize,
		int(total)/pageSize + (map[bool]int{true: 1, false: 0}[int(total)%pageSize > 0]),
	}, nil
}

// GetLatestTorrents 获取最新种子
func GetLatestTorrents(db *DB, limit int) ([]models.Torrent, error) {
	ctx, cancel := createContext()
	defer cancel()
	options := options.Find().
		SetSort(bson.D{{Key: "upload_date", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := db.Torrents.Find(ctx, bson.M{}, options)
	if err != nil {
		return nil, err
	}

	var torrents []models.Torrent
	if err := cursor.All(ctx, &torrents); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(torrents)
	log.Println("最新种子:", string(b))
	return torrents, nil
}

// GetPopularTorrents 获取热门种子
func GetPopularTorrents(db *DB, limit int) ([]models.Torrent, error) {
	ctx, cancel := createContext()
	defer cancel()
	options := options.Find().
		SetSort(bson.D{{Key: "heat", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := db.Torrents.Find(ctx, bson.M{}, options)
	if err != nil {
		return nil, err
	}

	var torrents []models.Torrent
	if err := cursor.All(ctx, &torrents); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(torrents)
	log.Println("热门种子:", string(b))

	return torrents, nil
}

// GetCategories 获取所有分类及其数量
func GetCategories(db *DB) ([]models.CategoryCount, error) {
	ctx, cancel := createContext()
	defer cancel()
	pipeline := mongo.Pipeline{
		{
			{"$group", bson.D{
				{"_id", "$category"},
				{"count", bson.D{{"$sum", 1}}},
			}},
		},
		{
			{"$sort", bson.D{{"count", -1}}},
		},
	}

	cursor, err := db.Torrents.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var results []struct {
		ID    string `bson:"_id"`
		Count int    `bson:"count"`
	}

	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	categories := make([]models.CategoryCount, 0, len(results))
	for _, result := range results {
		categories = append(categories, models.CategoryCount{
			Category: result.ID,
			Count:    result.Count,
		})
	}

	b, _ := json.Marshal(categories)
	log.Println("分类统计结果:", string(b))

	return categories, nil
}

// GetTorrentByInfoHash 通过InfoHash获取种子
func GetTorrentByInfoHash(db *DB, infoHash string) (*models.Torrent, error) {
	ctx, cancel := createContext()
	defer cancel()
	var torrent models.Torrent
	err := db.Torrents.FindOne(ctx, bson.M{"info_hash": infoHash}).Decode(&torrent)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("未找到种子")
		}
		return nil, err
	}
	return &torrent, nil
}
