# 产品名称显示"用户问题:"前缀问题修复

## 问题描述

登录界面显示的系统名称中出现了多余的"用户问题:"前缀文字。例如：
- 期望显示：`奇安信Mousika 深度内容检测引擎技术支持自服务系统`
- 实际显示：`用户问题: 奇安信Mousika 深度内容检测引擎技术支持自服务系统`

## 根本原因

在 `internal/llm/service.go` 的 `BuildMessages` 函数中，所有通过 `Generate` 方法发送的用户消息都会自动添加 `"用户问题："` 前缀。

当系统需要翻译产品名称或其他文本时，这些文本被当作"问题"参数传递给 `Generate` 方法，导致翻译结果中包含了不应该出现的前缀。

## 解决方案

### 1. 新增 `GenerateSimple` 方法

在 `internal/llm/service.go` 中添加了一个新的方法，用于简单的文本生成任务，不添加任何前缀：

```go
// GenerateSimple sends a simple prompt and text to the LLM without adding "用户问题：" prefix.
// This is useful for translation and other tasks where the input should not be prefixed.
func (s *APILLMService) GenerateSimple(systemPrompt string, userText string) (string, error)
```

### 2. 新增 `TranslateToUserLanguage` 方法

添加了专门用于根据用户问题语言进行翻译的方法：

```go
// TranslateToUserLanguage translates content to match the user's question language.
// It analyzes the user's question to detect the language and translates the content accordingly.
func (s *APILLMService) TranslateToUserLanguage(content string, userQuestion string) (string, error)
```

这个方法：
- 分析用户问题的语言
- 将内容翻译为相同的语言
- 不会给翻译内容添加"用户问题:"前缀
- 翻译失败时返回原始内容，不会导致系统错误

### 3. 优化 `TranslateText` 方法

修改了 `internal/query/engine.go` 中的 `TranslateText` 方法：
- 使用新的 `GenerateSimple` 方法进行翻译
- 简化了错误处理逻辑
- 翻译失败时返回原始文本而不是错误

### 4. 修复所有翻译调用点

在 `internal/query/engine.go` 中修复了以下翻译场景：
- 产品介绍（greeting）的语言适配
- 无关问题提示消息的语言适配
- 待处理问题提示消息的语言适配（3处）

所有这些场景现在都使用 `TranslateToUserLanguage` 方法，避免了"用户问题:"前缀问题。

## 修改的文件

1. **internal/llm/service.go**
   - 添加 `GenerateSimple` 方法
   - 添加 `TranslateToUserLanguage` 方法

2. **internal/query/engine.go**
   - 优化 `TranslateText` 方法
   - 修复产品介绍翻译调用
   - 修复无关问题提示翻译调用
   - 修复待处理问题提示翻译调用（3处）

## 优化点

1. **更好的错误处理**：翻译失败时返回原始文本，不会影响用户体验
2. **代码复用**：创建了专门的翻译方法，避免重复代码
3. **类型安全**：使用类型断言确保方法可用性
4. **向后兼容**：保留了原有的 `Generate` 方法，不影响其他功能

## 测试建议

1. 配置产品名称为中文，访问登录页面，验证不显示"用户问题:"前缀
2. 配置产品名称为英文，切换语言，验证翻译功能正常
3. 测试用户提问时的语言自动适配功能
4. 测试待处理问题的提示消息语言适配

## 影响范围

- 登录界面系统名称显示
- 产品名称翻译功能
- 用户消息的语言自动适配
- 待处理问题提示消息

所有修改都是向后兼容的，不会影响现有功能。
