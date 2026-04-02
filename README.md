# Sora Video 微服务使用说明

## 📋 概述

sora_video 微服务提供模板化视频生成功能，支持通过 template_id 快速创建视频。系统整合了多个 AI 视频生成 API（A2E、A4、ViduQ2），提供统一的接口。

## 🚀 启动服务

### 前置要求

服务需要 Redis 来存储任务状态：

```bash
# Docker 启动 Redis
docker run -d -p 6379:6379 redis:latest

# 或使用本地 Redis
redis-server
```

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| PORT | 服务端口 | 8083 |
| REDIS_ADDR | Redis 地址 | localhost:6379 |
| REDIS_PASSWORD | Redis 密码 | (空) |
| REDIS_DB | Redis 数据库 | 0 |
| A2E_TOKEN | A2E API Token | sk_xxx |
| A4_TOKEN | A4 API Token | Bearer api-xxx |
| SHORTAPI_KEY | ShortAPI Key (ViduQ2) | sk_xxx |
| AITUBO_API_BASE | A4 API 地址 | https://api.aitubo.ai |

### 启动命令

```bash
cd plugin/sora_video
go run main.go
```

## 📡 API 接口

### 1. 创建视频

**请求**
```http
POST /generate
Content-Type: application/json

{
  "template_id": "dance",
  "args": {
    "image": "https://example.com/image.jpg",  // 可选，部分类型需要
    "input_images": "https://example.com/input.jpg"  // 可选，图生图模式
  }
}
```

**参数说明**
- `template_id` (必需): 模板 ID
- `args.image` (可选): 图片 URL，用于图生视频类型
- `args.input_images` (可选): 原图 URL，用于图生图模式

**响应**
```json
{
  "code": 200,
  "data": {
    "task_id": "image_to_video_a2e_viduq2:a2e_viduq2_1234567890",
    "template_id": "dance",
    "template_type": "image_to_video_a2e_viduq2"
  }
}
```

### 2. 查询任务

**请求**
```http
GET /query?task_id=image_to_video_a2e_viduq2:a2e_viduq2_1234567890
```

**响应**
```json
{
  "code": 200,
  "data": {
    "task_id": "image_to_video_a2e_viduq2:a2e_viduq2_1234567890",
    "status": 2,
    "error_msg": "",
    "videos": [
      {
        "url": "https://example.com/generated-video.mp4"
      }
    ]
  }
}
```

**状态码**
- `0`: 处理中（pending/generating）
- `1`: 生成中（generating）
- `2`: 完成（completed）
- `3`: 失败（failed）

## 🎯 5种模板类型

| 类型 | 说明 | 流程 | 是否需要图片 |
|------|------|------|--------------|
| `image_to_video_a4` | A4 图生视频 | 单步 | ✅ 需要 `image` 参数 |
| `image_to_video_viduq2` | ViduQ2 图生视频 | 单步 | ✅ 需要 `image` 参数 |
| `image_to_video_a2e_viduq2` | A2E 图生图 + ViduQ2 图生视频 | 两步 | ❌ 可选（文生图/图生图） |
| `image_to_video_a2e_s_e_viduq2` | A2E 图生图（尾帧）+ ViduQ2 首尾帧生视频 | 两步 | ✅ 需要 `input_images`（首帧） |
| `image_to_video_a2e_a4` | A2E 图生图 + A4 图生视频 | 两步 | ❌ 可选（文生图/图生图） |

## 📋 各类型详细说明

### 1. image_to_video_a4

**说明**: 使用 A4 API 直接将图片转换为视频

**参数**:
- `image` (必需): 图片 URL

**示例**:
```json
{
  "template_id": "undress",
  "args": {
    "image": "https://example.com/image.jpg"
  }
}
```

---

### 2. image_to_video_viduq2

**说明**: 使用 ViduQ2 API 直接将图片转换为视频

**参数**:
- `image` (必需): 图片 URL

**示例**:
```json
{
  "template_id": "ahegao",
  "args": {
    "image": "https://example.com/image.jpg"
  }
}
```

---

### 3. image_to_video_a2e_viduq2

**说明**: 两步生成 - A2E 生成图片 → ViduQ2 生成视频

**流程**:
```
步骤1: A2E 图生图（使用 imageParameters）
  ↓ 轮询等待（约30秒）
步骤2: ViduQ2 图生视频（使用生成的图片）
  ↓ 轮询等待（约2分钟）
完成: 返回视频 URL
```

**参数**:
- `input_images` (可选): 原图 URL，图生图模式
- 不提供时使用文生图模式

**示例（图生图）**:
```json
{
  "template_id": "dance",
  "args": {
    "input_images": "https://example.com/source.jpg"
  }
}
```

---

### 4. image_to_video_a2e_s_e_viduq2

**说明**: 首尾帧生成 - A2E 生成尾帧图 → ViduQ2 使用首尾帧生成视频

**流程**:
```
步骤1: A2E 图生图（原图 → 尾帧）
  ↓ 轮询等待（约30秒）
步骤2: ViduQ2 首尾帧生视频（原图 + 尾帧）
  ↓ 轮询等待（约2分钟）
完成: 返回视频 URL
```

**参数**:
- `input_images` (必需): 原图 URL（首帧）

**示例**:
```json
{
  "template_id": "posing",
  "args": {
    "input_images": "https://example.com/source.jpg"
  }
}
```

---

### 5. image_to_video_a2e_a4

**说明**: 两步生成 - A2E 生成图片 → A4 生成视频

