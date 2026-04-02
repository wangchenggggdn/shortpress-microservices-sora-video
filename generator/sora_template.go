package generator

import (
	"fmt"
	"shortpress-sora-video/template"
)

// SoraTemplate 统一的模板生成器
// 根据 template_id 获取模板，根据 type 路由到对应的生成器
type SoraTemplate struct {
	// 保存所有可用的生成器
	generators map[string]Generator
}

func NewSoraTemplate() *SoraTemplate {
	return &SoraTemplate{
		generators: map[string]Generator{
			"image_to_video_a4":              NewA4(),
			"image_to_video_a2e_s_e_viduq2":  NewA2eSEViduQ2(),  // 首尾帧生成器
			"image_to_video_viduq2":           NewViduQ2(),
			"image_to_video_a2e_viduq2":       NewA2eViduQ2(),    // A2E+ViduQ2 两步生成器
			"image_to_video_a2e_a4":           NewA2eA4(),        // A2E+A4 两步生成器
		},
	}
}

// Generate 根据 template_id 获取模板并调用对应的生成器
func (s *SoraTemplate) Generate(args map[string]any) (string, error) {
	// 从 args 中获取 template_id
	templateID, ok := args["template_id"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 template_id 参数")
	}

	// 获取模板详情
	tmpl, err := template.GetTemplate(templateID)
	if err != nil {
		return "", fmt.Errorf("获取模板失败: %w", err)
	}

	// 根据模板的 type 获取对应的生成器
	gen, ok := s.generators[tmpl.Type]
	if !ok {
		return "", fmt.Errorf("不支持的模板类型: %s", tmpl.Type)
	}

	// 准备生成参数
	generateArgs := make(map[string]any)

	// 传递 template_id 给底层生成器
	generateArgs["template_id"] = templateID

	// 合并用户传入的参数
	for k, v := range args {
		generateArgs[k] = v
	}

	// 调用对应的生成器
	taskID, err := gen.Generate(generateArgs)
	if err != nil {
		return "", fmt.Errorf("生成视频失败: %w", err)
	}

	// 将类型信息编码到 taskID 中，格式: "type:actual_taskid"
	return fmt.Sprintf("%s:%s", tmpl.Type, taskID), nil
}

// Query 查询任务状态
// taskID 格式: "type:actual_taskid"
func (s *SoraTemplate) Query(taskID string) (*TaskResponse, error) {
	// 解析 taskID
	taskType, actualTaskID, err := parseTaskID(taskID)
	if err != nil {
		return nil, err
	}

	// 获取对应的生成器
	gen, ok := s.generators[taskType]
	if !ok {
		return nil, fmt.Errorf("不支持的模板类型: %s", taskType)
	}

	// 调用查询接口
	return gen.Query(actualTaskID)
}

// parseTaskID 从 taskID 中解析出类型和实际任务ID
// 格式: "type:actual_taskid" 例如: "image_to_video_a4:123456"
func parseTaskID(taskID string) (string, string, error) {
	// 支持的 5 种类型
	types := []string{
		"image_to_video_a4",
		"image_to_video_a2e_s_e_viduq2",
		"image_to_video_viduq2",
		"image_to_video_a2e_viduq2",
		"image_to_video_a2e_a4",
	}

	for _, typePrefix := range types {
		prefix := typePrefix + ":"
		if len(taskID) > len(prefix) && taskID[:len(prefix)] == prefix {
			return typePrefix, taskID[len(prefix):], nil
		}
	}

	return "", "", fmt.Errorf("无法解析 taskID 的类型，请使用格式 'type:actual_taskid'")
}
