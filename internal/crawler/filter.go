package crawler

import (
	"strings"
	"sync"
)

// KeywordFilter 关键词过滤器
type KeywordFilter struct {
	keywords    []string          // 关键词列表
	blacklist   []string          // 黑名单关键词
	categoryMap map[string]string // 关键词到分类的映射
	mutex       sync.RWMutex      // 读写锁
}

// NewKeywordFilter 创建一个新的关键词过滤器
func NewKeywordFilter() *KeywordFilter {
	return &KeywordFilter{
		keywords:    []string{},
		blacklist:   []string{},
		categoryMap: make(map[string]string),
	}
}

// AddKeyword 添加一个新的关键词
func (kf *KeywordFilter) AddKeyword(keyword, category string) {
	kf.mutex.Lock()
	defer kf.mutex.Unlock()

	keyword = strings.ToLower(keyword)
	if !kf.containsKeyword(keyword) {
		kf.keywords = append(kf.keywords, keyword)
		if category != "" {
			kf.categoryMap[keyword] = category
		}
	}
}

// AddKeywords 批量添加关键词
func (kf *KeywordFilter) AddKeywords(keywords []string, category string) {
	for _, keyword := range keywords {
		kf.AddKeyword(keyword, category)
	}
}

// AddToBlacklist 添加一个黑名单关键词
func (kf *KeywordFilter) AddToBlacklist(keyword string) {
	kf.mutex.Lock()
	defer kf.mutex.Unlock()

	keyword = strings.ToLower(keyword)
	if !kf.containsBlacklistKeyword(keyword) {
		kf.blacklist = append(kf.blacklist, keyword)
	}
}

// AddToBlacklist 批量添加黑名单关键词
func (kf *KeywordFilter) AddBlacklist(keywords []string) {
	for _, keyword := range keywords {
		kf.AddToBlacklist(keyword)
	}
}

// RemoveKeyword 移除一个关键词
func (kf *KeywordFilter) RemoveKeyword(keyword string) {
	kf.mutex.Lock()
	defer kf.mutex.Unlock()

	keyword = strings.ToLower(keyword)
	for i, k := range kf.keywords {
		if k == keyword {
			kf.keywords = append(kf.keywords[:i], kf.keywords[i+1:]...)
			delete(kf.categoryMap, keyword)
			break
		}
	}
}

// RemoveFromBlacklist 从黑名单中移除一个关键词
func (kf *KeywordFilter) RemoveFromBlacklist(keyword string) {
	kf.mutex.Lock()
	defer kf.mutex.Unlock()

	keyword = strings.ToLower(keyword)
	for i, k := range kf.blacklist {
		if k == keyword {
			kf.blacklist = append(kf.blacklist[:i], kf.blacklist[i+1:]...)
			break
		}
	}
}

// containsKeyword 检查关键词是否已存在
func (kf *KeywordFilter) containsKeyword(keyword string) bool {
	for _, k := range kf.keywords {
		if k == keyword {
			return true
		}
	}
	return false
}

// containsBlacklistKeyword 检查黑名单关键词是否已存在
func (kf *KeywordFilter) containsBlacklistKeyword(keyword string) bool {
	for _, k := range kf.blacklist {
		if k == keyword {
			return true
		}
	}
	return false
}

// GetKeywords 获取所有关键词
func (kf *KeywordFilter) GetKeywords() []string {
	kf.mutex.RLock()
	defer kf.mutex.RUnlock()

	result := make([]string, len(kf.keywords))
	copy(result, kf.keywords)
	return result
}

// GetBlacklist 获取所有黑名单关键词
func (kf *KeywordFilter) GetBlacklist() []string {
	kf.mutex.RLock()
	defer kf.mutex.RUnlock()

	result := make([]string, len(kf.blacklist))
	copy(result, kf.blacklist)
	return result
}

// GetCategory 获取关键词对应的分类
func (kf *KeywordFilter) GetCategory(keyword string) string {
	kf.mutex.RLock()
	defer kf.mutex.RUnlock()

	if category, ok := kf.categoryMap[strings.ToLower(keyword)]; ok {
		return category
	}
	return ""
}

// MatchContent 检查内容是否匹配关键词并返回匹配的关键词
func (kf *KeywordFilter) MatchContent(content string) (bool, string) {
	if content == "" {
		return false, ""
	}

	kf.mutex.RLock()
	defer kf.mutex.RUnlock()

	lowerContent := strings.ToLower(content)

	// 先检查黑名单
	for _, blacklisted := range kf.blacklist {
		if strings.Contains(lowerContent, blacklisted) {
			return false, ""
		}
	}

	// 检查白名单关键词
	for _, keyword := range kf.keywords {
		if strings.Contains(lowerContent, keyword) {
			return true, keyword
		}
	}

	return false, ""
}
