package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"shortpress-sora-video/config"
	"shortpress-sora-video/template"
	"shortpress-sora-video/util"
	"time"

	"github.com/redis/go-redis/v9"
)

// ViduQ2GenerateResponse ViduQ2 生成响应
type ViduQ2GenerateResponse struct {
	Code int `json:"code"`
	Data struct {
		JobID string `json:"job_id"`
	} `json:"data"`
	Message string `json:"message"`
}

// ViduQ2QueryResponse ViduQ2 查询响应
type ViduQ2QueryResponse struct {
	Code int `json:"code"`
	Data struct {
		JobID  string `json:"job_id"`
		Status int32  `json:"status"`
		Error  string `json:"error"`
		Result struct {
			Videos []struct {
				URL string `json:"url"`
			} `json:"videos"`
		} `json:"result"`
	} `json:"data"`
	Message string `json:"message"`
}

// ViduQ2 ViduQ2 视频生成器（支持模板和 Redis）
type ViduQ2 struct {
	client      util.HTTPClient
	redis       *redis.Client
	generateURL string
	queryURL    string
	headers     map[string]string
}

// NewViduQ2 创建 ViduQ2 生成器
func NewViduQ2() *ViduQ2 {
	// 初始化 Redis 客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	return &ViduQ2{
		client:      util.DefaultClient(),
		redis:       redisClient,
		generateURL: config.ShortAPIJobCreateURL,
		queryURL:    config.ShortAPIJobQueryURL,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.ShortAPIKey,
		},
	}
}

// Generate 通过模板ID生成视频
func (v *ViduQ2) Generate(args map[string]any) (string, error) {
	ctx := context.Background()

	// 获取 template_id
	templateID, ok := args["template_id"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 template_id 参数")
	}

	// 获取 image 参数
	imageURL, ok := args["image"].(string)
	if !ok || imageURL == "" {
		return "", fmt.Errorf("缺少 image 参数（图片地址）")
	}

	// 获取模板详情
	tmpl, err := template.GetTemplate(templateID)
	if err != nil {
		return "", fmt.Errorf("获取模板失败: %w", err)
	}

	log.Printf("[ViduQ2] 模板 %s (%s)", tmpl.TemplateID, tmpl.Type)
	log.Printf("[ViduQ2] Video Parameters: %s", tmpl.VideoParameters)
	log.Printf("[ViduQ2] Image URL: %s", imageURL)

	// 生成任务ID
	taskID := fmt.Sprintf("viduq2_%d", time.Now().UnixNano())

	// 创建任务状态
	taskStatus := map[string]interface{}{
		"template_id":      templateID,
		"template_type":    tmpl.Type,
		"video_parameters": tmpl.VideoParameters,
		"image_url":        imageURL,
		"status":           "pending",
		"viduq2_job_id":    "",
		"created_at":       time.Now(),
		"updated_at":       time.Now(),
	}

	// 保存到 Redis（24小时过期）
	statusJSON, err := json.Marshal(taskStatus)
	if err != nil {
		return "", err
	}

	taskKey := fmt.Sprintf("task:%s", taskID)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[ViduQ2] Redis 保存失败: %v", err)
		return "", fmt.Errorf("保存任务失败")
	}

	log.Printf("[ViduQ2] 任务 %s 已保存到 Redis", taskID)

	// 异步调用 ViduQ2 API
	go v.processTask(ctx, taskID, tmpl, imageURL)

	return taskID, nil
}

// processTask 异步处理任务
func (v *ViduQ2) processTask(ctx context.Context, taskID string, tmpl *template.Template, imageURL string) {
	// 更新状态为生成中
	v.updateTaskStatus(ctx, taskID, "generating", "")

	// 解析 VideoParameters JSON 字符串
	var viduq2Req map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl.VideoParameters), &viduq2Req); err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("解析 VideoParameters 失败: %v", err))
		return
	}

	// 注入 image 参数到 args 中
	if args, ok := viduq2Req["args"].(map[string]interface{}); ok {
		args["image"] = imageURL
	} else {
		v.updateTaskStatus(ctx, taskID, "failed", "VideoParameters 格式错误：缺少 args 字段")
		return
	}

	// 序列化为 JSON
	payload, err := json.Marshal(viduq2Req)
	if err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("序列化请求失败: %v", err))
		return
	}

	log.Printf("[ViduQ2] 任务 %s - 调用 ViduQ2 API", taskID)
	log.Printf("[ViduQ2] 请求 URL: %s", v.generateURL)
	log.Printf("[ViduQ2] 请求 Headers:")
	for key, value := range v.headers {
		log.Printf("[ViduQ2]   %s: %s", key, value)
	}
	log.Printf("[ViduQ2] 请求 Body: %s", string(payload))

	// 调用 ViduQ2 生成视频
	resp, err := util.RequestWithClient[ViduQ2GenerateResponse](
		v.client,
		http.MethodPost,
		v.generateURL,
		v.headers,
		payload,
	)
	if err != nil {
		log.Printf("[ViduQ2] 调用 ViduQ2 失败: %v", err)
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("调用 ViduQ2 失败: %v", err))
		return
	}

	// 打印完整响应
	respJSON, _ := json.Marshal(resp)
	log.Printf("[ViduQ2] 响应 Body: %s", string(respJSON))
	log.Printf("[ViduQ2] 响应 Code: %d", resp.Code)
	log.Printf("[ViduQ2] 响应 Message: %s", resp.Message)
	log.Printf("[ViduQ2] 响应 JobID: %s", resp.Data.JobID)

	if resp.Data.JobID == "" {
		// 打印详细错误信息
		respJSON, _ := json.Marshal(resp)
		log.Printf("[ViduQ2] ViduQ2 API 错误响应: %s", string(respJSON))
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("ViduQ2 返回错误: code=%d, message=%s", resp.Code, resp.Message))
		return
	}

	// 保存 ViduQ2 任务 ID
	v.updateTaskField(ctx, taskID, "viduq2_job_id", resp.Data.JobID)

	log.Printf("[ViduQ2] 任务 %s - ViduQ2 JobID: %s", taskID, resp.Data.JobID)
}

