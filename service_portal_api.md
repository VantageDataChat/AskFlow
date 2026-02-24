# Service Portal API — Marketplace 对接指南

本文档供 Marketplace 开发者参考，列出所有需要向 Service Portal 发起的请求。

> Base URL: `https://service.vantagics.com`

---

## 1. 店铺注册

**调用时机：** 店铺主点击「申请开通客户支持」，Marketplace 验证资格并获取 Auth_Token 后调用。

```
POST /api/store-support/register
Content-Type: application/json
```

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "software_name": "vantagics",
  "store_name": "我的数据分析小铺",
  "welcome_message": "欢迎来到我的数据分析小铺的客户支持",
  "parent_product_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "storefront_id": 123
}
```

| 字段 | 类型 | 必填 | 来源 / 说明 |
|------|------|------|-------------|
| token | string | 是 | `POST license.vantagedata.chat/api/marketplace-auth` 返回的 token |
| software_name | string | 是 | 固定传 `"vantagics"` |
| store_name | string | 是 | `author_storefronts.store_name` |
| welcome_message | string | 是 | 店铺介绍原文；若为空则传 `"欢迎来到 {store_name} 的客户支持"` |
| parent_product_id | string | 是 | Service Portal 的父产品 ID（从 Service Portal 管理后台「产品管理」页面获取） |
| storefront_id | int64 | 是 | `author_storefronts.id`，用于后续登录时匹配店铺 |

**成功响应 (200)：**

```json
{ "success": true, "message": "注册成功" }
```

**失败响应：**

```json
{ "success": false, "message": "token 验证失败" }
```

---

## 2. 同步欢迎语

**调用时机：** 店铺主保存店铺设置且 description 发生变化时，异步调用（`go` 协程）。

```
POST /api/store-support/update-welcome
Content-Type: application/json
```

```json
{
  "storefront_id": 123,
  "welcome_message": "更新后的欢迎语文本"
}
```

| 字段 | 类型 | 必填 | 来源 / 说明 |
|------|------|------|-------------|
| storefront_id | int64 | 是 | `author_storefronts.id` |
| welcome_message | string | 是 | 新的店铺介绍；若为空则传默认值 |

**成功响应 (200)：**

```json
{ "success": true }
```

---

## 3. 获取登录 Ticket（通用）

店主登录管理后台、客户进入客服界面都需要先获取一次性 ticket。

```
POST /api/auth/sn-login
Content-Type: application/json
```

```json
{ "license_token": "eyJhbGciOiJIUzI1NiIs..." }
```

| 字段 | 类型 | 必填 | 来源 / 说明 |
|------|------|------|-------------|
| license_token | string | 是 | `POST license.vantagedata.chat/api/marketplace-auth` 返回的 token |

**成功响应 (200)：**

```json
{ "success": true, "login_ticket": "7d8cf5ee-ecab-5904-93de-7e9f999f86b1" }
```

> Ticket 有效期 5 分钟，一次性使用。

---

## 4. 店主进入管理后台

获取 ticket 后，构造 URL 让浏览器新标签页打开：

```
https://service.vantagics.com/auth/ticket-login?ticket={login_ticket}&scope=store&store_id={storefront_id}
```

| 参数 | 类型 | 必填 | 来源 / 说明 |
|------|------|------|-------------|
| ticket | string | 是 | 步骤 3 返回的 `login_ticket` |
| scope | string | 是 | 固定传 `"store"` |
| store_id | int | 是 | `author_storefronts.id` |

**示例：**

```
https://service.vantagics.com/auth/ticket-login?ticket=7d8cf5ee-ecab-5904-93de-7e9f999f86b1&scope=store&store_id=1
```

**行为：** Service Portal 验证 ticket 后，自动创建店铺管理会话，重定向到管理后台。店主只能看到文档管理、问题管理、知识录入、FAQ 管理四个模块，操作范围限定在自己店铺的子产品下。

---

## 5. 客户进入店铺客服界面

获取 ticket 后，构造 URL 让浏览器新标签页打开：

```
https://service.vantagics.com/auth/ticket-login?ticket={login_ticket}&scope=customer&store_id={storefront_id}&product={url_encoded_product_name}
```

| 参数 | 类型 | 必填 | 来源 / 说明 |
|------|------|------|-------------|
| ticket | string | 是 | 步骤 3 返回的 `login_ticket` |
| scope | string | 是 | 固定传 `"customer"` |
| store_id | int | 是 | `author_storefronts.id` |
| product | string | 否 | 格式：`"vantagics-{store_name}"`，需 URL encode |

**示例：**

```
https://service.vantagics.com/auth/ticket-login?ticket=7d8cf5ee-ecab-5904-93de-7e9f999f86b1&scope=customer&store_id=123&product=vantagics-%E6%88%91%E7%9A%84%E5%BA%97
```

**行为：** Service Portal 验证 ticket 后，创建普通客户会话，自动切换到该店铺的产品，进入客服聊天界面（查看 FAQ、文档、提问）。

---

## 完整调用时序

### 开通申请

```
Marketplace → License_Server: POST /api/marketplace-auth {sn, email}
License_Server → Marketplace: {token}
Marketplace → Service_Portal: POST /api/store-support/register {token, software_name, store_name, welcome_message, parent_product_id, storefront_id}
Service_Portal → Marketplace: {success: true}
```

### 店主登录管理后台

```
Marketplace → License_Server: POST /api/marketplace-auth {sn, email}
License_Server → Marketplace: {token}
Marketplace → Service_Portal: POST /api/auth/sn-login {license_token: token}
Service_Portal → Marketplace: {login_ticket}
Marketplace → 浏览器: window.open(service_portal_url + "/auth/ticket-login?ticket={ticket}&scope=store&store_id={id}")
```

### 客户进入客服

```
Marketplace → License_Server: POST /api/marketplace-auth {sn, email}
License_Server → Marketplace: {token}
Marketplace → Service_Portal: POST /api/auth/sn-login {license_token: token}
Service_Portal → Marketplace: {login_ticket}
Marketplace → 浏览器: window.open(service_portal_url + "/auth/ticket-login?ticket={ticket}&scope=customer&store_id={id}&product=vantagics-{url_encode(store_name)}")
```

### 欢迎语同步

```
Marketplace（异步）→ Service_Portal: POST /api/store-support/update-welcome {storefront_id, welcome_message}
```
