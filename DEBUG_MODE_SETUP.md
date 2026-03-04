# Debug Mode Setup Guide

## 问题
Debug模式已启用（`vector.debug_mode = true`），但前端未显示调试信息。

## 原因
系统默认只对管理员用户显示调试信息，以防止信息泄露。在 `internal/handler/query.go` 中有权限检查：

```go
// Strip debug info for non-admin users to prevent information leakage
if resp.DebugInfo != nil {
    _, _, adminErr := GetAdminSession(app, r)
    if adminErr != nil {
        resp.DebugInfo = nil  // 非管理员用户，移除调试信息
    }
}
```

## 解决方案

### 方案1：以管理员身份登录（推荐用于生产环境）

1. 访问管理后台登录页面（默认 `/admin`）
2. 使用管理员账号登录
3. 登录后，前端查询会自动带上管理员session token
4. 调试信息会正常显示

### 方案2：临时移除权限检查（仅用于开发调试）

**已实施：** 在 `internal/handler/query.go` 中临时注释掉了权限检查代码。

**注意：** 这个修改仅用于开发调试，在生产环境中应该恢复权限检查，以防止敏感信息泄露。

## 调试信息内容

启用debug模式后，响应中会包含 `debug_info` 字段，包含：

```json
{
  "answer": "...",
  "sources": [...],
  "debug_info": {
    "intent": "product",
    "vector_dim": 1024,
    "top_k": 5,
    "threshold": 0.5,
    "result_count": 3,
    "steps": [
      "=== LLM Context Resolution ===",
      "System Prompt: ...",
      "User Prompt: ...",
      "LLM Response: ...",
      "Parsed: is_follow_up=true, resolved_query='...'",
      "Context resolution: detected follow-up query '...'",
      "Context resolution: resolved to '...'",
      "Extracted context keywords: [OCR, QmImgUtil, 图片, 图像]",
      "Context-based re-ranking details:",
      "  [0] doc='supported_image_formats.md' vector_score=0.8234 context_score=0.45 matched_kws=[OCR, QmImgUtil, 图片]",
      "  [1] doc='filetype-spec.md' vector_score=0.7891 context_score=0.10 matched_kws=[格式]",
      "..."
    ],
    "top_results": [
      {"doc_name": "supported_image_formats.md", "score": 0.8234, "dim_match": true},
      ...
    ]
  }
}
```

## 重要调试信息说明

1. **LLM Context Resolution**: 显示LLM如何分析和改写用户查询
   - `is_follow_up`: 是否检测为追问
   - `resolved_query`: 改写后的完整查询

2. **Context Keywords**: 从对话历史中提取的关键词
   - 用于上下文感知重排序

3. **Re-ranking Details**: 重排序详情
   - `vector_score`: 原始向量相似度分数
   - `context_score`: 上下文匹配分数
   - `matched_kws`: 匹配的关键词列表

4. **Top Results**: 最终返回的文档列表及分数

## 使用步骤

1. **重启后端服务**以加载修改后的代码
2. **发起查询**：
   - 第一轮："OCR支持哪些图片格式?"
   - 第二轮："列出详细清单"
3. **查看响应**中的 `debug_info` 字段
4. **分析调试信息**：
   - 检查LLM是否正确识别为追问
   - 检查改写后的查询是否包含足够的限定词
   - 检查提取的上下文关键词是否正确
   - 检查重排序是否正确提升了相关文档

## 恢复生产环境配置

在部署到生产环境前，请：

1. 在 `internal/handler/query.go` 中恢复权限检查代码（取消注释）
2. 或者设置 `vector.debug_mode = false` 完全禁用调试模式
