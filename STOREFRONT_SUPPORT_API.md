# 店铺客户支持系统 — 接口对接文档

本文档描述 Marketplace Server（`market.vantagedata.chat`）与 Service Portal（`service.vantagedata.chat`）之间的接口约定。

> `POST /api/auth/sn-login` 和 `GET /auth/ticket-login` 在上次桌面应用"客户服务"对接时已实现（详见 `doc/service_login.md`）。本次店铺支持系统复用 sn-login，仅需扩展 ticket-login 支持 `scope` 和 `store_id` 参数。

## 概述

- Marketplace 负责：资格校验、开通申请管理、管理员审批、状态存储
- Service Portal 负责：实际的客服功能（文档管理、问题管理、知识录入、FAQ 管理）

交互关系：

```
店铺主浏览器 → Marketplace → License_Server（获取 Auth_Token）→ Service_Portal（注册/登录）
Service_Portal → Marketplace（查询店铺批准状态）
```

## 工作量总结

| 接口 | 状态 | 工作量 |
|------|------|--------|
| POST /api/store-support/register | **新增** | 全新实现 |
| POST /api/store-support/update-welcome | **新增** | 全新实现 |
| POST /api/auth/sn-login | 已有 | 无需修改 |
| GET /auth/ticket-login | 已有 | 需扩展 scope + store_id 参数 |
| GET /api/storefront-support/check | Marketplace 提供 | Service Portal 调用即可 |

---

## 一、需新增的接口

### 1. POST /api/store-support/register

店铺主在 Marketplace 申请开通客户支持时，Marketplace 调用此接口向 Service Portal 注册店铺支持信息。

**调用时机：** 店铺主点击"申请开通客户支持"，Marketplace 验证资格并获取 Auth_Token 后调用。

**请求：**

```http
POST https://service.vantagedata.chat/api/store-support/register
Content-Type: application/json
```

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "software_name": "vantagics",
  "store_name": "我的数据分析小铺",
  "welcome_message": "欢迎来到我的数据分析小铺的客户支持"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | License_Server 签发的 Auth_Token（JWT），用于验证请求来源身份 |
| software_name | string | 是 | 固定为 `"vantagics"` |
| store_name | string | 是 | 店铺名称，取自 Marketplace 的 `author_storefronts.store_name` |
| welcome_message | string | 是 | 欢迎语。若店铺介绍非空则为店铺介绍原文；若为空则为默认值 `"欢迎来到 {store_name} 的客户支持"` |

**成功响应（200）：**

```json
{
  "success": true,
  "message": "注册成功"
}
```

**失败响应：**

```json
{
  "success": false,
  "message": "token 验证失败"
}
```

**实现要求：**
- 验证 `token` 的有效性（与 License_Server 的签发密钥一致）
- 记录该店铺的支持系统注册信息，关联 software_name、store_name、welcome_message
- 此时 Marketplace 侧状态为 `pending`，需等待管理员在 Marketplace 后台批准后才变为 `approved`

---

### 2. POST /api/store-support/update-welcome

店铺主在 Marketplace 更新店铺介绍时，若支持系统状态为 `approved`，Marketplace 异步调用此接口同步欢迎语。

**调用时机：** 店铺主保存店铺设置且 description 发生变化时，Marketplace 后台异步调用（`go` 协程）。

**请求：**

```http
POST https://service.vantagedata.chat/api/store-support/update-welcome
Content-Type: application/json
```

```json
{
  "storefront_id": 123,
  "welcome_message": "更新后的欢迎语文本"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| storefront_id | int64 | 是 | 店铺 ID，对应 Marketplace 的 `author_storefronts.id` |
| welcome_message | string | 是 | 新的欢迎语。若店铺介绍为空则为默认值 `"欢迎来到 {store_name} 的客户支持"` |

**成功响应（200）：**

```json
{
  "success": true
}
```

**实现要求：**
- 根据 `storefront_id` 找到对应的店铺支持记录
- 更新该记录的 welcome_message
- 此接口为异步调用，Marketplace 不依赖响应内容，仅检查 HTTP 状态码是否为 200

---

## 二、已有接口需扩展

### 3. POST /api/auth/sn-login（无需修改）

此接口在上次桌面应用"客户服务"对接时已实现（详见 `doc/service_login.md`）。店铺支持系统复用同一接口，调用方式完全一致。

Marketplace 在店铺主点击"进入客服后台"时调用，获取一次性 login_ticket。

**请求：**

```json
POST /api/auth/sn-login
{ "license_token": "eyJhbGciOiJIUzI1NiIs..." }
```

**响应：**

```json
{ "success": true, "login_ticket": "ticket_abc123def456" }
```

无需任何修改，与现有实现完全兼容。

---

### 4. GET /auth/ticket-login（需扩展）

此页面在上次对接时已实现基础版本。本次需扩展支持 `scope` 和 `store_id` 两个新参数，以创建店铺管理角色会话。

**URL 格式：**

```
https://service.vantagedata.chat/auth/ticket-login?ticket={login_ticket}&scope=store&store_id={storefront_id}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ticket | string | 是 | 从 `/api/auth/sn-login` 获取的一次性登录凭证（已有） |
| scope | string | 否 | **新增参数**。值为 `store` 时表示店铺管理角色；不传时保持原有行为 |
| store_id | int | 条件必填 | **新增参数**。当 `scope=store` 时必填。店铺 ID，对应 Marketplace 的 `author_storefronts.id` |

