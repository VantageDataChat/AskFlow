# 场景切换检测阈值配置功能

## 功能概述

将视频关键帧提取的场景切换检测阈值从硬编码改为可配置参数，管理员可以在后台根据实际需求调整阈值。

---

## 实现细节

### 1. 后端配置 (Go)

#### 配置结构 (`internal/config/config.go`)

```go
type VideoConfig struct {
    // ... 其他字段
    SceneChangeThreshold  float64 `json:"scene_change_threshold"`   // 场景切换检测阈值 (0-1), 默认 0.3
}
```

#### 默认值设置

```go
func DefaultConfig() *Config {
    return &Config{
        Video: VideoConfig{
            // ... 其他字段
            SceneChangeThreshold:  0.3,
        },
    }
}
```

#### 配置更新验证

```go
case "video.scene_change_threshold":
    f, err := toFloat64(val)
    if err != nil {
        return err
    }
    if f < 0 || f > 1.0 {
        return errors.New("scene_change_threshold must be between 0 and 1.0")
    }
    cm.config.Video.SceneChangeThreshold = f
```

### 2. 视频处理 (`internal/video/parser.go`)

#### Parser结构

```go
type Parser struct {
    FFmpegPath           string
    RapidSpeechPath      string
    KeyframeInterval     int
    RapidSpeechModel     string
    SceneChangeThreshold float64  // 新增字段
}
```

#### 使用配置的阈值

```go
func (p *Parser) ExtractKeyframes(videoPath, outputDir string) ([]Keyframe, error) {
    // ...
    cmd := exec.Command(p.FFmpegPath,
        "-i", videoPath,
        "-vf", fmt.Sprintf("select='gt(scene,%.2f)',showinfo", p.SceneChangeThreshold),
        "-vsync", "vfr",
        "-q:v", "2",
        outputPattern,
    )
    // ...
}
```

### 3. 前端UI (`frontend/dist/index.html`)

#### HTML输入框

```html
<div class="admin-form-row">
    <label data-i18n="admin_multimodal_scene_threshold">场景切换检测阈值</label>
    <input type="number" id="cfg-video-scene-threshold" 
           min="0" max="1" step="0.01" placeholder="0.3">
    <span class="admin-form-hint" data-i18n="admin_multimodal_scene_threshold_hint">
        场景变化检测的敏感度（0-1），值越大越严格，只在画面发生实质性改变时提取关键帧，默认 0.3
    </span>
</div>
```

### 4. 前端逻辑 (`frontend/dist/app.js`)

#### 加载配置

```javascript
function loadMultimodalSettings() {
    // ...
    .then(function (cfg) {
        var video = cfg.video || {};
        setVal('cfg-video-scene-threshold', video.scene_change_threshold || 0.3);
        // ...
    })
}
```

#### 保存配置

```javascript
function saveMultimodalSettings() {
    var sceneThreshold = getVal('cfg-video-scene-threshold');
    if (sceneThreshold !== '') {
        updates['video.scene_change_threshold'] = parseFloat(sceneThreshold);
    }
    // ...
}
```

### 5. 多语言支持 (`frontend/dist/i18n.js`)

#### 中文翻译

```javascript
'admin_multimodal_scene_threshold': '场景切换检测阈值',
'admin_multimodal_scene_threshold_hint': '场景变化检测的敏感度（0-1），值越大越严格，只在画面发生实质性改变时提取关键帧，默认 0.3',
'admin_multimodal_keyframe_hint': '每隔多少秒从视频中提取一帧图像用于图像检索，默认 10 秒（仅在场景检测失败时使用）',
```

#### 英文翻译

```javascript
'admin_multimodal_scene_threshold': 'Scene Change Detection Threshold',
'admin_multimodal_scene_threshold_hint': 'Scene change detection sensitivity (0-1). Higher values are stricter, extracting keyframes only when significant changes occur. Default 0.3',
'admin_multimodal_keyframe_hint': 'Extract one frame every N seconds for image search, default 10 (used only when scene detection fails)',
```

---

## 使用指南

### 管理员配置步骤

1. 登录管理后台
2. 进入"多模态配置"页面
3. 找到"场景切换检测阈值"输入框
4. 输入 0-1 之间的值
5. 点击"保存多模态设置"

### 阈值选择建议

