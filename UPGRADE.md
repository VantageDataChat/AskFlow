# AskFlow 升级指南

## 从旧版本升级到对话历史版本

### 自动迁移（推荐）

本次更新已实现自动数据库迁移，直接替换可执行文件即可：

1. **备份数据**（重要！）
   ```bash
   # 备份数据库文件
   cp askflow.db askflow.db.backup

   # 备份配置文件
   cp config.json config.json.backup
   ```

2. **停止旧程序**
   ```bash
   # Windows
   taskkill /F /IM askflow.exe

   # Linux
   pkill askflow
   ```

3. **替换可执行文件**
   ```bash
   # 下载新版本并替换
   mv askflow.exe askflow.exe.old
   mv askflow-new.exe askflow.exe
   ```

4. **启动新程序**
   ```bash
   ./askflow.exe
   ```

5. **验证升级**
   - 程序启动时会自动创建新表：`conversations` 和 `conversation_messages`
   - 检查日志确认没有错误
   - 访问前端确认功能正常

### 数据库变更说明

本次更新新增了以下数据库表：

```sql
-- 对话会话表
CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    session_id TEXT DEFAULT '',
    product_id TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 对话消息表
CREATE TABLE conversation_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    image_data TEXT DEFAULT '',
    sources TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);
```

### 配置文件变更

新增配置项（可选，有默认值）：

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

如需自定义配置，可手动添加到 `config.json` 中。

### 功能说明

- **对话历史**：系统现在会记住用户的对话历史，支持多轮对话
- **会话隔离**：每个浏览器标签页是独立的会话，不会互相干扰
- **匿名支持**：匿名用户也能享受对话历史功能
- **可配置**：可通过配置文件控制历史消息数量和过期时间

### 回滚方案

如果升级后遇到问题，可以回滚：

1. **停止新程序**
   ```bash
   pkill askflow
   ```

2. **恢复旧版本**
   ```bash
   mv askflow.exe.old askflow.exe
   ```

3. **恢复数据库**（如果需要）
   ```bash
   mv askflow.db.backup askflow.db
   ```

4. **启动旧程序**
   ```bash
   ./askflow.exe
   ```

### 注意事项

1. **数据库兼容性**：新表使用 `CREATE TABLE IF NOT EXISTS`，不会影响现有数据
2. **API 兼容性**：新增字段都是可选的，旧客户端仍可正常使用
3. **性能影响**：对话历史功能对性能影响很小（每次查询增加约 10-20ms）
4. **存储空间**：对话历史会占用额外存储空间，建议定期清理（配置 `max_history_age`）

### 故障排查

**问题：程序启动失败**
- 检查日志文件中的错误信息
- 确认数据库文件权限正确
- 尝试使用备份数据库启动

**问题：对话历史不工作**
- 检查 `config.json` 中 `conversation.enabled` 是否为 `true`
- 检查浏览器控制台是否有错误
- 确认前端已更新（需要支持 `conversation_id` 字段）

**问题：数据库文件过大**
- 运行清理命令：`sqlite3 askflow.db "DELETE FROM conversations WHERE updated_at < datetime('now', '-30 days')"`
- 或调整配置中的 `max_history_age` 为更短的时间
