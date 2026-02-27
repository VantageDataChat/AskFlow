# 文档删除后重复检测问题调试

## 问题描述

用户报告：删除文档后，再次上传相同文档会因为重复检测失败。

## 问题分析

### 1. 删除逻辑检查

`DeleteDocument()` 函数的删除流程：
- ✅ 调用 `vectorStore.DeleteByDocID()` 删除向量数据
- ✅ 使用事务删除 `video_segments` 表记录
- ✅ 使用事务删除 `documents` 表记录（包含 `content_hash` 字段）
- ✅ 提交事务
- ✅ 删除文件目录

### 2. 重复检测逻辑

`findDocumentByContentHash()` 函数：
```sql
SELECT id FROM documents WHERE content_hash = ? AND status != 'failed' LIMIT 1
```

### 3. 上传流程

`UploadFile()` 函数：
1. 计算文件的 SHA256 哈希值（`fileHash()`）
2. 调用 `findDocumentByContentHash()` 检查是否存在相同哈希的文档
3. 如果存在，返回"文档内容重复"错误

### 4. 可能的原因

理论上删除逻辑是完整的，但可能存在以下问题：

1. **事务未正确提交**：虽然代码中有 `tx.Commit()`，但可能存在错误处理不当
2. **数据库连接缓存**：SQLite WAL 模式下可能存在读缓存
3. **并发问题**：删除和上传操作之间存在竞态条件
4. **content_hash 未正确设置**：某些文档的 `content_hash` 可能为空字符串

## 解决方案

### 已实施的改进

1. **增强日志记录**：
   - 删除前记录 `content_hash` 值
   - 记录删除的行数
   - 删除后验证记录是否真正删除
   - 上传时记录哈希值和重复检测结果

2. **验证删除结果**：
   - 在事务提交后，立即查询验证记录是否已删除
   - 如果记录仍存在，记录错误日志

3. **改进错误处理**：
   - 检查 `RowsAffected()` 确保删除操作真正执行
   - 区分 `sql.ErrNoRows` 和其他错误

### 调试步骤

用户可以通过以下步骤复现和调试问题：

1. **上传一个文档**：
   ```
   查看日志：[UploadFile] Checking for duplicate: file=xxx hash=xxxxxxxx
   ```

2. **删除该文档**：
   ```
   查看日志：
   [DeleteDocument] Starting deletion for docID=xxx
   [DeleteDocument] Document content_hash=xxxxxxxx
   [DeleteDocument] Deleted X video segments
   [DeleteDocument] Deleted document record (rows=1)
   [DeleteDocument] Transaction committed successfully
   [DeleteDocument] Verified: Document record deleted successfully
   ```

3. **再次上传相同文档**：
   ```
   查看日志：
   [UploadFile] Checking for duplicate: file=xxx hash=xxxxxxxx
   [findDocumentByContentHash] Found duplicate document: hash=xxx docID=xxx (如果仍然重复)
   或
   [UploadFile] No duplicate found, proceeding with upload (如果已修复)
   ```

### 潜在的进一步修复

如果日志显示删除成功但仍然检测到重复，可能需要：

1. **强制刷新 WAL**：
   ```go
   _, err := dm.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
   ```

2. **使用 IMMEDIATE 事务**：
   ```go
   tx, err := dm.db.Begin()
   // 改为
   tx, err := dm.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
   ```

3. **清空 content_hash 而不是删除记录**：
   ```go
   UPDATE documents SET content_hash = '', status = 'deleted' WHERE id = ?
   ```

## 测试建议

1. 上传一个小文件（如文本文件）
2. 记录其 `docID` 和 `content_hash`
3. 删除该文档
4. 检查日志确认删除成功
5. 立即再次上传相同文件
6. 检查是否仍然报告重复

## 相关文件

- `internal/document/manager.go`: `DeleteDocument()`, `findDocumentByContentHash()`, `UploadFile()`
- `sqlite-vec/store.go`: `DeleteByDocID()`
- `internal/db/db.go`: 数据库初始化和 schema

## 更新日期

2026-02-28
