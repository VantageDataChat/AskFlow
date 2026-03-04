# Multi-Turn Context Resolution Improvements

## Date: 2026-03-04

## Problem
After implementing LLM-based context resolution, the system still returns wrong documents when users ask follow-up questions with ambiguous references. For example:
- User asks: "OCR支持哪些图片格式?" (What image formats does OCR support?)
- User follows up: "列出详细清单" (List detailed list)
- System incorrectly returns: Programming language formats from `filetype-spec.md` (VERILOG, VHDL, ASM)
- Expected: OCR image format details from `supported_image_formats.md`

## Root Cause Analysis
1. **LLM query reformulation not specific enough**: The LLM might generate queries like "列出格式详细清单" which is still ambiguous
2. **Vector embeddings not discriminating enough**: Embeddings for "OCR图片格式" and "文件格式" might be too similar
3. **No context-aware re-ranking**: Even if the right document is retrieved, it might not be ranked first

## Improvements Implemented

### 1. Enhanced LLM Prompt (engine.go:826-890)
**Changes:**
- Added explicit requirement to include ALL discriminating keywords from context
- Added emphasis on extracting keywords from assistant responses (e.g., "QmImgUtil", "图片读取库")
- Added negative example showing what NOT to do (avoid ambiguous reformulations)
- Added example showing correct reformulation: "详细看看OCR程序QmImgUtil图片读取库支持的图片图像格式"

**Key additions to prompt:**
```
**关键要求**：补充问题时必须：
- 明确指出具体的主题，包含**所有**关键限定词和上下文词汇
- 特别注意：如果前文提到"OCR图片格式"，补充时必须包含"OCR"+"图片"/"图像"等多个限定词
- 从助手的回答中提取核心主题词（如"QmImgUtil"、"图片读取库"等），加入补充问题中增强特异性
```

### 2. Context-Aware Re-Ranking (engine.go:560-580)
**New feature:** After vector search, re-rank results based on context keywords from conversation history

**How it works:**
1. Extract keywords from last user question and assistant response
2. Score each search result based on keyword matches in document name and chunk text
3. Re-rank results: context score (descending) → vector score (descending)
4. Boost documents that match specific technical terms (OCR, QmImgUtil, etc.)

**Implementation:**
- `extractContextKeywords()`: Extracts technical terms from conversation history
- `reRankByContext()`: Re-ranks search results based on context keyword matches
- Logs detailed re-ranking information in debug mode

### 3. Debug Mode Enabled (data/config.json)
**Change:** Set `vector.debug_mode = true`

**Benefits:**
- See exact LLM prompts and responses for context resolution
- See which documents are matched and their scores
- See context keywords extracted and re-ranking details
- Diagnose why wrong documents are being returned

## Expected Impact

### Before:
1. LLM reformulates "列出详细清单" → "列出格式详细清单" (still ambiguous)
2. Vector search returns both `filetype-spec.md` and `supported_image_formats.md`
3. `filetype-spec.md` might rank higher due to embedding similarity
4. Wrong document is used for answer generation

### After:
1. LLM reformulates "列出详细清单" → "列出OCR程序QmImgUtil支持的图片图像格式详细清单" (specific)
2. Vector search returns both documents
3. Context-aware re-ranking boosts `supported_image_formats.md` because it matches keywords: "OCR", "QmImgUtil", "图片", "图像"
4. Correct document is ranked first and used for answer generation

## Testing Instructions

1. **Enable debug mode** (already done): `vector.debug_mode = true` in `data/config.json`
2. **Restart the backend** to load new code and config
3. **Test the scenario:**
   - Ask: "OCR支持哪些图片格式?"
   - Follow up: "列出详细清单"
4. **Check debug output** in the response to see:
   - LLM-resolved query (should include "OCR", "图片", "QmImgUtil")
   - Context keywords extracted
   - Re-ranking details showing which documents matched which keywords
   - Final document ranking

## Files Modified

1. `internal/query/engine.go`:
   - Enhanced LLM prompt in `resolveContextWithLLM()` (lines 826-890)
   - Added context-aware re-ranking after vector search (lines 560-580)
   - Added `extractContextKeywords()` helper function (lines 1500-1560)
   - Added `reRankByContext()` helper function (lines 1562-1630)

2. `data/config.json`:
   - Set `vector.debug_mode = true`

## Next Steps

1. Test with the problematic scenario and review debug output
2. If still returning wrong documents, analyze:
   - Is the LLM generating specific enough queries?
   - Are the context keywords being extracted correctly?
   - Are the right documents being boosted by re-ranking?
3. Further tune based on debug output:
   - Adjust keyword extraction logic if needed
   - Adjust re-ranking boost scores if needed
   - Add more technical terms to the keyword list if needed
