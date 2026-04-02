package generator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"shortpress-sora-video/config"
	"shortpress-sora-video/util"
)

// ViduQ2ImageGenerateRequest ViduQ2 图片生视频请求
type ViduQ2ImageGenerateRequest struct {
	Model string `json:"model"`
	Args  struct {
		Mode              string `json:"mode"`
		Prompt            string `json:"prompt"`
		Duration          string `json:"duration"`
		Image             string `json:"image"`
		Resolution        string `json:"resolution"`
		MovementAmplitude string `json:"movement_amplitude"`
		GenerateAudio     bool   `json:"generate_audio"`
		VoiceID           string `json:"voice_id"`
	} `json:"args"`
	CallbackURL string `json:"callback_url,omitempty"`
}

// ViduQ2ImageGenerateResponse ViduQ2 生成响应
type ViduQ2ImageGenerateResponse struct {
	Code int `json:"code"`
	Data struct {
		JobID string `json:"job_id"`
	} `json:"data"`
	Message string `json:"message"`
}

// ViduQ2ImageQueryResponse ViduQ2 查询响应
type ViduQ2ImageQueryResponse struct {
	Code int `json:"code"`
	Data struct {
		ID         string      `json:"id"`
		KeyName    string      `json:"key_name"`
		Model      string      `json:"model"`
		Args       interface{} `json:"args"` // 可根据需要定义具体结构体
		Status     int32       `json:"status"`
		Credit     string      `json:"credit"`
		Refunded   bool        `json:"refunded"`
		CreatedAt  string      `json:"created_at"`
		UpdatedAt  string      `json:"updated_at"`
		FinishedAt string      `json:"finished_at"`
		Error      string      `json:"error"`
		Result     struct {
			Videos []struct {
				URL string `json:"url"`
			} `json:"videos"`
		} `json:"result"`
	} `json:"data"`

	Message string `json:"message"`
}

// ViduQ2Image ViduQ2 图片生视频生成器
type ViduQ2Image struct {
	client      util.HTTPClient
	generateURL string
	queryURL    string
	headers     map[string]string
}

// NewViduQ2Image 创建 ViduQ2 图片生视频生成器
func NewViduQ2Image() *ViduQ2Image {
	return &ViduQ2Image{
		client:      util.DefaultClient(),
		generateURL: config.ShortAPIJobCreateURL,
		queryURL:    config.ShortAPIJobQueryURL,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.ShortAPIKey,
		},
	}
}

// Generate 生成视频任务
// 返回任务 ID 和可能的错误
func (v *ViduQ2Image) Generate(args map[string]any) (string, error) {
	// 构建请求参数
	req := ViduQ2ImageGenerateRequest{
		Model: "vidu/vidu-q2/image-to-video",
	}

	// 解析 args 参数
	if mode, ok := args["mode"].(string); ok {
		req.Args.Mode = mode
	}
	if prompt, ok := args["prompt"].(string); ok {
		req.Args.Prompt = prompt
	}
	if duration, ok := args["duration"].(string); ok {
		req.Args.Duration = duration
	}
	if image, ok := args["image"].(string); ok {
		req.Args.Image = image
	}
	if resolution, ok := args["resolution"].(string); ok {
		req.Args.Resolution = resolution
	}
	if movementAmplitude, ok := args["movement_amplitude"].(string); ok {
		req.Args.MovementAmplitude = movementAmplitude
	}
	if generateAudio, ok := args["generate_audio"].(bool); ok {
		req.Args.GenerateAudio = generateAudio
	}
	if voiceID, ok := args["voice_id"].(string); ok {
		req.Args.VoiceID = voiceID
	}
	if callbackURL, ok := args["callback_url"].(string); ok {
		req.CallbackURL = callbackURL
	}

	// 序列化请求参数
	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("serialize request failed: %w", err)
	}

	// 发送生成请求
	resp, err := util.RequestWithClient[ViduQ2ImageGenerateResponse](
		v.client,
		http.MethodPost,
		v.generateURL,
		v.headers,
		payload,
	)
	if err != nil {
		return "", fmt.Errorf("call generate API failed: %w", err)
	}

	// 验证响应
	if resp.Code != 0 {
		return "", fmt.Errorf("generate video failed: %s", resp.Message)
	}

	// 验证任务 ID
	if resp.Data.JobID == "" {
		return "", fmt.Errorf("generate video failed: job_id is empty")
	}

	return resp.Data.JobID, nil
}

// Query 查询任务状态
// 返回统一的任务响应格式
func (v *ViduQ2Image) Query(taskID string) (*TaskResponse, error) {
	// 验证任务 ID
	if taskID == "" {
		return nil, fmt.Errorf("task_id cannot be empty")
	}

	// 构建查询 URL
	url := fmt.Sprintf(v.queryURL, taskID)

	// 发送查询请求
	resp, err := util.RequestWithClient[ViduQ2ImageQueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.headers,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("call query API failed: %w", err)
	}

	// 验证响应
	if resp.Code != 0 {
		return nil, fmt.Errorf("query job failed: %s", resp.Message)
	}

	// 转换为统一的任务响应格式
	taskResp := &TaskResponse{
		TaskID:   taskID,
		Status:   resp.Data.Status,
		ErrorMsg: resp.Data.Error,
		RawData:  resp, // 保留原始数据
	}

	if len(resp.Data.Result.Videos) > 0 {
		for _, v := range resp.Data.Result.Videos {
			taskResp.Videos = append(taskResp.Videos, VideoInfo{
				URL: v.URL,
			})
		}
	}

	return taskResp, nil
}
