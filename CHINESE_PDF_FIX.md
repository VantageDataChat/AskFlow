# 中文PDF解析乱码修复 v1.1

## 问题描述

在解析中文PDF文件时，提取的文本出现乱码，显示为 `�Í�Ý�Ð�Ý�Ý�Ý` 或 `锟斤拷` 等无法识别的字符。

## 测试案例：test.pdf

**文件信息：**
- 文件大小：620,075 字节  
- 页数：19页
- 提取文本长度：74,561字符

**乱码统计：**
- 总字符数：46,368
- 乱码字符：11,005 (23.73%)
- 替换字符（�）：11,005 (23.73%)

**检测结果：**
```
[PDF] Detected garbled text on page 1-10, 17-19, may need OCR
[PDF] Garbled text detected (ratio: 22.81%, length: 76441), will attempt OCR fallback
```

✅ 系统正确检测到乱码并触发OCR回退！

## 解决方案

### 版本历史

**v1.0 (2026-03-05 14:08)** - 初始实现
- 仅检查文本长度 < 100字符
- 问题：对于长文本的乱码PDF（如test.pdf）无法触发OCR

**v1.1 (2026-03-05 18:24)** - 改进版本  
- 增加乱码比例检测（>15%）
- 解决了长文本乱码PDF的问题
- 更智能的双重触发机制

### OCR触发条件（满足任一即可）

1. **短文本检测**：提取的文本内容很少（<100字符）
2. **高乱码比例**：乱码字符超过15%

### 工作流程

1. 逐页提取PDF文本
2. 检测每页是否包含乱码（>5%阈值）
3. 计算整体乱码比例
4. 如果满足触发条件，清空乱码文本并启动OCR
5. 使用LLM视觉API对PDF页面进行OCR识别
6. 返回准确的中文文本

## 代码实现

### 改进的OCR触发逻辑

```go
// If garbled text detected, calculate ratio and decide whether to use OCR
extractedText := sb.String()
if hasGarbledText {
    // Calculate garbled character ratio
    garbledCount := 0
    totalChars := 0
    for _, r := range extractedText {
        totalChars++
        if r == '\uFFFD' {
            garbledCount++
        }
        if r >= 0x0080 && r <= 0x009F && r != '\n' && r != '\r' && r != '\t' {
            garbledCount++
        }
    }
    
    // Trigger OCR if:
    // 1. Very little text extracted (< 100 chars), OR
    // 2. High garbled ratio (> 15% of characters are garbled)
    garbledRatio := 0.0
    if totalChars > 0 {
        garbledRatio = float64(garbledCount) / float64(totalChars)
    }
    
    if len(strings.TrimSpace(extractedText)) < 100 || garbledRatio > 0.15 {
        log.Printf("[PDF] Garbled text detected (ratio: %.2f%%, length: %d), will attempt OCR fallback", 
            garbledRatio*100, len(strings.TrimSpace(extractedText)))
        extractedText = "" // Clear garbled text to trigger OCR fallback
    }
}
```

## 阈值说明

| 阈值类型 | 值 | 用途 |
|---------|-----|------|
| 乱码检测阈值 | 5% | 判断单页是否包含乱码 |
| OCR触发阈值 | 15% | 判断是否需要OCR整个文档 |
| 短文本阈值 | 100字符 | 判断是否为扫描PDF |

## 测试验证

使用 `test_pdf_parser.go` 测试工具：

```bash
go run test_pdf_parser.go
```

输出示例：
```
PDF file size: 620075 bytes
[PDF] Detected garbled text on page 1, may need OCR
...
[PDF] Garbled text detected (ratio: 22.81%, length: 76441), will attempt OCR fallback

Garbled character analysis:
Total characters: 46368
Garbled characters: 11005 (23.73%)
⚠️  WARNING: Garbled text detected (23.73% > 5% threshold)
This PDF should trigger OCR fallback in the system.
```

## 优势

1. **双重保护**：短文本和高乱码比例都能触发OCR
2. **智能判断**：根据实际乱码情况决定是否使用OCR
3. **自动检测**：无需手动判断PDF编码问题
4. **保持兼容**：不影响正常PDF的处理流程

## 相关文件

- `internal/parser/parser.go` - PDF解析和乱码检测逻辑
- `internal/document/manager.go` - OCR回退处理
- `test_pdf_parser.go` - 测试工具
- `test.pdf` - 测试用例

## 编译信息

- Go版本：1.26.0
- GoPDF2版本：v0.0.0-20260212143022-4f8ad48dca6e
- 最后更新：2026-03-05 18:24
