package config

import (
	"os"
)

// getEnv 从环境变量获取配置，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

var (
	// Port 服务监听端口 (可通过环境变量 PORT 覆盖，默认 8083)
	Port = func() string {
		port := getEnv("PORT", "8083")
		return ":" + port
	}()

	// MasterDomain 主站API地址 (可通过环境变量 MASTER_DOMAIN 覆盖)
	MasterDomain = getEnv("MASTER_DOMAIN", "http://localhost:8000")

	// Aitubo API 基础路径
	// 开发环境: https://api-marmot.wenuts.top  69a2d2d40f3e36659fe6e90a
	// 生产环境: https://api.aitubo.ai  api-d3d2db2cfdd511f0b4b886b589842e42
	AituboAPIBase = getEnv("AITUBO_API_BASE", "https://api.aitubo.ai")

	A4Token = func() string {
		token := getEnv("A4_TOKEN", "")
		if token == "" {
			return "Bearer 69659a4ca433865c23f0138b"
		}
		if len(token) < 7 || token[:7] != "Bearer " {
			return "Bearer " + token
		}
		return token
	}()

	A4GenerateURL = AituboAPIBase + "/api/job/create-video"
	A4QueryURL    = AituboAPIBase + "/api/job/get-video?id=%s"

	// A2E API 配置
	A2eToken = getEnv("A2E_TOKEN", "sk_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJfaWQiOiI2OTVlMWNkMWZjNjczMDAwNjdlMjJkMDgiLCJuYW1lIjoidmljdG9yeXZpZGVvMDFAZ21haWwuY29tIiwicm9sZSI6ImNvaW4iLCJpYXQiOjE3NjgzNjM3ODF9.4Kgr7rGvdDUP2mc8S8_GxbAv4SkDBe418r3C7IIfawA")

	A2eGenerateURL = "https://video.a2e.ai/api/v1/userImage2Video/start"
	A2eQueryURL    = "https://video.a2e.ai/api/v1/userImage2Video/%s"

	// ShortAPI 配置
	ShortAPIKey = func() string {
		key := getEnv("SHORTAPI_KEY", "ak-40d93179fc2d11f0bc1bb6cdb4ddc449")
		if len(key) < 7 || key[:7] != "Bearer " {
			return "Bearer " + key
		}
		return key
	}()

	ShortAPIJobCreateURL = "https://api.shortapi.ai/api/v1/job/create"
	ShortAPIJobQueryURL  = "https://api.shortapi.ai/api/v1/job/query?id=%s"

	// Redis 配置 (用于存储任务状态)
	RedisAddr     = getEnv("REDIS_ADDR", "localhost:6379")
	RedisPassword = getEnv("REDIS_PASSWORD", "")
	RedisDB       = 0
)
