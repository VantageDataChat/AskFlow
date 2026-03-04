# 多轮对话上下文丢失问题 - 完整修复总结

## 问题回顾

用户报告：在多轮对话中，当用户询问"OCR支持哪些图片格式？"后追问"列出详细清单"时，系统返回了错误的文档（编程语言格式列表），而不是OCR图像格式的详细信息。

## 根本原因

通过调试信息确认，问题的根本原因是：

```
Conversation history length: 0
Skipping context resolution: no conversation history
```

**前端没有传递 `conversation_id`**，导致：
1. 每次查询都被当作新对话
2. 无法加载历史消息
3. 上下文解析代码被跳过
4. LLM无法识别追问并改写查询
5. 返回错误的文档

## 完整解决方案

### 1. 前端修复（主要修复）

**文件：** `frontend/dist/app.js`

#### 修改1：发送请求时包含 conversation_id
```javascript
// Build request body
var reqBody = {
    question: question,
    user_id: getChatUserID(),
    product_id: localStorage.getItem('askflow_product_id') || ''
};
if (imageData) {
    reqBody.image_data = imageData;
}
// Include conversation_id for multi-turn context
var conversationId = localStorage.getItem('askflow_conversation_id');
if (conversationId) {
    reqBody.conversation_id = conversationId;
}
```

#### 修改2：保存后端返回的 conversation_id
```javascript
.then(function (data) {
    // Save conversation_id for multi-turn context
    if (data.conversation_id) {
        localStorage.setItem('askflow_conversation_id', data.conversation_id);
    }
    // ... rest of the code
})
```

#### 修改3：切换产品时清除 conversation_id
```javascript
// Clear chat and load new welcome message
chatMessages = [];
// Clear conversation_id when switching products
localStorage.removeItem('askflow_conversation_id');
loadWelcomeMessage();
```

#### 修改4：登出时清除 conversation_id
```javascript
window.logout = function () {
    chatMessages = [];
    chatLoading = false;
    // ... other cleanup
    // Clear conversation_id on logout
    localStorage.removeItem('askflow_conversation_id');
    clearSession();
    clearAdminSession();
    navigate('/login');
};
```

#### 修改5：清除会话时清除 conversation_id
```javascript
function clearSession() {
    localStorage.removeItem(SESSION_KEY);
    localStorage.removeItem(USER_KEY);
    // Clear conversation_id when session is cleared
    localStorage.removeItem('askflow_conversation_id');
}
```

### 2. 后端增强（临时兜底方案）

**文件：** `internal/conversation/manager.go`

#### 新增方法：GetMostRecentConversation
```go
// GetMostRecentConversation retrieves the most recent conversation for a user and product.
// This is used for auto-linking when the frontend doesn't provide a conversation_id.
func (m *Manager) GetMostRecentConversation(userID, productID string, maxAge time.Duration) (string, error) {
    if userID == "" {
        return "", nil
    }

    cutoff := time.Now().Add(-maxAge)
    var conversationID string
    err := m.readDB.QueryRow(`
        SELECT id FROM conversations
        WHERE user_id = ? AND product_id = ? AND updated_at > ?
        ORDER BY updated_at DESC
        LIMIT 1
    `, userID, productID, cutoff).Scan(&conversationID)
    
    if err == sql.ErrNoRows {
        return "", nil
    }
    if err != nil {
        return "", fmt.Errorf("query recent conversation: %w", err)
    }

    return conversationID, nil
}
```

**文件：** `internal/handler/query.go`

#### 修改：自动关联最近对话
```go
} else if userID != "" {
    // No conversation_id provided, try to auto-link to recent conversation
    // This is a temporary workaround for frontend not passing conversation_id
    recentConvID, _ := convMgr.GetMostRecentConversation(userID, req.ProductID, 5*time.Minute)
    if recentConvID != "" {
        conversationID = recentConvID
        log.Printf("[Query] auto-linked to recent conversation %s (age < 5min)", conversationID)
    } else {
        // No recent conversation, create new one
        conversationID, _ = convMgr.GetOrCreateConversation(userID, sessionID, req.ProductID)
        log.Printf("[Query] created new conversation %s", conversationID)
    }
}
```

