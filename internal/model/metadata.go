package model

import (
	"time"
)

// TorrentFile 表示种子中的一个文件
type TorrentFile struct {
	Length int64    `json:"length"`
	Path   []string `json:"path"`
}

// TorrentMetadata 种子元数据
type TorrentMetadata struct {
	InfoHash    []byte        `json:"info_hash"`
	Name        string        `json:"name"`
	Length      int64         `json:"length"`
	Files       []TorrentFile `json:"files"`
	PieceLength int64         `json:"piece_length"`
	Pieces      string        `json:"pieces"`
	Private     int           `json:"private"`
	Announce    string        `json:"announce"`
	Comment     string        `json:"comment"`
	Creation    time.Time     `json:"creation"`
}

// Torrent 表示一个种子资源
type Torrent struct {
	Title       string        `json:"title" bson:"title"`
	InfoHash    string        `json:"info_hash" bson:"info_hash"`
	MagnetLink  string        `json:"magnet_link" bson:"magnet_link"`
	Size        int64         `json:"size" bson:"size"`
	FileCount   int           `json:"file_count" bson:"file_count"`
	Category    string        `json:"category" bson:"category"`
	UploadDate  time.Time     `json:"upload_date" bson:"upload_date"`
	Seeds       int           `json:"seeds" bson:"seeds"`
	Peers       int           `json:"peers" bson:"peers"`
	Downloads   int           `json:"downloads" bson:"downloads"`
	Description string        `json:"description" bson:"description"`
	Source      string        `json:"source" bson:"source"`
	Heat        int           `json:"heat" bson:"heat"`
	Files       []TorrentFile `json:"files" bson:"files"` // 文件列表
}

// CategoryCount 表示分类及其数量
type CategoryCount struct {
	Category string `json:"category" bson:"category"`
	Count    int    `json:"count" bson:"count"`
}

// SearchRequest 表示搜索请求参数
type SearchRequest struct {
	Query    string // 搜索关键词
	Category string // 分类筛选
	Sort     string // 排序方式
	Order    string // 排序顺序
	Page     int    // 页码
	PageSize int    // 每页结果数
}

// SearchResult 表示搜索结果
type SearchResult struct {
	Torrents  []Torrent // 搜索结果列表
	Total     int       // 总结果数
	Page      int       // 当前页码
	PageSize  int       // 每页结果数
	TotalPage int       // 总页数
}
