package crawler

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
