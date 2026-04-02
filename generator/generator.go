package generator

// VideoInfo 视频信息
type VideoInfo struct {
	URL      string `json:"url,omitempty"`
	CoverURL string `json:"cover_url,omitempty"`
}

// TaskResponse 统一的视频任务响应格式
type TaskResponse struct {
	TaskID   string      `json:"task_id"`
	Status   int32       `json:"status"`
	ErrorMsg string      `json:"error_msg,omitempty"`
	Videos   []VideoInfo `json:"videos,omitempty"`
	Images   []string    `json:"images,omitempty"`
	RawData  interface{} `json:"raw_data,omitempty"` // 保留原始数据用于调试
}

// Generator 视频生成器接口
type Generator interface {
	Generate(args map[string]any) (string, error)
	Query(taskID string) (*TaskResponse, error)
}