### 3. 诊断日志增强

**文件：** `internal/query/engine.go`

```go
if debugMode && dbg != nil {
    dbg.Steps = append(dbg.Steps, fmt.Sprintf("Conversation history length: %d", len(history)))
}

if len(history) > 0 && len(history) <= 10 {
    if debugMode && dbg != nil {
        dbg.Steps = append(dbg.Steps, "Starting LLM context resolution...")
    }
    // ... context resolution code
} else if debugMode && dbg != nil {
    if len(history) == 0 {
        dbg.Steps = append(dbg.Steps, "Skipping context resolution: no conversation history")
    } else {
        dbg.Steps = append(dbg.Steps, fmt.Sprintf("Skipping context resolution: history too long (%d messages)", len(history)))
    }
}
```

## 预期效果

修复后，多轮对话流程：

### 第一轮查询
```
用户: "OCR支持哪些图片格式?"
→ 前端：conversation_id = null，不传递
→ 后端：创建新对话 conv-abc123
→ 后端：history为空，跳过上下文解析（正常）
→ 后端：返回 conversation_id: "conv-abc123"
→ 前端：保存到 localStorage
→ 返回正确答案
```

### 第二轮查询
```
用户: "列出详细清单"
→ 前端：从localStorage读取 conversation_id = "conv-abc123"
→ 前端：在请求中包含 conversation_id
→ 后端：加载对话历史（2条消息）
→ 后端：执行上下文解析
→ LLM：识别为追问，改写为"列出OCR程序QmImgUtil支持的图片图像格式详细清单"
→ 向量搜索：使用改写后的查询
→ 上下文重排序：提升 supported_image_formats.md
→ 返回正确答案
```

### 调试信息（修复后）
```json
{
  "debug_info": {
    "steps": [
      "Conversation history length: 2",
      "Starting LLM context resolution...",
      "=== LLM Context Resolution ===",
      "Parsed: is_follow_up=true, resolved_query='列出OCR程序QmImgUtil支持的图片图像格式详细清单'",
      "Context resolution: detected follow-up query '列出详细清单'",
      "Context resolution: resolved to '列出OCR程序QmImgUtil支持的图片图像格式详细清单'",
      "TextMatch: using enhanced query '列出OCR程序QmImgUtil支持的图片图像格式详细清单'",
      "Extracted context keywords: [OCR, QmImgUtil, 图片, 图像]",
      "Context-based re-ranking details:",
      "  [0] doc='supported_image_formats.md' vector_score=0.82 context_score=0.45 matched_kws=[OCR, QmImgUtil, 图片, 图像]",
      "  [1] doc='filetype-spec.md' vector_score=0.79 context_score=0.10 matched_kws=[格式]"
    ]
  }
}
```

## 测试步骤

1. **清除浏览器缓存**（清除旧的localStorage数据）
2. **刷新页面**
3. **第一轮查询**："OCR支持哪些图片格式?"
4. **第二轮查询**："列出详细清单"
5. **查看调试信息**，确认：
   - `Conversation history length: 2`
   - 显示LLM上下文解析步骤
   - 显示上下文关键词提取
   - 显示重排序详情
   - 返回 `supported_image_formats.md`

## 相关文档

- `CONTEXT_RESOLUTION_IMPROVEMENTS.md` - LLM提示词和重排序改进
- `THREE_LEVEL_QUERY_CONTEXT_HANDLING.md` - 3级查询系统说明
- `CONTEXT_RESOLUTION_DIAGNOSIS.md` - 问题诊断指南
- `DEBUG_MODE_SETUP.md` - 调试模式设置指南

## 技术栈

- **前端**：Vanilla JavaScript，localStorage
- **后端**：Go，SQLite
- **对话管理**：conversation_messages表存储历史
- **上下文解析**：LLM智能分析 + 关键词重排序

## 注意事项

1. **后端兜底方案**：即使前端未传递conversation_id，后端也会尝试自动关联最近5分钟内的对话
2. **会话隔离**：不同产品的对话是隔离的
3. **清理机制**：旧对话会被定期清理（根据max_history_age配置）
4. **调试模式**：生产环境应该恢复管理员权限检查
