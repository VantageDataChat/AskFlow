# 上下文解析未执行问题诊断

## 问题现象

从调试信息可以看到，第二轮查询"列出详细清单"时：
- ❌ 没有显示 "=== LLM Context Resolution ===" 步骤
- ❌ 没有显示 "Context resolution: detected follow-up query"
- ❌ 没有显示 "Extracted context keywords"
- ❌ 没有显示 "Context-based re-ranking details"
- ❌ 返回了错误的文档 `filetype-spec.md`（分数0.4485，使用relaxed search）

## 根本原因分析

### 可能原因1：对话历史为空

**症状：** 第二轮查询时，`history` 数组为空，导致跳过上下文解析

**原因：**
1. 前端没有传递 `conversation_id`
2. 每次查询都创建新的conversation，导致无法关联历史
3. 用户身份识别问题（session ID或anonymous ID不一致）

**验证方法：**
查看后端日志，应该看到：
```
[Query] loaded 0 history messages for conversation xxx
[Query] no conversationID, history will be empty
```

### 可能原因2：对话功能未启用

**症状：** `conversation.enabled = false` 或配置未正确加载

**验证方法：**
查看后端日志，应该看到：
```
[Query] conversation disabled, history will be empty
```

### 可能原因3：消息保存失败

**症状：** 第一轮查询的消息没有成功保存到数据库

**原因：**
- 数据库写入错误
- conversation_id 无效
- AddMessage 调用失败但被忽略（使用了 `_ = convMgr.AddMessage(...)`）

## 诊断步骤

### 步骤1：检查配置

确认 `data/config.json` 中：
```json
{
  "conversation": {
    "enabled": true,
    "max_history_messages": 10,
    "max_history_age": 30,
    "compression_enabled": false
  }
}
```

### 步骤2：检查前端请求

第二轮查询时，前端应该传递第一轮返回的 `conversation_id`：

**第一轮请求：**
```json
{
  "question": "ocr功能支持哪些格式？",
  "product_id": "xxx"
  // 没有 conversation_id
}
```

**第一轮响应：**
```json
{
  "answer": "...",
  "conversation_id": "conv-abc123"  // ← 前端需要保存这个
}
```

**第二轮请求：**
```json
{
  "question": "列出详细清单",
  "product_id": "xxx",
  "conversation_id": "conv-abc123"  // ← 必须传递这个
}
```

### 步骤3：查看后端日志

重启后端，执行两轮查询，查看日志输出：

**期望看到（第一轮）：**
```
[Query] no conversationID, history will be empty
[Query] loaded 0 history messages for conversation conv-xxx
```

**期望看到（第二轮）：**
```
[Query] loaded 2 history messages for conversation conv-xxx
[Query] history preview: first message role=user, content=ocr功能支持哪些格式？
```

**如果看到：**
```
[Query] conversation disabled, history will be empty
```
→ 说明对话功能未启用

**如果看到：**
```
[Query] no conversationID, history will be empty
```
→ 说明前端没有传递 conversation_id

### 步骤4：查看调试信息

在第二轮查询的 debug_info 中，应该看到：

**如果 history 为空：**
```
Conversation history length: 0
Skipping context resolution: no conversation history
```

**如果 history 正常：**
```
Conversation history length: 2
Starting LLM context resolution...
=== LLM Context Resolution ===
System Prompt: ...
User Prompt: ...
LLM Response: ...
Context resolution: detected follow-up query '列出详细清单'
Context resolution: resolved to '列出OCR程序QmImgUtil支持的图片图像格式详细清单'
```

## 解决方案

### 方案1：前端修复（最可能）

确保前端正确处理 conversation_id：

```javascript
// 保存 conversation_id
let conversationId = null;

async function sendQuery(question) {
  const payload = {
    question: question,
    product_id: currentProductId
  };
  
  // 如果有 conversation_id，传递它
  if (conversationId) {
    payload.conversation_id = conversationId;
  }
  
  const response = await fetch('/api/query', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  });
  
  const data = await response.json();
  
  // 保存返回的 conversation_id
  if (data.conversation_id) {
    conversationId = data.conversation_id;
  }
  
  return data;
}
```

### 方案2：后端增强错误处理

修改 `internal/handler/query.go`，记录消息保存失败：

```go
// Save user question
if err := convMgr.AddMessage(conversationID, "user", req.Question, req.ImageData, nil); err != nil {
    log.Printf("[Query] WARNING: failed to save user message: %v", err)
}

// Save assistant answer
if err := convMgr.AddMessage(conversationID, "assistant", resp.Answer, "", sources); err != nil {
    log.Printf("[Query] WARNING: failed to save assistant message: %v", err)
}
```

### 方案3：数据库检查

检查 `conversations` 和 `conversation_messages` 表：

```sql
-- 查看最近的对话
SELECT * FROM conversations ORDER BY created_at DESC LIMIT 10;

-- 查看最近的消息
SELECT * FROM conversation_messages ORDER BY created_at DESC LIMIT 20;
```

## 临时解决方案

如果前端无法立即修复，可以在后端实现基于session的自动关联：

```go
// 在 handler 中，如果没有 conversation_id，尝试查找该用户最近的对话
if req.ConversationID == "" && userID != "" {
    recentConv, _ := convMgr.GetMostRecentConversation(userID, req.ProductID)
    if recentConv != nil && time.Since(recentConv.UpdatedAt) < 5*time.Minute {
        conversationID = recentConv.ID
        log.Printf("[Query] auto-linked to recent conversation %s", conversationID)
    }
}
```

## 验证修复

修复后，第二轮查询的 debug_info 应该显示：

```
Conversation history length: 2
Starting LLM context resolution...
=== LLM Context Resolution ===
Parsed: is_follow_up=true, resolved_query='列出OCR程序QmImgUtil支持的图片图像格式详细清单'
Context resolution: detected follow-up query '列出详细清单'
Context resolution: resolved to '列出OCR程序QmImgUtil支持的图片图像格式详细清单'
TextMatch: using enhanced query '列出OCR程序QmImgUtil支持的图片图像格式详细清单'
Extracted context keywords: [OCR, QmImgUtil, 图片, 图像]
Context-based re-ranking details:
  [0] doc='supported_image_formats.md' vector_score=0.82 context_score=0.45 matched_kws=[OCR, QmImgUtil, 图片, 图像]
```

并且应该返回正确的文档 `supported_image_formats.md`。
