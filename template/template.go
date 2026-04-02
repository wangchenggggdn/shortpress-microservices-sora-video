package template

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Template 视频生成模板
type Template struct {
	TemplateID       string `json:"templateId"`
	ImageParameters  string `json:"imageParameters"`
	VideoParameters  string `json:"videoParameters"`
	Type             string `json:"type"`
	VideoURL         string `json:"videoUrl"`
	Title            string `json:"title"`
	Desc             string `json:"desc"`
}

// Manager 模板管理器
type Manager struct {
	templates map[string]*Template
	mu        sync.RWMutex
}

var (
	globalManager *Manager
	once          sync.Once
)

// LoadTemplates 从 JSON 文件加载模板
func LoadTemplates(path string) error {
	once.Do(func() {
		globalManager = &Manager{
			templates: make(map[string]*Template),
		}
	})

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}

	var list []Template
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("解析模板文件失败: %w", err)
	}

	globalManager.mu.Lock()
	defer globalManager.mu.Unlock()

	// 清空现有模板
	globalManager.templates = make(map[string]*Template)

	// 加载新模板
	for i := range list {
		globalManager.templates[list[i].TemplateID] = &list[i]
	}

	return nil
}

// GetTemplate 根据 ID 获取模板
func GetTemplate(id string) (*Template, error) {
	if globalManager == nil {
		return nil, fmt.Errorf("模板管理器未初始化")
	}

	globalManager.mu.RLock()
	defer globalManager.mu.RUnlock()

	tmpl, ok := globalManager.templates[id]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", id)
	}
	return tmpl, nil
}

// ListTemplates 获取所有模板列表
func ListTemplates() []*Template {
	if globalManager == nil {
		return nil
	}

	globalManager.mu.RLock()
	defer globalManager.mu.RUnlock()

	list := make([]*Template, 0, len(globalManager.templates))
	for _, tmpl := range globalManager.templates {
		list = append(list, tmpl)
	}
	return list
}

// HasTemplate 检查模板是否存在
func HasTemplate(id string) bool {
	if globalManager == nil {
		return false
	}

	globalManager.mu.RLock()
	defer globalManager.mu.RUnlock()

	_, ok := globalManager.templates[id]
	return ok
}