| 阈值范围 | 适用场景 | 提取效果 | 示例 |
|---------|---------|---------|------|
| 0.1-0.2 | 静态内容为主 | 提取更多帧 | 讲座、演示、教学视频 |
| 0.3 | 通用场景（默认） | 平衡 | 大多数视频 |
| 0.4-0.5 | 动态内容为主 | 提取较少帧 | 电影、动画、游戏录像 |
| 0.6+ | 极度动态 | 提取很少帧 | 快速剪辑、动作片段 |

### 配置文件示例

```json
{
  "video": {
    "ffmpeg_path": "/usr/bin/ffmpeg",
    "rapidspeech_path": "/usr/local/bin/rs-asr-offline",
    "keyframe_interval": 10,
    "scene_change_threshold": 0.3,
    "rapidspeech_model": "/path/to/model.gguf",
    "max_upload_size_mb": 500,
    "keyframe_ocr_enabled": true,
    "keyframe_ocr_max_frames": 20,
    "processing_timeout_min": 120
  }
}
```

---

## 技术优势

### 1. 灵活性
- 管理员可根据视频类型动态调整
- 无需修改代码或重启服务
- 支持实时配置更新

### 2. 智能化
- 场景检测优先，固定间隔回退
- 自适应不同类型的视频内容
- 减少冗余帧，提高检索效率

### 3. 用户友好
- 直观的UI界面
- 详细的提示说明
- 多语言支持（中文/英文）

### 4. 向后兼容
- 默认值保持0.3，与之前行为一致
- 旧配置文件自动应用默认值
- 不影响现有视频处理流程

---

## 测试建议

### 功能测试

1. **配置保存测试**
   - 输入不同阈值（0.1, 0.3, 0.5, 0.8）
   - 验证保存后配置文件正确更新
   - 刷新页面验证配置正确加载

2. **边界值测试**
   - 输入0：验证接受
   - 输入1：验证接受
   - 输入-0.1：验证拒绝
   - 输入1.1：验证拒绝

3. **视频处理测试**
   - 使用不同阈值处理同一视频
   - 验证提取的关键帧数量符合预期
   - 检查时间戳记录是否正确

### 多语言测试

1. 切换到中文界面，验证标签和提示正确显示
2. 切换到英文界面，验证翻译准确
3. 验证输入框placeholder显示正确

### 回退机制测试

1. 设置极高阈值（0.9），处理静态视频
2. 验证自动回退到固定间隔模式
3. 检查日志确认回退逻辑执行

---

## Git提交记录

### Commit 1: 添加场景切换检测阈值可配置参数
```
d5574f0 - 添加场景切换检测阈值可配置参数

功能改进：
- 在VideoConfig中添加SceneChangeThreshold字段（默认0.3）
- 管理后台多模态配置页面添加场景切换检测阈值输入框
- 支持0-1范围的阈值配置，值越大越严格
- 更新video/parser.go使用配置的阈值进行场景检测
- 更新文档说明配置方法和阈值调整建议

修改文件：
- internal/config/config.go
- internal/video/parser.go
- frontend/dist/index.html
- frontend/dist/app.js
- KEYFRAME_EXTRACTION_IMPROVEMENT.md
```

### Commit 2: 添加场景切换检测阈值的多语言本地化支持
```
8745392 - 添加场景切换检测阈值的多语言本地化支持

多语言更新：
- 中文：添加'场景切换检测阈值'和提示文本
- 英文：添加'Scene Change Detection Threshold'和提示文本
- 更新关键帧间隔提示，说明仅在场景检测失败时使用

修改文件：
- frontend/dist/i18n.js
```

---

## 相关文档

- [KEYFRAME_EXTRACTION_IMPROVEMENT.md](./KEYFRAME_EXTRACTION_IMPROVEMENT.md) - 关键帧提取改进详细说明
- [CODE_REVIEW_FIXES.md](./CODE_REVIEW_FIXES.md) - 代码审查和修复报告
- [OPTIMIZATION_SUMMARY.md](./OPTIMIZATION_SUMMARY.md) - 优化工作总结

---

## 后续优化建议

1. **预设模板**
   - 添加"讲座模式"、"电影模式"等预设
   - 一键应用推荐配置

2. **实时预览**
   - 上传测试视频，实时预览不同阈值的效果
   - 显示预计提取的关键帧数量

3. **统计分析**
   - 记录不同阈值下的提取效果
   - 提供优化建议

4. **批量调整**
   - 支持对已处理视频重新提取关键帧
   - 使用新阈值批量重新处理

---

**创建日期**: 2026-02-28  
**版本**: 1.0  
**状态**: ✅ 已完成并推送到远程仓库
