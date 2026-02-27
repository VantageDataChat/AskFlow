# 视频关键帧提取改进

## 改进内容

将 FFmpeg 关键帧提取方式从固定时间间隔改为基于场景切换检测（Scene Change Detection），只在画面发生实质性改变时抽取关键帧，并将阈值作为可配置参数。

## 技术实现

### 主要变更

1. **场景切换检测**
   - 使用 FFmpeg 的 `select='gt(scene,threshold)'` 过滤器
   - 场景变化阈值可在管理后台配置（范围 0-1，默认 0.3）
   - 只在画面发生实质性改变时提取帧

2. **时间戳记录**
   - 添加 `showinfo` 过滤器输出每帧的精确时间戳
   - 新增 `parseShowinfoTimestamps()` 函数解析 pts_time 信息
   - 关键帧的 Timestamp 字段现在记录实际的视频时间点

3. **回退机制**
   - 如果场景检测未提取到帧（阈值过严或视频静态），自动回退到固定间隔模式
   - 如果固定间隔模式也失败，提取视频中间帧作为最后保障

4. **可配置参数**
   - 在 `VideoConfig` 中添加 `SceneChangeThreshold` 字段
   - 管理后台多模态配置页面添加"场景切换检测阈值"输入框
   - 默认值为 0.3，可在 0-1 之间调整

## 配置说明

### 后端配置

在 `config.json` 中：

```json
{
  "video": {
    "scene_change_threshold": 0.3
  }
}
```

### 管理后台配置

1. 登录管理后台
2. 进入"多模态配置"页面
3. 找到"场景切换检测阈值"输入框
4. 输入 0-1 之间的值：
   - 0.1-0.2：敏感，提取更多帧
   - 0.3：默认值，平衡
   - 0.4-0.5：严格，只提取明显变化的帧
5. 点击保存

## 优势

1. **更智能的帧选择**：只在场景切换时提取，避免冗余的相似帧
2. **精确的时间戳**：记录每帧的实际时间点，便于视频定位和检索
3. **自适应处理**：根据视频内容动态调整提取数量
4. **向后兼容**：保留固定间隔模式作为回退方案
5. **灵活配置**：管理员可根据实际需求调整阈值

## FFmpeg 命令示例

```bash
# 场景切换检测模式（新方式，阈值可配置）
ffmpeg -i video.mp4 -vf "select='gt(scene,0.3)',showinfo" -vsync vfr -q:v 2 frame_%04d.jpg

# 固定间隔模式（回退方案）
ffmpeg -i video.mp4 -vf "fps=1/10" -q:v 2 frame_%04d.jpg
```

## 阈值调整建议

| 阈值范围 | 适用场景 | 提取帧数 |
|---------|---------|---------|
| 0.1-0.2 | 静态内容多的视频（如讲座、演示） | 较多 |
| 0.3 | 通用场景（默认） | 适中 |
| 0.4-0.5 | 动态内容多的视频（如电影、动画） | 较少 |
| 0.6+ | 极度动态的视频 | 很少 |

## 相关文件

### 后端
- `internal/config/config.go`: 配置结构和验证
  - `VideoConfig.SceneChangeThreshold`: 新增字段
  - `applyUpdate()`: 添加配置更新逻辑
  - `applyDefaults()`: 添加默认值处理
- `internal/video/parser.go`: 核心实现
  - `Parser.SceneChangeThreshold`: 新增字段
  - `NewParser()`: 初始化阈值
  - `ExtractKeyframes()`: 使用配置的阈值
  - `parseShowinfoTimestamps()`: 时间戳解析函数

### 前端
- `frontend/dist/index.html`: 管理后台UI
  - 添加"场景切换检测阈值"输入框
- `frontend/dist/app.js`: 前端逻辑
  - `loadMultimodalSettings()`: 加载配置
  - `saveMultimodalSettings()`: 保存配置

## 测试建议

1. **不同阈值测试**：使用同一视频测试不同阈值的效果
2. **视频类型测试**：测试静态和动态视频的提取效果
3. **回退机制测试**：测试场景检测失败时的回退逻辑
4. **配置持久化测试**：验证配置保存和加载是否正确
