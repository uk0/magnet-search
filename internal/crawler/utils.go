package crawler

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// 从bencode字典中计算infohash
func calculateInfoHash(info map[string]interface{}) (string, error) {
	var buffer bytes.Buffer
	err := writeBencodedDict(&buffer, info)
	if err != nil {
		return "", err
	}

	hasher := sha1.New()
	hasher.Write(buffer.Bytes())
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// 将bencode字典写入io.Writer
func writeBencodedDict(writer io.Writer, dict map[string]interface{}) error {
	// 字典前缀'd'
	if _, err := writer.Write([]byte{'d'}); err != nil {
		return err
	}

	// 按键排序
	keys := make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 写入键值对
	for _, k := range keys {
		v := dict[k]

		// 写入键
		if _, err := writer.Write([]byte(strconv.Itoa(len(k)))); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{':'}); err != nil {
			return err
		}
		if _, err := writer.Write([]byte(k)); err != nil {
			return err
		}

		// 写入值
		if err := writeBencodedValue(writer, v); err != nil {
			return err
		}
	}

	// 字典后缀'e'
	if _, err := writer.Write([]byte{'e'}); err != nil {
		return err
	}

	return nil
}

// 写入bencode值
func writeBencodedValue(writer io.Writer, value interface{}) error {
	switch v := value.(type) {
	case string:
		// 字符串: <长度>:<内容>
		if _, err := writer.Write([]byte(strconv.Itoa(len(v)))); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{':'}); err != nil {
			return err
		}
		if _, err := writer.Write([]byte(v)); err != nil {
			return err
		}
	case int:
		// 整数: i<整数值>e
		if _, err := writer.Write([]byte{'i'}); err != nil {
			return err
		}
		if _, err := writer.Write([]byte(strconv.Itoa(v))); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{'e'}); err != nil {
			return err
		}
	case int64:
		// 整数: i<整数值>e
		if _, err := writer.Write([]byte{'i'}); err != nil {
			return err
		}
		if _, err := writer.Write([]byte(strconv.FormatInt(v, 10))); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{'e'}); err != nil {
			return err
		}
	case []interface{}:
		// 列表: l<元素1><元素2>...e
		if _, err := writer.Write([]byte{'l'}); err != nil {
			return err
		}
		for _, item := range v {
			if err := writeBencodedValue(writer, item); err != nil {
				return err
			}
		}
		if _, err := writer.Write([]byte{'e'}); err != nil {
			return err
		}
	case map[string]interface{}:
		// 字典
		return writeBencodedDict(writer, v)
	default:
		return fmt.Errorf("不支持的类型: %T", v)
	}

	return nil
}
