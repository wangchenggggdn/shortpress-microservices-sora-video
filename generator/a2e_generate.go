package generator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"shortpress-sora-video/config"
	"shortpress-sora-video/util"
	"time"
)

type a2eGenerateGenerateResponse struct {
	Code int `json:"code"`
	Data struct {
		Id string `json:"_id"`
	} `json:"data"`
}

type a2eGenerateQueryResponse struct {
	Code int `json:"code"`
	Data struct {
		Id             string    `json:"_id"`
		Name           string    `json:"name"`
		Duration       int       `json:"duration"`
		VideoUrl       string    `json:"video_url"`
		ImageUrl       string    `json:"image_url"`
		CurrentStatus  string    `json:"current_status"`
		ResultUrl      string    `json:"result_url"`
		ErrorCode      string    `json:"error_code"`
		FaildMessage   string    `json:"faild_message"`
		CreatedAt      time.Time `json:"createdAt"`
		UpdatedAt      time.Time `json:"updatedAt"`
		UserId         string    `json:"user_id"`
		CoverUrl       string    `json:"cover_url"`
		Coins          int       `json:"coins"`
		HasRefundCoin  bool      `json:"hasRefundCoin"`
		RemainingDays  int       `json:"remainingDays"`
		ExpirationDate time.Time `json:"expirationDate"`
		IsExpired      bool      `json:"isExpired"`
		ExpirationDays int       `json:"expirationDays"`
	} `json:"data"`
	TraceId string `json:"trace_id"`
}

type A2eGenerate struct {
	client      util.HTTPClient
	generateURL string
	queryURL    string
	headers     map[string]string
}

func NewA2eGenerate() *A2eGenerate {
	return &A2eGenerate{
		client:      util.DefaultClient(),
		generateURL: config.A2eGenerateURL,
		queryURL:    config.A2eQueryURL,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": config.A2eToken,
		},
	}
}

// Generate 生成视频任务
// 返回任务 ID 和可能的错误
func (v *A2eGenerate) Generate(args map[string]any) (string, error) {
	// 序列化请求参数
	payload, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("serialize request failed: %w", err)
	}

	// 发送生成请求
	resp, err := util.RequestWithClient[a2eGenerateGenerateResponse](
		v.client,
		http.MethodPost,
		v.generateURL,
		v.headers,
		payload,
	)
	if err != nil {
		return "", fmt.Errorf("call generate API failed: %w", err)
	}

	// 验证响应状态
	if resp.Code != 0 {
		return "", fmt.Errorf("generate video failed with code: %d", resp.Code)
	}

	// 验证任务 ID
	if resp.Data.Id == "" {
		return "", fmt.Errorf("generate video failed: task_id is empty")
	}

	return resp.Data.Id, nil
}

// Query 查询任务状态
// 返回统一的任务响应格式
func (v *A2eGenerate) Query(taskID string) (*TaskResponse, error) {
	// 验证任务 ID
	if taskID == "" {
		return nil, fmt.Errorf("task_id cannot be empty")
	}

	// 构建查询 URL
	url := fmt.Sprintf(v.queryURL, taskID)

	// 发送查询请求
	resp, err := util.RequestWithClient[a2eGenerateQueryResponse](
		v.client,
		http.MethodGet,
		url,
		v.headers,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("call query API failed: %w", err)
	}

	var status int32
	if resp.Data.CurrentStatus == "completed" {
		status = 2
	}
	if resp.Data.CurrentStatus == "failed" {
		status = 3
	}

	// 转换为统一的任务响应格式
	taskResp := &TaskResponse{
		TaskID:  taskID,
		Status:  status,
		RawData: resp, // 保留原始数据
	}

	// 处理错误信息
	if resp.Data.CurrentStatus == "failed" {
		taskResp.ErrorMsg = "failed"
	}

	taskResp.Videos = append(taskResp.Videos, VideoInfo{
		URL:      resp.Data.ResultUrl,
		CoverURL: resp.Data.CoverUrl,
	})

	return taskResp, nil
}
