# 3级查询系统与多轮会话的上下文处理

## 问题
系统支持3级查询（Level 1: 文本匹配, Level 2: 向量确认, Level 3: 完整RAG），这是否会影响多轮会话时的向量检索？

## 答案：不会影响，已正确处理

3级查询系统在所有级别都正确使用了增强后的查询（`enhancedQuestion`），确保多轮会话的上下文一致性。

## 实现细节

### 上下文解析（所有级别共享）

在进入3级查询之前，系统首先进行上下文解析：

```go
// 第330-360行：上下文解析
enhancedQuestion := req.Question
isFollowUp := false

if len(history) > 0 && len(history) <= 10 {
    // 使用LLM检测是否为追问并解析上下文
    contextResolution := qe.resolveContextWithLLM(req.Question, history, ls, debugMode, dbg)
    
    if contextResolution.IsFollowUp && contextResolution.ResolvedQuery != "" {
        isFollowUp = true
        enhancedQuestion = contextResolution.ResolvedQuery
        // 例如："列出详细清单" → "列出OCR程序QmImgUtil支持的图片图像格式详细清单"
    }
}
```

### Level 1: 文本匹配（使用enhancedQuestion）

```go
// 第426-437行
searchQuery := req.Question
if enhancedQuestion != req.Question {
    searchQuery = enhancedQuestion  // ✓ 使用增强查询
    if debugMode {
        dbg.Steps = append(dbg.Steps, fmt.Sprintf("TextMatch: using enhanced query '%s'", searchQuery))
    }
}

// Level 1: Text-based search
textResults, textErr := qe.vectorStore.TextSearch(searchQuery, 3, 0.65, req.ProductID)
```

**优势：**
- 零API成本
- 直接使用增强后的查询进行文本匹配
- 如果命中且有缓存答案，立即返回

### Level 2: 向量确认（使用enhancedQuestion）

```go
// 第463行
queryVector, embErr := qe.cachedEmbed(searchQuery, es)  // ✓ searchQuery = enhancedQuestion
if embErr == nil {
    vecResults, vecErr := qe.vectorStore.Search(queryVector, cfg.Vector.TopK, cfg.Vector.Threshold, req.ProductID)
    // 如果找到缓存答案，返回
}
```

**优势：**
- 仅使用embedding API，不调用LLM
- 使用增强查询的向量进行搜索
- 如果找到缓存答案，节省LLM成本

### Level 3: 完整RAG管道（使用enhancedQuestion + 特殊优化）

```go
// 第503行：使用enhancedQuestion进行embedding
queryVector, err := qe.cachedEmbed(enhancedQuestion, es)  // ✓ 使用增强查询

// 第518-533行：追问特殊处理
if isFollowUp {
    // 提高阈值，确保只返回高度相关的结果
    threshold = threshold * 1.15
    if threshold > 0.9 {
        threshold = 0.9
    }
    // 减少返回数量
    topK = (topK + 1) / 2
    if topK < 2 {
        topK = 2
    }
}

// 第556-580行：上下文感知重排序
if isFollowUp && len(results) > 1 && len(history) > 0 {
    contextKeywords := qe.extractContextKeywords(history, debugMode, dbg)
    if len(contextKeywords) > 0 {
        results = qe.reRankByContext(results, contextKeywords, debugMode, dbg)
    }
}
```

**优势：**
- 使用增强查询进行向量搜索
- 对追问使用更严格的阈值（减少噪音）
- 使用上下文关键词重排序（提升相关文档）
- 完整的LLM生成答案

## 多轮会话处理流程

### 示例：OCR格式查询

**第一轮（独立查询）：**
```
用户: "OCR支持哪些图片格式?"
→ enhancedQuestion = "OCR支持哪些图片格式?" (无变化)
→ isFollowUp = false
→ Level 1/2/3 都使用原始查询
→ 正常返回结果
```

**第二轮（追问）：**
```
用户: "列出详细清单"
→ LLM分析对话历史
→ enhancedQuestion = "列出OCR程序QmImgUtil支持的图片图像格式详细清单"
→ isFollowUp = true

Level 1 (文本匹配):
  - 使用 enhancedQuestion 进行文本搜索
  - 搜索 "列出OCR程序QmImgUtil支持的图片图像格式详细清单"
  - 如果命中 supported_image_formats.md 且有缓存，立即返回

Level 2 (向量确认):
  - 使用 enhancedQuestion 的向量进行搜索
  - 如果找到缓存答案，返回

Level 3 (完整RAG):
  - 使用 enhancedQuestion 的向量进行搜索
  - 提高阈值到 0.5 * 1.15 = 0.575
  - 减少 topK 到 5 / 2 = 2
  - 提取上下文关键词: [OCR, QmImgUtil, 图片, 图像]
  - 重排序结果:
    * supported_image_formats.md: vector_score=0.82, context_score=0.45 (匹配4个关键词)
    * filetype-spec.md: vector_score=0.79, context_score=0.10 (匹配1个关键词)
  - supported_image_formats.md 排名第一
  - 使用正确文档生成答案
```

## 关键优势

1. **一致性**：所有3个级别都使用相同的 `enhancedQuestion`，确保上下文一致
2. **效率**：Level 1和2可以快速返回缓存答案，节省API成本
3. **准确性**：Level 3有额外的追问优化（更严格阈值 + 上下文重排序）
4. **渐进式**：从快速廉价的Level 1到完整昂贵的Level 3，逐步提升

## 调试验证

启用debug模式后，可以看到每个级别使用的查询：

```json
{
  "debug_info": {
    "steps": [
      "Context resolution: detected follow-up query '列出详细清单'",
      "Context resolution: resolved to '列出OCR程序QmImgUtil支持的图片图像格式详细清单'",
      "TextMatch: using enhanced query '列出OCR程序QmImgUtil支持的图片图像格式详细清单'",
      "TextMatch: Level 1 — text-based matching (no API cost)",
      "TextMatch: Level 1 miss (best_score=0.6234), proceeding to Level 3",
      "Step 1: embedded question, vector_dim=1024",
      "Follow-up question: increased threshold to 0.58, reduced topK to 2",
      "Step 2: search topK=2 threshold=0.58 results=2",
      "Extracted context keywords: [OCR, QmImgUtil, 图片, 图像]",
      "Context-based re-ranking details:",
      "  [0] doc='supported_image_formats.md' vector_score=0.8234 context_score=0.45 matched_kws=[OCR, QmImgUtil, 图片, 图像]",
      "  [1] doc='filetype-spec.md' vector_score=0.7891 context_score=0.10 matched_kws=[格式]"
    ]
  }
}
```

## 结论

3级查询系统不会影响多轮会话的向量检索，反而增强了它：
- ✓ 所有级别都使用 `enhancedQuestion`
- ✓ Level 3 对追问有额外优化
- ✓ 上下文感知重排序进一步提升准确性
- ✓ 保持了性能优势（Level 1/2快速返回）
