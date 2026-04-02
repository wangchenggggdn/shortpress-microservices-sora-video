package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient HTTP 客户端接口，便于测试和扩展
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultClient 默认的 HTTP 客户端
var defaultClient = &http.Client{
	Timeout: 30 * time.Second,
}

// DefaultClient 返回默认的 HTTP 客户端实例
func DefaultClient() HTTPClient {
	return defaultClient
}

// Request 泛型 HTTP 请求函数
// 支持自定义请求方法、URL、请求头和请求体
// 自动处理 JSON 响应解析
func Request[T any](method, url string, headers map[string]string, body []byte) (*T, error) {
	return RequestWithClient[T](defaultClient, method, url, headers, body)
}

// RequestWithClient 使用自定义 HTTP 客户端发送请求
// 便于单元测试和自定义超时配置
func RequestWithClient[T any](client HTTPClient, method, url string, headers map[string]string, body []byte) (*T, error) {
	var result T

	// 创建请求
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体（用于错误信息）
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, errors.New(fmt.Sprintf("request failed, status code: %d, message: %s", resp.StatusCode, string(bodyBytes)))
	}

	// 解析 JSON 响应
	if err = json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("parse response JSON failed: %w, body: %s", err, string(bodyBytes))
	}

	return &result, nil
}