**扩展要求（相对于现有实现）：**
- 当 `scope=store` 且 `store_id` 存在时，创建店铺管理会话（而非普通用户会话）
- 店铺管理会话权限限定为以下四个模块：
  - 文档管理（添加、编辑、删除文档资料）
  - 问题管理（查看和回答客户问题）
  - 知识录入（新增、编辑、删除知识条目）
  - FAQ 管理（新增、编辑、删除 FAQ 条目）
- 操作范围限定在 `store_id` 对应的店铺数据内
- 隐藏其他管理模块（系统设置、用户管理等）
- 页面顶部显示当前管理的店铺名称
- 不传 `scope` 参数时保持原有行为不变（向后兼容）

---

## 三、Marketplace 提供的回调查询接口

### GET /api/storefront-support/check

Service Portal 在生成店铺支持模块前，应调用此接口验证店铺是否已被 Marketplace 管理员批准。

**URL：**

```
https://market.vantagedata.chat/api/storefront-support/check?storefront_id={id}
```

或

```
https://market.vantagedata.chat/api/storefront-support/check?store_slug={slug}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| storefront_id | int64 | 二选一 | 店铺 ID |
| store_slug | string | 二选一 | 店铺 slug（URL 友好标识） |

**已批准响应（200）：**

```json
{
  "approved": true,
  "store_name": "我的数据分析小铺",
  "welcome_message": "欢迎来到我的数据分析小铺的客户支持",
  "software_name": "vantagics"
}
```

**未批准响应（200）：**

```json
{
  "approved": false,
  "status": "none"
}
```

| status 值 | 含义 |
|-----------|------|
| none | 该店铺未提交过开通请求 |
| pending | 开通请求待审批 |
| disabled | 已被管理员禁用 |

**错误响应：**

| HTTP 状态码 | 响应 | 说明 |
|------------|------|------|
| 400 | `{"error": "storefront_id or store_slug is required"}` | 缺少参数 |
| 400 | `{"error": "invalid storefront_id"}` | storefront_id 格式无效 |
| 404 | `{"error": "storefront not found"}` | 店铺不存在 |

**使用建议：**
- Service Portal 在为店铺生成支持模块前调用此接口
- 仅当 `approved == true` 时才允许生成 Store_Support_Module
- 可缓存结果，建议缓存时间不超过 5 分钟

---

## 四、认证流程说明

所有涉及 Auth_Token 的接口，token 均由 License_Server（`license.vantagedata.chat`）签发。

Marketplace 通过以下接口获取 Auth_Token：

```http
POST https://license.vantagedata.chat/api/marketplace-auth
Content-Type: application/json

{ "sn": "用户的序列号", "email": "用户绑定的邮箱" }
```

响应：

```json
{ "success": true, "token": "eyJhbGciOiJIUzI1NiIs..." }
```

Service Portal 验证 token 时需与 License_Server 使用相同的签发密钥或调用 License_Server 的验证接口。

---

## 五、状态流转图

```
                    ┌──────────┐
                    │  未申请   │
                    │  (none)  │
                    └────┬─────┘
                         │ 店铺主申请开通
                         ▼
                    ┌──────────┐
          ┌────────│  待审批   │────────┐
          │        │ (pending) │        │
          │        └──────────┘        │
          │ 管理员批准                   │ 管理员禁用
          ▼                            ▼
    ┌──────────┐                ┌──────────┐
    │  已批准   │◄───────────────│  已禁用   │
    │(approved)│  管理员重新批准  │(disabled)│
    └────┬─────┘                └──────────┘
         │ 管理员禁用                   ▲
         └─────────────────────────────┘
```

---

## 六、接口调用时序总览

### 开通申请流程

```
店铺主 → Marketplace: POST /user/storefront/support/apply
Marketplace → License_Server: POST /api/marketplace-auth {sn, email}
License_Server → Marketplace: {token}
Marketplace → Service_Portal: POST /api/store-support/register {token, software_name, store_name, welcome_message}
Service_Portal → Marketplace: {success: true}
Marketplace: 创建 storefront_support_requests 记录 (status=pending)
```

### 一键登录流程

```
店铺主 → Marketplace: POST /user/storefront/support/login
Marketplace → License_Server: POST /api/marketplace-auth {sn, email}
License_Server → Marketplace: {token}
Marketplace → Service_Portal: POST /api/auth/sn-login {license_token}  ← 已有接口
Service_Portal → Marketplace: {login_ticket}
Marketplace → 店铺主: {login_url}
店铺主浏览器: 新标签页打开 /auth/ticket-login?ticket=xxx&scope=store&store_id=123
```

### 欢迎语同步流程

```
店铺主 → Marketplace: 保存店铺设置（更新 description）
Marketplace（异步）→ Service_Portal: POST /api/store-support/update-welcome {storefront_id, welcome_message}
```

### 状态查询流程

```
Service_Portal → Marketplace: GET /api/storefront-support/check?storefront_id=123
Marketplace → Service_Portal: {approved: true, store_name, welcome_message, software_name}
```
