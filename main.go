package main

import (
	"fmt"
	"log"
	"net/http"
	"shortpress-sora-video/generator"
	"shortpress-sora-video/handler"
	"shortpress-sora-video/template"

	"github.com/gin-gonic/gin"
)

// GenerateRequest 视频生成请求
type GenerateRequest struct {
	TemplateID string         `json:"template_id" binding:"required"`
	Args       map[string]any `json:"args" binding:"required"`
}

// GenerateResponse 视频生成响应
type GenerateResponse struct {
	TaskID       string `json:"task_id"`
	TemplateID   string `json:"template_id"`
	TemplateType string `json:"template_type"`
}

func main() {
	// 加载模板文件
	if err := template.LoadTemplates("template_nsfw.json"); err != nil {
		log.Printf("警告: 加载模板失败: %v", err)
	} else {
		log.Printf("成功加载 %d 个模板", len(template.ListTemplates()))
	}

	// 创建统一的模板生成器
	soraGen := generator.NewSoraTemplate()

	// 创建 Gin 路由实例
	r := gin.Default()

	// 视频生成接口
	r.POST("/generate", func(c *gin.Context) {
		generate(c, soraGen)
	})

	// 任务查询接口
	r.GET("/query", func(c *gin.Context) {
		query(c, soraGen)
	})

	// 启动服务器
	port := ":8083"
	log.Printf("Sora视频生成服务启动，监听端口: %s", port)
	if err := r.Run(port); err != nil {
		panic("Failed to start server: " + err.Error())
	}
}

// generate 处理视频生成请求
func generate(c *gin.Context, gen generator.Generator) {
	var req GenerateRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		handler.BadRequest(c, "invalid_request", "请求参数格式错误: "+err.Error())
		return
	}

	// 准备生成参数
	generateArgs := make(map[string]any)
	generateArgs["template_id"] = req.TemplateID

	// 合并用户传入的参数
	for k, v := range req.Args {
		generateArgs[k] = v
	}

	// 调用生成器
	taskID, err := gen.Generate(generateArgs)
	if err != nil {
		handler.InternalServerError(c, "generate_failed", "视频生成失败: "+err.Error())
		return
	}

	// 解析 taskID 获取类型信息
	taskType, _, err := parseTaskID(taskID)
	if err != nil {
		handler.InternalServerError(c, "parse_taskid_failed", err.Error())
		return
	}

	handler.Success(c, http.StatusOK, GenerateResponse{
		TaskID:       taskID,
		TemplateID:   req.TemplateID,
		TemplateType: taskType,
	})
}

// query 查询任务状态
func query(c *gin.Context, gen generator.Generator) {
	taskID := c.Query("task_id")
	if taskID == "" {
		handler.BadRequest(c, "invalid_request", "缺少 task_id 参数")
		return
	}

	// 调用查询接口
	taskResp, err := gen.Query(taskID)
	if err != nil {
		handler.InternalServerError(c, "query_failed", "查询任务失败: "+err.Error())
		return
	}

	// 返回统一的任务响应格式
	handler.Success(c, http.StatusOK, taskResp)
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

	return "", "", fmt.Errorf("无法解析 taskID 的类型")
}
