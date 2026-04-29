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

// ==================== 任务状态定义 ====================

type TaskStatus struct {
	ImageTaskID     string    `json:"image_task_id"`
	VideoTaskID     string    `json:"video_task_id"`
	ImageURL        string    `json:"image_url"`
	ImageParameters string    `json:"image_parameters"`
	VideoParameters string    `json:"video_parameters"`
	Status          string    `json:"status"` // pending, generating_image, generating_video, completed, failed
	ErrorMessage    string    `json:"error_message"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ==================== A2E 图片生成相关 ====================

type a2eImageGenerateRequest struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	ModelType string `json:"model_type"`
	MaxImages int    `json:"max_images"`
}

type a2eImageGenerateResponse struct {
	Code int `json:"code"`
	Data []struct {
		Id string `json:"_id"`
	} `json:"data"`
}

type a2eImageQueryResponse struct {
	Code int `json:"code"`
	Data struct {
		Id            string   `json:"_id"`
		ImageUrls     []string `json:"image_urls"`
		CurrentStatus string   `json:"current_status"`
		FailedMessage string   `json:"failed_message"`
	} `json:"data"`
}

// ==================== ViduQ2 视频生成相关 ====================

type viduq2ImageGenerateRequest struct {
	Model string `json:"model"`
	Args  struct {
		Mode              string `json:"mode"`
		Prompt            string `json:"prompt"`
		Duration          string `json:"duration"`
		Image             string `json:"image"`
		Resolution        string `json:"resolution"`
		MovementAmplitude string `json:"movement_amplitude"`
		SafetyChecker     bool   `json:"safety_checker"`
	} `json:"args"`
}

type viduq2ImageGenerateResponse struct {
	Code int `json:"code"`
	Data struct {
		JobID string `json:"job_id"`
	} `json:"data"`
	Message string `json:"message"`
}

type viduq2ImageQueryResponse struct {
	Code int `json:"code"`
	Data struct {
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

// ==================== A2eViduQ2 生成器 ====================

type A2eViduQ2 struct {
	client    util.HTTPClient
	redis     *redis.Client
	a2eURL    string
	viduq2URL string

	// A2E 配置
	a2eGenerateURL string
	a2eQueryURL    string
	a2eHeaders     map[string]string

	// ViduQ2 配置
	viduq2GenerateURL string
	viduq2QueryURL    string
	viduq2Headers     map[string]string
}

func NewA2eViduQ2() *A2eViduQ2 {
	// 初始化 Redis 客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	return &A2eViduQ2{
		client:    util.DefaultClient(),
		redis:     redisClient,
		a2eURL:    "https://video.a2e.ai/api/v1/userText2image",
		viduq2URL: config.ShortAPIJobCreateURL,

		a2eGenerateURL: "https://video.a2e.ai/api/v1/userText2image/start",
		a2eQueryURL:    "https://video.a2e.ai/api/v1/userText2image/%s",
		a2eHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.A2eToken,
		},
		viduq2GenerateURL: config.ShortAPIJobCreateURL,
		viduq2QueryURL:    config.ShortAPIJobQueryURL,
		viduq2Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.ShortAPIKey,
		},
	}
}

// Generate 创建任务，立即返回任务ID
func (v *A2eViduQ2) Generate(args map[string]any) (string, error) {
	ctx := context.Background()

	// 打印用户传入的所有参数
	argsJSON, _ := json.Marshal(args)
	log.Printf("[A2eViduQ2] 用户传入参数: %s", string(argsJSON))

	// 获取 template_id
	templateID, ok := args["template_id"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 template_id 参数")
	}

	// 获取 input_images（用户提供的图片）
	// 优先使用 args["input_images"]，兼容旧的 args["image"]
	var inputImages string
	if v, ok := args["input_images"]; ok {
		switch vv := v.(type) {
		case string:
			inputImages = vv
		case []interface{}:
			if len(vv) > 0 {
				if s, ok := vv[0].(string); ok {
					inputImages = s
				}
			}
		}
	} else if v, ok := args["image"].(string); ok {
		// 兼容旧字段
		inputImages = v
	}

	hasInputImages := inputImages != ""
	if hasInputImages {
		log.Printf("[A2eViduQ2] 用户提供的图片: %s", inputImages)
	} else {
		log.Printf("[A2eViduQ2] 未提供 input_images/image 参数，将使用文生图模式")
	}

	// 获取模板详情
	tmpl, err := template.GetTemplate(templateID)
	if err != nil {
		return "", fmt.Errorf("获取模板失败: %w", err)
	}

	log.Printf("[A2eViduQ2] 模板 %s (%s)", tmpl.TemplateID, tmpl.Type)

	// 生成任务ID
	taskID := fmt.Sprintf("a2e_viduq2_%d", time.Now().UnixNano())

	// 如果有 input_images，需要将其注入到 ImageParameters 中
	imageParameters := tmpl.ImageParameters
	if hasInputImages && inputImages != "" {
		// 解析 ImageParameters
		var imgParams map[string]interface{}
		if err := json.Unmarshal([]byte(tmpl.ImageParameters), &imgParams); err == nil {
			// 添加 input_images 字段（作为数组）
			imgParams["input_images"] = []string{inputImages}
			// 重新序列化
			if newParams, err := json.Marshal(imgParams); err == nil {
				imageParameters = string(newParams)
				log.Printf("[A2eViduQ2] 已注入 input_images 到 ImageParameters (数组格式)")
			}
		}
	}

	// 创建任务状态
	status := &TaskStatus{
		Status:          "pending",
		ImageParameters: imageParameters,
		VideoParameters: tmpl.VideoParameters,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// 保存到 Redis（24小时过期）
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return "", err
	}

	taskKey := fmt.Sprintf("task:%s", taskID)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[A2eViduQ2] Redis 保存失败: %v", err)
		return "", fmt.Errorf("保存任务失败")
	}

	// 异步处理任务
	go v.processTask(taskID, args)

	log.Printf("[A2eViduQ2] 创建任务 %s", taskID)

	return taskID, nil
}

// processTask 异步处理任务
func (v *A2eViduQ2) processTask(taskID string, args map[string]any) {
	ctx := context.Background()

	// 从 Redis 获取任务状态
	taskKey := fmt.Sprintf("task:%s", taskID)
	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		log.Printf("[A2eViduQ2] 获取任务失败: %v", err)
		v.updateTaskStatus(ctx, taskID, "failed", "获取任务状态失败")
		return
	}

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		log.Printf("[A2eViduQ2] 解析任务失败: %v", err)
		v.updateTaskStatus(ctx, taskID, "failed", "解析任务状态失败")
		return
	}

	// 步骤1：生成图片
	log.Printf("[A2eViduQ2] 任务 %s - 步骤1：开始生成图片", taskID)
	log.Printf("[A2eViduQ2] Image Parameters: %s", taskStatus.ImageParameters)

	// 更新状态为生成图片中
	v.updateTaskStatus(ctx, taskID, "generating_image", "")

	// 直接使用 ImageParameters JSON 字符串作为 A2E 请求体
	payload := []byte(taskStatus.ImageParameters)

	log.Printf("[A2eViduQ2] 调用 A2E API: %s", v.a2eGenerateURL)
	log.Printf("[A2eViduQ2] 请求 Body: %s", string(payload))

	// 调用 A2E
	a2eResp, err := util.RequestWithClient[a2eImageGenerateResponse](
		v.client,
		http.MethodPost,
		v.a2eGenerateURL,
		v.a2eHeaders,
		payload,
	)
	if err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("调用 A2E 失败: %v", err))
		return
	}

	if len(a2eResp.Data) == 0 || a2eResp.Data[0].Id == "" {
		v.updateTaskStatus(ctx, taskID, "failed", "A2E 返回的任务 ID 为空")
		return
	}

	imageTaskID := a2eResp.Data[0].Id

	// 保存 imageTaskID 到 Redis
	v.updateTaskField(ctx, taskID, "image_task_id", imageTaskID)
	log.Printf("[A2eViduQ2] 任务 %s - 图片任务 ID: %s", taskID, imageTaskID)

	// 轮询图片生成状态
	for {
		// 检查图片状态
		imageURL := v.pollImageStatus(ctx, taskID, imageTaskID)
		if imageURL != "" {
			// 图片生成成功，开始生成视频
			v.startVideoGeneration(ctx, taskID, imageURL)
			break
		} else if v.isImageFailed(ctx, taskID) {
			// 图片生成失败
			log.Printf("[A2eViduQ2] 任务 %s - 图片生成失败", taskID)
			break
		}

		// 等待 5 秒后重试
		time.Sleep(5 * time.Second)
	}
}

// pollImageStatus 轮询图片生成状态，返回图片URL（如果完成）
func (v *A2eViduQ2) pollImageStatus(ctx context.Context, taskID, imageTaskID string) string {
	url := fmt.Sprintf(v.a2eQueryURL, imageTaskID)

	resp, err := util.RequestWithClient[a2eImageQueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.a2eHeaders,
		nil,
	)
	if err != nil {
		log.Printf("[A2eViduQ2] 查询 A2E 失败: %v", err)
		return ""
	}

	log.Printf("[A2eViduQ2] A2E 图片状态: %s", resp.Data.CurrentStatus)

	if resp.Data.CurrentStatus == "completed" && len(resp.Data.ImageUrls) > 0 {
		imageURL := resp.Data.ImageUrls[0]
		log.Printf("[A2eViduQ2] 任务 %s - 图片生成完成", taskID)
		log.Printf("[A2eViduQ2] 任务 %s - A2E 生成的图片地址: %s", taskID, imageURL)
		return imageURL
	} else if resp.Data.CurrentStatus == "failed" {
		// 图片生成失败，更新任务状态
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("图片生成失败: %s", resp.Data.FailedMessage))
	}

	return ""
}

// isImageFailed 检查图片是否生成失败
func (v *A2eViduQ2) isImageFailed(ctx context.Context, taskID string) bool {
	taskKey := fmt.Sprintf("task:%s", taskID)
	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		return false
	}

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return false
	}

	// 检查 Redis 中是否已标记为失败
	// （可能由其他线程或之前的查询更新）
	return taskStatus.Status == "failed"
}

// pollVideoStatus 轮询视频生成状态，返回视频URL（如果完成）
func (v *A2eViduQ2) pollVideoStatus(ctx context.Context, taskID, videoTaskID string) string {
	url := fmt.Sprintf(v.viduq2QueryURL, videoTaskID)

	resp, err := util.RequestWithClient[viduq2ImageQueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.viduq2Headers,
		nil,
	)
	if err != nil {
		log.Printf("[A2eViduQ2] 查询 ViduQ2 失败: %v", err)
		return ""
	}

	log.Printf("[A2eViduQ2] ViduQ2 视频状态: %d", resp.Data.Status)

	// Status: 0=processing, 1=succeed, 2=completed, 3=failed
	if resp.Data.Status == 2 {
		if len(resp.Data.Result.Videos) > 0 && resp.Data.Result.Videos[0].URL != "" {
			videoURL := resp.Data.Result.Videos[0].URL

			// 更新任务状态为完成
			taskKey := fmt.Sprintf("task:%s", taskID)
			statusData, _ := v.redis.Get(ctx, taskKey).Result()

			var taskStatus TaskStatus
			json.Unmarshal([]byte(statusData), &taskStatus)

			taskStatus.Status = "completed"
			taskStatus.ImageURL = videoURL // 使用 ImageURL 字段存储视频 URL
			taskStatus.UpdatedAt = time.Now()

			statusJSON, _ := json.Marshal(taskStatus)
			v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)

			return videoURL
		}
	} else if resp.Data.Status == 3 {
		// 视频生成失败，更新任务状态
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("视频生成失败: %s", resp.Data.Error))
	}

	return ""
}

// isVideoFailed 检查视频是否生成失败
func (v *A2eViduQ2) isVideoFailed(ctx context.Context, taskID string) bool {
	taskKey := fmt.Sprintf("task:%s", taskID)
	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		return false
	}

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return false
	}

	return taskStatus.Status == "failed"
}

// updateTaskStatus 更新任务状态
func (v *A2eViduQ2) updateTaskStatus(ctx context.Context, taskID, status, errorMsg string) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	// 获取现有任务
	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		log.Printf("[A2eViduQ2] 获取任务失败: %v", err)
		return
	}

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		log.Printf("[A2eViduQ2] 解析任务失败: %v", err)
		return
	}

	// 更新字段
	taskStatus.Status = status
	taskStatus.ErrorMessage = errorMsg
	taskStatus.UpdatedAt = time.Now()

	// 保存回 Redis
	statusJSON, _ := json.Marshal(taskStatus)
	if err := v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("[A2eViduQ2] 更新任务失败: %v", err)
	}
}

// updateTaskField 更新任务的某个字段
func (v *A2eViduQ2) updateTaskField(ctx context.Context, taskID, field, value string) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	statusData, err := v.redis.Get(ctx, taskKey).Result()
	if err != nil {
		return
	}

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return
	}

	// 使用反射或其他方式更新字段
	// 这里简化处理，只更新特定字段
	switch field {
	case "image_task_id":
		taskStatus.ImageTaskID = value
	case "video_task_id":
		taskStatus.VideoTaskID = value
	case "image_url":
		taskStatus.ImageURL = value
	}

	taskStatus.UpdatedAt = time.Now()
	statusJSON, _ := json.Marshal(taskStatus)
	v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)
}

// Query 查询任务状态
func (v *A2eViduQ2) Query(taskID string) (*TaskResponse, error) {
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

	var taskStatus TaskStatus
	if err := json.Unmarshal([]byte(statusData), &taskStatus); err != nil {
		return nil, fmt.Errorf("解析任务失败: %v", err)
	}

	// 根据状态决定是否需要查询上游
	if taskStatus.Status == "generating_image" && taskStatus.ImageTaskID != "" {
		// 查询 A2E 图片状态
		imageURL := v.pollImageStatus(ctx, taskID, taskStatus.ImageTaskID)
		if imageURL != "" {
			// 重新获取任务状态
			statusData, _ = v.redis.Get(ctx, taskKey).Result()
			json.Unmarshal([]byte(statusData), &taskStatus)
		}
	} else if taskStatus.Status == "generating_video" && taskStatus.VideoTaskID != "" {
		// 查询 ViduQ2 视频状态
		videoURL := v.pollVideoStatus(ctx, taskID, taskStatus.VideoTaskID)
		if videoURL != "" {
			// 重新获取任务状态
			statusData, _ = v.redis.Get(ctx, taskKey).Result()
			json.Unmarshal([]byte(statusData), &taskStatus)
		}
	}

	// 映射状态
	var status int32
	switch taskStatus.Status {
	case "pending", "generating_image":
		status = 0
	case "generating_video":
		status = 1
	case "completed":
		status = 2
	case "failed":
		status = 3
	}

	// 构建响应
	taskResp := &TaskResponse{
		TaskID:   taskID,
		Status:   status,
		ErrorMsg: taskStatus.ErrorMessage,
	}

	// 如果完成且有视频URL，添加到响应中
	if status == 2 && taskStatus.ImageURL != "" {
		taskResp.Videos = append(taskResp.Videos, VideoInfo{
			URL: taskStatus.ImageURL,
		})
	}

	return taskResp, nil
}

// startVideoGeneration 开始生成视频
func (v *A2eViduQ2) startVideoGeneration(ctx context.Context, taskID, imageURL string) {
	// 获取任务
	taskKey := fmt.Sprintf("task:%s", taskID)
	statusData, _ := v.redis.Get(ctx, taskKey).Result()

	var taskStatus TaskStatus
	json.Unmarshal([]byte(statusData), &taskStatus)

	log.Printf("[A2eViduQ2] 任务 %s - 步骤2：开始生成视频", taskID)
	log.Printf("[A2eViduQ2] Video Parameters: %s", taskStatus.VideoParameters)

	// 解析 VideoParameters JSON 字符串
	var viduq2Req viduq2ImageGenerateRequest
	if err := json.Unmarshal([]byte(taskStatus.VideoParameters), &viduq2Req); err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("解析 VideoParameters 失败: %v", err))
		return
	}

	// 设置图片 URL
	viduq2Req.Args.Image = imageURL
	viduq2Req.Args.SafetyChecker = false

	payload, _ := json.Marshal(viduq2Req)

	log.Printf("[A2eViduQ2] 调用 ViduQ2 API: %s", v.viduq2GenerateURL)
	log.Printf("[A2eViduQ2] 请求 Body: %s", string(payload))

	// 调用 ViduQ2
	viduq2Resp, err := util.RequestWithClient[viduq2ImageGenerateResponse](
		v.client,
		http.MethodPost,
		v.viduq2GenerateURL,
		v.viduq2Headers,
		payload,
	)
	if err != nil {
		v.updateTaskStatus(ctx, taskID, "failed", fmt.Sprintf("调用 ViduQ2 失败: %v", err))
		return
	}

	if viduq2Resp.Code != 0 || viduq2Resp.Data.JobID == "" {
		v.updateTaskStatus(ctx, taskID, "failed", "ViduQ2 返回错误")
		return
	}

	// 更新状态
	taskStatus.Status = "generating_video"
	taskStatus.VideoTaskID = viduq2Resp.Data.JobID
	taskStatus.ImageURL = imageURL
	taskStatus.UpdatedAt = time.Now()

	statusJSON, _ := json.Marshal(taskStatus)
	v.redis.Set(ctx, taskKey, statusJSON, 24*time.Hour)

	log.Printf("[A2eViduQ2] 任务 %s - 视频任务 ID: %s", taskID, viduq2Resp.Data.JobID)

	// 轮询视频生成状态
	for {
		videoURL := v.pollVideoStatus(ctx, taskID, viduq2Resp.Data.JobID)
		if videoURL != "" {
			// 视频生成成功
			log.Printf("[A2eViduQ2] 任务 %s - 视频生成完成", taskID)
			log.Printf("[A2eViduQ2] 视频 URL: %s", videoURL)
			break
		} else if v.isVideoFailed(ctx, taskID) {
			// 视频生成失败
			log.Printf("[A2eViduQ2] 任务 %s - 视频生成失败", taskID)
			break
		}

		// 等待 5 秒后重试
		time.Sleep(5 * time.Second)
	}
}
