package utils

import (
	"fmt"
	"strings"
)

// GetFileURL 获取文件URL
func GetFileURL(baseURL, filePath string) string {
	// 如果文件URL以 http 开头，则直接返回
	if strings.HasPrefix(filePath, "http") {
		return filePath
	}

	// baseURL 以 / 结尾，则去掉/
	baseURL = strings.TrimSuffix(baseURL, "/")
	// filePath 以 / 开头，则去掉/
	filePath = strings.TrimPrefix(filePath, "/")

	// 否则返回本地文件URL
	fullURL := fmt.Sprintf("%s/%s", baseURL, filePath)
	return fullURL
}
