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

// A4GenerateResponse A4 生成响应
type A4GenerateResponse struct {
	Code int `json:"code"`
	Data struct {
		Id string `json:"id"`
	} `json:"data"`
	Message string `json:"message"`
}

// A4QueryResponse A4 查询响应
type A4QueryResponse struct {
	Code int `json:"code"`
	Data struct {
		Status int `json:"status"`
		Result struct {
			Data struct {
				Domain string `json:"domain"`
				Video  string `json:"video"`
				Image  string `json:"image"`
			} `json:"data"`
		} `json:"result"`
	} `json:"data"`
	Message string `json:"message"`
}

// A4WithTemplate A4 模板生成器（支持 Redis）
type A4WithTemplate struct {
	client util.HTTPClient
	redis  *redis.Client

	// A4 API 配置
	generateURL string
	queryURL    string
	headers     map[string]string

	// 模板管理
	templateMgr *template.Manager
}

func NewA4() *A4WithTemplate {
	// 初始化 Redis 客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	return &A4WithTemplate{
		client: util.DefaultClient(),
		redis:  redisClient,
		generateURL: config.A4GenerateURL,
		queryURL:    config.A4QueryURL,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.A4Token,
		},
	}
}

// Generate 通过模板ID生成视频
func (v *A4WithTemplate) Generate(args map[string]any) (string, error) {
	ctx := context.Background()

	// 获取 template_id
	templateID, ok := args["template_id"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 template_id 参数")
	}

	// 获取 image 参数（A4 也是图生视频）
	imageURL, ok := args["image"].(string)
	if !ok || imageURL == "" {
		return "", fmt.Errorf("缺少 image 参数（图片地址）")
	}

	// 获取模板详情
	tmpl, err := template.GetTemplate(templateID)
	if err != nil {
		return "", fmt.Errorf("获取模板失败: %w", err)
	}

	log.Printf("[A4] 模板 %s (%s)", tmpl.TemplateID, tmpl.Type)
	log.Printf("[A4] Video Parameters: %s", tmpl.VideoParameters)
	log.Printf("[A4] Image URL: %s", imageURL)

	// 生成任务ID
	taskID := fmt.Sprintf("a4_%d", time.Now().UnixNano())

	// 创建任务状态
	taskStatus := map[string]interface{}{
		"template_id":       templateID,
		"template_type":     tmpl.Type,
		"video_parameters":  tmpl.VideoParameters,
		"image_url":         imageURL,
		"status":            "pending",
		"a4_task_id":        "",
		"created_at":        time.Now(),
		"updated_at":        time.Now(),
	}

	// 保存到 Redis（24小时过期）
	statusJSON, err := json.Marshal(taskStatus)
	if err != nil {
		return "", err
	}

	taskKey := fmt.Sprintf("task:%s", taskID)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[A4] Redis 保存失败: %v", err)
		return "", fmt.Errorf("保存任务失败")
	}

	log.Printf("[A4] 任务 %s 已保存到 Redis", taskID)

	// 异步调用 A4 API
	go v.processTask(ctx, taskID, tmpl, imageURL)

	return taskID, nil
}

// processTask 异步处理任务
func (v *A4WithTemplate) processTask(ctx context.Context, taskID string, tmpl *template.Template, imageURL string) {
	// 更新状态为生成中
	v.updateTaskStatus(ctx, taskID, "generating", "")

	// 解析 VideoParameters JSON 字符串
	var a4Req map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl.VideoParameters), &a4Req); err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("解析 VideoParameters 失败: %v", err))
		return
	}

	// 注入 image 参数
	a4Req["image"] = imageURL

	// 序列化为 JSON
	payload, err := json.Marshal(a4Req)
	if err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("序列化请求失败: %v", err))
		return
	}

	log.Printf("[A4] 任务 %s - 调用 A4 API", taskID)
	log.Printf("[A4] 请求 URL: %s", v.generateURL)
	log.Printf("[A4] 请求 Headers:")
	for key, value := range v.headers {
		log.Printf("[A4]   %s: %s", key, value)
	}
	log.Printf("[A4] 请求 Body: %s", string(payload))

	// 调用 A4 生成视频
	resp, err := util.RequestWithClient[A4GenerateResponse](
		v.client,
		http.MethodPost,
		v.generateURL,
		v.headers,
		payload,
	)
	if err != nil {
		log.Printf("[A4] 调用 A4 失败: %v", err)
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("调用 A4 失败: %v", err))
		return
	}

	// 打印完整响应
	respJSON, _ := json.Marshal(resp)
	log.Printf("[A4] 响应 Body: %s", string(respJSON))
	log.Printf("[A4] 响应 Code: %d", resp.Code)
	log.Printf("[A4] 响应 Message: %s", resp.Message)
	log.Printf("[A4] 响应 TaskID: %s", resp.Data.Id)

	if resp.Data.Id == "" {
		v.updateTaskStatus(ctx, taskID, "failed", "A4 返回的任务 ID 为空")
		return
	}

	// 保存 A4 任务 ID
	v.updateTaskField(ctx, taskID, "a4_task_id", resp.Data.Id)

	log.Printf("[A4] 任务 %s - A4 任务ID: %s", taskID, resp.Data.Id)
}

