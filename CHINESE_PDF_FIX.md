# 中文PDF解析乱码修复

## 问题描述

在解析中文PDF文件时，提取的文本出现乱码，显示为 `�Í�Ý�Ð�Ý�Ý�Ý` 等无法识别的字符。这是由于GoPDF2库在处理某些中文PDF时存在字符编码问题。

## 根本原因

GoPDF2库的 `ExtractPageText()` 函数在处理某些中文PDF时无法正确解码字符编码，导致返回乱码文本。这是PDF解析库的常见问题，特别是对于CJK（中文、日文、韩文）字符。

## 解决方案

实现了智能乱码检测和OCR回退机制：

### 1. 乱码检测

添加了 `containsGarbledText()` 函数来检测文本是否包含乱码字符：

- 检测Unicode替换字符 (�, U+FFFD)，这是编码失败的标准指示符
- 检测异常控制字符 (0x0080-0x009F)
- 如果超过5%的字符是乱码，则判定为乱码文本
- 特别检测 "�" 字符的高频出现

### 2. OCR回退机制

当检测到乱码文本且提取的文本内容很少（<100字符）时：

1. 清空乱码文本
2. 触发现有的OCR回退逻辑
3. 使用LLM视觉API对PDF页面图片进行OCR识别
4. 提取准确的中文文本

## 代码修改

### 文件：`internal/parser/parser.go`

#### 修改1：在PDF文本提取中添加乱码检测

```go
// Extract text page by page
var sb strings.Builder
hasGarbledText := false
for i := 0; i < pageCount; i++ {
    text, err := gopdf.ExtractPageText(data, i)
    if err != nil {
        continue
    }
    if text != "" {
        // Detect garbled Chinese text (common encoding issue indicators)
        if containsGarbledText(text) {
            hasGarbledText = true
            log.Printf("[PDF] Detected garbled text on page %d, may need OCR", i+1)
        }
        if sb.Len() > 0 {
            sb.WriteString("\n\n")
        }
        sb.WriteString(text)
    }
}

// If garbled text detected and we have very little valid text, treat as scanned PDF
extractedText := sb.String()
if hasGarbledText && len(strings.TrimSpace(extractedText)) < 100 {
    log.Printf("[PDF] Garbled text detected with minimal content, will attempt OCR fallback")
    extractedText = "" // Clear garbled text to trigger OCR fallback
}
```

#### 修改2：添加乱码检测函数

```go
// containsGarbledText detects if text contains garbled characters that indicate
// encoding issues, particularly common with Chinese PDFs.
// Returns true if the text appears to be garbled and may need OCR.
func containsGarbledText(text string) bool {
    if len(text) == 0 {
        return false
    }
    
    garbledCount := 0
    totalChars := 0
    
    for _, r := range text {
        totalChars++
        // Check for replacement character (�) which indicates encoding failure
        if r == '\uFFFD' {
            garbledCount++
        }
        // Check for unusual control characters that shouldn't appear in normal text
        if r >= 0x0080 && r <= 0x009F && r != '\n' && r != '\r' && r != '\t' {
            garbledCount++
        }
    }
    
    // If more than 5% of characters are garbled, consider it garbled text
    if totalChars > 0 && float64(garbledCount)/float64(totalChars) > 0.05 {
        return true
    }
    
    // Also check for patterns like "�Í�Ý" which are common in garbled Chinese text
    if strings.Contains(text, "�") && strings.Count(text, "�") > len(text)/20 {
        return true
    }
    
    return false
}
```

#### 修改3：更新返回语句使用提取的文本变量

```go
return &ParseResult{
    Text: CleanText(extractedText),  // 使用 extractedText 而不是 sb.String()
    Metadata: map[string]string{
        "type":        "pdf",
        "page_count":  fmt.Sprintf("%d", pageCount),
        "image_count": fmt.Sprintf("%d", len(images)),
    },
    Images: images,
}, nil
```

## 工作原理

1. **正常PDF**：如果PDF文本提取成功且没有乱码，直接使用提取的文本
2. **乱码PDF**：
   - 检测到乱码字符
   - 如果提取的文本很少（<100字符），清空文本
   - 触发现有的OCR逻辑（在 `internal/document/manager.go` 中）
   - 使用LLM视觉API对每页进行OCR识别
   - 返回准确的中文文本

## 优势

1. **自动检测**：无需手动判断PDF是否有编码问题
2. **智能回退**：只在必要时使用OCR，节省API调用
3. **保持兼容**：不影响正常PDF的处理流程
4. **利用现有功能**：复用已有的OCR基础设施

## 测试

编译后的可执行文件：`askflow_chinese_fix.exe`

测试步骤：
1. 上传有乱码问题的中文PDF
2. 系统自动检测乱码
3. 触发OCR处理
4. 返回正确的中文文本

## 注意事项

1. OCR处理需要配置LLM服务（用于视觉API）
2. OCR处理比直接文本提取慢，但能保证准确性
3. 对于大型PDF文件，OCR处理可能需要较长时间
4. 系统会自动并发处理多页（最多3个并发LLM调用）

## 未来改进建议

1. **替换PDF库**：考虑使用对中文支持更好的PDF解析库
   - `unidoc/unipdf`（商业库，Unicode支持好）
   - `pdfcpu`（纯Go实现）
   - 调用外部工具如 `pdftotext` 并指定编码

2. **编码转换**：尝试检测和转换不同的中文编码（GB2312, GBK, UTF-8等）

3. **混合策略**：对于部分页面乱码的PDF，只对乱码页面使用OCR

## 相关文件

- `internal/parser/parser.go` - PDF解析逻辑
- `internal/document/manager.go` - OCR回退逻辑
- `go.mod` - 依赖包版本

## 编译信息

- 修复时间：2026-03-05
- 编译输出：`askflow_chinese_fix.exe`
- Go版本：1.26.0
- GoPDF2版本：v0.0.0-20260212143022-4f8ad48dca6e
