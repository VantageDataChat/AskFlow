# 视频关键帧提取改进

## 改进内容

将 FFmpeg 关键帧提取方式从固定时间间隔改为基于场景切换检测（Scene Change Detection），只在画面发生实质性改变时抽取关键帧。

## 技术实现

### 主要变更

1. **场景切换检测**
   - 使用 FFmpeg 的 `select='gt(scene,0.3)'` 过滤器
   - 场景变化阈值设置为 0.3（范围 0-1，值越大越严格）
   - 只在画面发生实质性改变时提取帧

2. **时间戳记录**
   - 添加 `showinfo` 过滤器输出每帧的精确时间戳
   - 新增 `parseShowinfoTimestamps()` 函数解析 pts_time 信息
   - 关键帧的 Timestamp 字段现在记录实际的视频时间点

3. **回退机制**
   - 如果场景检测未提取到帧（阈值过严或视频静态），自动回退到固定间隔模式
   - 如果固定间隔模式也失败，提取视频中间帧作为最后保障

## 优势

1. **更智能的帧选择**：只在场景切换时提取，避免冗余的相似帧
2. **精确的时间戳**：记录每帧的实际时间点，便于视频定位和检索
3. **自适应处理**：根据视频内容动态调整提取数量
4. **向后兼容**：保留固定间隔模式作为回退方案

## FFmpeg 命令示例

```bash
# 场景切换检测模式（新方式）
ffmpeg -i video.mp4 -vf "select='gt(scene,0.3)',showinfo" -vsync vfr -q:v 2 frame_%04d.jpg

# 固定间隔模式（回退方案）
ffmpeg -i video.mp4 -vf "fps=1/10" -q:v 2 frame_%04d.jpg
```

## 相关文件

- `internal/video/parser.go`: 核心实现
  - `ExtractKeyframes()`: 关键帧提取主函数
  - `parseShowinfoTimestamps()`: 时间戳解析函数