// updateTaskStatus 更新任务状态
func (v *A4WithTemplate) updateTaskStatus(ctx context.Context, taskID, status, errorMsg string) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		log.Printf("[A4] 获取任务失败: %v", err)
		return
	}

	var taskStatus map[string]interface{}
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		log.Printf("[A4] 解析任务失败: %v", err)
		return
	}

	taskStatus["status"] = status
	taskStatus["error_message"] = errorMsg
	taskStatus["updated_at"] = time.Now()

	statusJSON, _ := json.Marshal(taskStatus)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[A4] 更新任务失败: %v", err)
	}
}

// updateTaskField 更新任务的某个字段
func (v *A4WithTemplate) updateTaskField(ctx context.Context, taskID, field, value string) {
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

// Query 查询任务状态
func (v *A4WithTemplate) Query(taskID string) (*TaskResponse, error) {
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

	// 如果还在生成中，查询 A4 状态
	a4TaskID, _ := taskStatus["a4_task_id"].(string)
	if taskStatus["status"] == "generating" && a4TaskID != "" {
		v.checkA4Status(ctx, taskID, a4TaskID)
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

// checkA4Status 检查 A4 视频生成状态
func (v *A4WithTemplate) checkA4Status(ctx context.Context, taskID, a4TaskID string) {
	url := fmt.Sprintf(v.queryURL, a4TaskID)

	log.Printf("[A4] 任务 %s - 查询 A4 状态", taskID)
	log.Printf("[A4] 查询 URL: %s", url)

	resp, err := util.RequestWithClient[A4QueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.headers,
		nil,
	)
	if err != nil {
		log.Printf("[A4] 查询失败: %v", err)
		return
	}

	// 打印完整响应
	respJSON, _ := json.Marshal(resp)
	log.Printf("[A4] 查询响应 Body: %s", string(respJSON))
	log.Printf("[A4] 响应 Code: %d", resp.Code)
	log.Printf("[A4] 响应 Message: %s", resp.Message)
	log.Printf("[A4] 响应 Status: %d", resp.Data.Status)

	// 如果完成，保存视频URL
	if resp.Data.Status == 2 {
		videoURL := resp.Data.Result.Data.Domain + resp.Data.Result.Data.Video
		imageURL := resp.Data.Result.Data.Domain + resp.Data.Result.Data.Image

		log.Printf("[A4] 任务 %s - 视频生成完成", taskID)
		log.Printf("[A4] 视频 URL: %s", videoURL)
		log.Printf("[A4] 封面 URL: %s", imageURL)

		// 重新获取任务状态
		taskKey := fmt.Sprintf("task:%s", taskID)
		statusData, _ := v.redis.Get(ctx, taskKey).Result()

		var taskStatus map[string]interface{}
		json.Unmarshal([]byte(statusData), &taskStatus)

		taskStatus["status"] = "completed"
		taskStatus["video_url"] = videoURL
		taskStatus["image_url"] = imageURL
		taskStatus["updated_at"] = time.Now()

		// 保存到 Redis
		statusJSON, _ := json.Marshal(taskStatus)
		v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)
	} else if resp.Data.Status == 3 {
		log.Printf("[A4] 任务 %s - 视频生成失败", taskID)
		v.updateTaskStatus(ctx, taskID, "failed", "视频生成失败")
	}
}