**流程**:
```
步骤1: A2E 图生图（使用 imageParameters）
  ↓ 轮询等待（约30秒）
步骤2: A4 图生视频（使用生成的图片）
  ↓ 轮询等待（约2分钟）
完成: 返回视频 URL
```

**参数**:
- `input_images` (可选): 原图 URL，图生图模式
- 不提供时使用文生图模式

**示例（图生图）**:
```json
{
  "template_id": "bouncing_breasts",
  "args": {
    "input_images": "https://example.com/source.jpg"
  }
}
```

## 📝 模板配置

模板文件：`template_nsfw.json`

```json
[
  {
    "templateId": "dance",
    "imageParameters": "{\"name\":\"Template-Dance\",\"prompt\":\"图片生成提示词...\",\"width\":768,\"height\":512,\"model_type\":\"a2e\",\"max_images\":1}",
    "videoParameters": "{\"model\":\"vidu/vidu-q2/image-to-video\",\"args\":{\"prompt\":\"视频生成提示词...\",\"duration\":\"5\",\"mode\":\"pro\",\"resolution\":\"720p\"}}",
    "type": "image_to_video_a2e_viduq2",
    "videoUrl": "https://opengoon.com/assets/styles/video/video-naked_dance.mp4",
    "title": "Dance",
    "desc": "描述文字"
  }
]
```

**字段说明**:
- `templateId`: 模板唯一标识
- `imageParameters`: A2E 图片生成参数（JSON 字符串）
- `videoParameters`: 视频生成参数（JSON 字符串）
- `type`: 生成器类型
- `videoUrl`: 示例视频 URL
- `title`: 模板标题
- `desc`: 模板描述

## 🔧 开发调试

### 查看所有任务

```bash
redis-cli
> KEYS task:*
> GET task:a2e_viduq2_xxx
```

### 监控日志

日志会打印用户传入的参数和 API 调用详情：

```bash
# 查看实时日志
tail -f sora-video.log

# 搜索特定任务
grep "a2e_viduq2_1234567890" sora-video.log
```

### 测试工具

项目提供了多个测试脚本：

```bash
# 测试 A4 图生视频
go run test_a4.go

# 测试 ViduQ2 图生视频
go run test_viduq2.go

# 测试 A2E+ViduQ2 两步生成（文生图）
go run test_a2e_viduq2_wait.go

# 测试 A2E+ViduQ2 两步生成（图生图）
go run test_a2e_viduq2_img2img.go

# 测试 A2E+ViduQ2 首尾帧生成
go run test_a2e_se_viduq2.go

# 测试 A2E+A4 两步生成
go run test_a2e_a4.go
```

## 📊 性能优化

1. **异步处理**: 所有耗时操作在 goroutine 中执行，立即返回 taskID
2. **轮询机制**: 每 5 秒查询一次上游 API 状态
3. **Redis 缓存**: 任务状态存储 24 小时，自动过期
4. **连接池**: Redis 使用连接池，支持高并发

## 🐳 Docker 部署

```bash
# 构建镜像
docker build -t sora-video-server .

# 运行容器
docker run -d \
  -p 8083:8083 \
  -e REDIS_ADDR=redis:6379 \
  -e A2E_TOKEN=sk_xxx \
  -e A4_TOKEN="Bearer api-xxx" \
  -e SHORTAPI_KEY=sk_xxx \
  -e AITUBO_API_BASE=https://api-marmot.wenuts.top \
  sora-video-server
```

## ⚠️ 注意事项

1. **Redis 必须**: 服务依赖 Redis，启动前确保 Redis 可用
2. **网络连接**: 需要能访问 A2E、A4、ShortAPI
3. **Token 配置**: 确保所有 API Token 配置正确
4. **A4 API 开发环境**: 开发环境使用 `https://api-marmot.wenuts.top`，需配置对应的 Token
5. **轮询超时**: 任务轮询最多 5 分钟，超时需重新查询
6. **内存限制**: Redis 设置合理的 maxmemory-policy

## 🔍 故障排查

### 问题1: 任务一直处于 pending 状态

**原因**: 后台 goroutine 可能未启动或已崩溃

**解决**:
```bash
# 查看 goroutine 日志
grep "任务.*- 步骤1" sora-video.log

# 检查 Redis 任务是否存在
redis-cli GET task:xxx
```

### 问题2: A4 API 返回 403

**原因**: Cloudflare 保护或使用错误的 API 地址

**解决**:
```bash
# 使用开发环境 API
export AITUBO_API_BASE="https://api-marmot.wenuts.top"
```

### 问题3: A4 API 返回 401

**原因**: Token 不正确或过期

**解决**:
```bash
# 检查 Token 配置
redis-cli GET task:xxx | jq .video_task_id

# 确认 A4_TOKEN 环境变量格式
export A4_TOKEN="Bearer api-xxx"
```

## 📈 架构设计

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP
       ▼
┌─────────────────────────────────┐
│   SoraVideo Service (Gin)       │
│  ┌───────────────────────────┐  │
│  │   SoraTemplate Router    │  │
│  └───────┬───────────┬──────┘  │
└──────────┼───────────┼──────────┘
           │           │
      ┌────▼────┐ ┌───▼────┐
      │   A4    │ │ ViduQ2  │
      │ Generator│ │ Generator│
      └────┬────┘ └───┬─────┘
           │           │
      ┌────▼────┐ ┌───▼─────┐
      │   A2E   │ │  A2E    │
      │  API    │ │   API   │
      └─────────┘ └─────────┘
           │           │
           └─────┬─────┘
                 ▼
           ┌─────────┐
           │  Redis  │
           │ (State) │
           └─────────┘
```

## 📞 支持

如有问题请查看日志或联系开发团队。