// Query 查询任务状态
func (v *ViduQ2) Query(taskID string) (*TaskResponse, error) {
	ctx := context.Background()
	taskKey := fmt.Sprintf("task:%s", taskID)

	// 从 Redis 获取任务
	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %v", err)
	}

	var taskStatus map[string]interface{}
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return nil, fmt.Errorf("解析任务失败: %v", err)
	}

	// 如果还在生成中，查询 ViduQ2 状态
	viduq2JobID, _ := taskStatus["viduq2_job_id"].(string)
	if taskStatus["status"] == "generating" && viduq2JobID != "" {
		v.checkViduQ2Status(ctx, taskID, viduq2JobID)
		// 重新获取任务状态
		statusData, _ = v.redis.Get(ctx, taskKey).Result()
		json.Unmarshal([]byte(statusData), &taskStatus)
	}

	// 映射状态
	var status int32
	switch taskStatus["status"] {
	case "pending":
		status = 0
	case "generating":
		status = 1
	case "completed":
		status = 2
	case "failed":
		status = 3
	default:
		status = 0
	}

	// 安全地获取 error_message
	errorMsg := ""
	if msg, ok := taskStatus["error_message"].(string); ok {
		errorMsg = msg
	}

	// 构建响应
	taskResp := &TaskResponse{
		TaskID:   taskID,
		Status:   status,
		ErrorMsg: errorMsg,
	}

	// 如果完成且有视频URL，添加到响应中
	if status == 2 {
		if videoURL, ok := taskStatus["video_url"].(string); ok && videoURL != "" {
			taskResp.Videos = append(taskResp.Videos, VideoInfo{
				URL: videoURL,
			})
		}
	}

	return taskResp, nil
}

// checkViduQ2Status 检查 ViduQ2 视频生成状态
func (v *ViduQ2) checkViduQ2Status(ctx context.Context, taskID, viduq2JobID string) {
	url := fmt.Sprintf(v.queryURL, viduq2JobID)

	log.Printf("[ViduQ2] 任务 %s - 查询 ViduQ2 状态", taskID)
	log.Printf("[ViduQ2] 查询 URL: %s", url)

	resp, err := util.RequestWithClient[ViduQ2QueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.headers,
		nil,
	)
	if err != nil {
		log.Printf("[ViduQ2] 查询失败: %v", err)
		return
	}

	// 打印完整响应
	respJSON, _ := json.Marshal(resp)
	log.Printf("[ViduQ2] 查询响应 Body: %s", string(respJSON))
	log.Printf("[ViduQ2] 响应 Code: %d", resp.Code)
	log.Printf("[ViduQ2] 响应 Status: %d", resp.Data.Status)

	// 如果完成，保存视频URL
	if resp.Data.Status == 2 {
		if len(resp.Data.Result.Videos) > 0 && resp.Data.Result.Videos[0].URL != "" {
			videoURL := resp.Data.Result.Videos[0].URL

			// 重新获取任务状态
			taskKey := fmt.Sprintf("task:%s", taskID)
			statusData, _ := v.redis.Get(ctx, taskKey).Result()

			var taskStatus map[string]interface{}
			json.Unmarshal([]byte(statusData), &taskStatus)

			taskStatus["status"] = "completed"
			taskStatus["video_url"] = videoURL
			taskStatus["updated_at"] = time.Now()

			// 保存到 Redis
			statusJSON, _ := json.Marshal(taskStatus)
			v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)

			log.Printf("[ViduQ2] 任务 %s - 视频生成完成", taskID)
			log.Printf("[ViduQ2] 视频 URL: %s", videoURL)
		}
	} else if resp.Data.Status == 3 {
		log.Printf("[ViduQ2] 任务 %s - 视频生成失败", taskID)
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("视频生成失败: %s", resp.Data.Error))
	}
}

// updateTaskStatus 更新任务状态
func (v *ViduQ2) updateTaskStatus(ctx context.Context, taskID, status, errorMsg string) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		log.Printf("[ViduQ2] 获取任务失败: %v", err)
		return
	}

	var taskStatus map[string]interface{}
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		log.Printf("[ViduQ2] 解析任务失败: %v", err)
		return
	}

	taskStatus["status"] = status
	taskStatus["error_message"] = errorMsg
	taskStatus["updated_at"] = time.Now()

	statusJSON, _ := json.Marshal(taskStatus)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[ViduQ2] 更新任务失败: %v", err)
	}
}

// updateTaskField 更新任务的某个字段
func (v *ViduQ2) updateTaskField(ctx context.Context, taskID, field, value string) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		return
	}

	var taskStatus map[string]interface{}
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return
	}

	taskStatus[field] = value
	taskStatus["updated_at"] = time.Now()

	statusJSON, _ := json.Marshal(taskStatus)
	v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)
}
