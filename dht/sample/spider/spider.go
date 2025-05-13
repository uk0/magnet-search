package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"magnet-search/dht"
	"net/http"
	_ "net/http/pprof"
)

type file struct {
	Path   []interface{} `json:"path"`
	Length int           `json:"length"`
}

type bitTorrent struct {
	InfoHash string `json:"infohash"`
	Name     string `json:"name"`
	Files    []file `json:"files,omitempty"`
	Length   int    `json:"length,omitempty"`
}

func main() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	w := dht.NewWire(65536, 1024, 256)
	go func() {
		for resp := range w.Response() {
			metadata, err := dht.Decode(resp.MetadataInfo)
			if err != nil {
				continue
			}
			info := metadata.(map[string]interface{})

			if _, ok := info["name"]; !ok {
				continue
			}

			bt := bitTorrent{
				InfoHash: hex.EncodeToString(resp.InfoHash),
				Name:     info["name"].(string),
			}

			if v, ok := info["files"]; ok {
				files := v.([]interface{})
				bt.Files = make([]file, len(files))

				for i, item := range files {
					f := item.(map[string]interface{})
					bt.Files[i] = file{
						Path:   f["path"].([]interface{}),
						Length: f["length"].(int),
					}
				}
			} else if _, ok := info["length"]; ok {
				bt.Length = info["length"].(int)
			}

			data, err := json.Marshal(bt)
			if err == nil {
				fmt.Printf("%s\n\n", data)
			}
		}
	}()
	go w.Run()

	config := dht.NewCrawlConfig()
	// 公告对等点时的回调
	config.OnAnnouncePeer = func(infoHash, ip string, port int) {
		w.Request([]byte(infoHash), ip, port)
		hexInfoHash := hex.EncodeToString([]byte(infoHash))
		log.Printf("收到announce_peer请求: %s，来自 %s:%d",
			hexInfoHash, ip, port)
	}
	// 配置DHT回调函数
	config.OnGetPeers = func(infoHash, ip string, port int) {
		// 将二进制InfoHash转换为十六进制字符串
		hexInfoHash := hex.EncodeToString([]byte(infoHash))
		log.Printf("收到get_peers请求: %s，来自 %s:%d", hexInfoHash, ip, port)
	}

	// 对等点找到时的回调
	config.OnGetPeersResponse = func(infoHash string, peer *dht.Peer) {
		hexInfoHash := hex.EncodeToString([]byte(infoHash))
		log.Printf("找到对等点: %s:%d，所属资源: %s",
			peer.IP.String(), peer.Port, hexInfoHash)
	}

	d := dht.New(config)

	d.Run()
}
