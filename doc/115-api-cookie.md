# 115 API 文档

基于微信小程序 User-Agent 的 115 网盘 API 接口文档。

## 基础信息

| 项目 | 值 |
|------|-----|
| Base URL | `https://webapi.115.com` |
| Content-Type | `application/x-www-form-urlencoded` |
| 认证方式 | Cookie |

## 请求头

```http
Host: webapi.115.com
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.102 Safari/537.36 MicroMessenger/6.8.0(0x16080000) NetType/WIFI MiniProgramEnv/Mac MacWechat/WMPF XWEB/30626
Referer: https://servicewechat.com/wx2c744c010a61b0fa/94/page-frame.html
Cookie: UID=xxx; CID=xxx; SEID=xxx; ...
```

---

## 接口列表

### 1. 获取用户信息

验证 Cookie 有效性并获取用户基本信息。

**请求**

```http
GET /files/index_info
```

**响应**

```json
{
  "state": true,
  "data": {
    "user_name": "用户名"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| state | boolean | 请求是否成功 |
| data.user_name | string | 用户昵称 |

---

### 2. 获取文件夹列表

浏览指定目录下的文件夹。

**请求**

```http
GET /files?aid=1&cid={cid}&o=user_ptime&asc=0&offset=0&show_dir=1&limit=100&type=0&format=json
```

**参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| aid | number | 是 | 固定值 `1` |
| cid | string | 是 | 目录ID，根目录为 `0` |
| o | string | 是 | 排序字段，默认 `user_ptime` |
| asc | number | 是 | 排序方向，`0`=降序，`1`=升序 |
| offset | number | 是 | 偏移量，默认 `0` |
| show_dir | number | 是 | 是否显示文件夹，`1`=是 |
| limit | number | 是 | 每页数量，默认 `100` |
| type | number | 是 | 文件类型，`0`=全部 |
| format | string | 是 | 返回格式，固定 `json` |

**响应**

```json
{
  "state": true,
  "path": [
    { "cid": "0", "name": "根目录" },
    { "cid": "123456", "name": "视频" }
  ],
  "data": [
    {
      "cid": "789012",
      "n": "电影",
      "is_directory": true
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| path | array | 当前路径面包屑 |
| data[].cid | string | 文件夹ID |
| data[].n | string | 文件夹名称 |

---

### 3. 创建文件夹

在指定目录下创建新文件夹。

**请求**

```http
POST /files/add

pid={parentCid}&cname={folderName}
```

**参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pid | string | 是 | 父目录ID，根目录为 `0` |
| cname | string | 是 | 新文件夹名称 |

**响应**

```json
{
  "state": true,
  "data": {
    "cid": "新建文件夹ID",
    "file_name": "新建文件夹名称"
  }
}
```

---

### 4. 获取分享信息

获取分享链接的文件列表和元信息。

**请求**

```http
GET /share/snap?share_code={shareCode}&receive_code={receiveCode}&offset=0&limit=100&cid=
```

**参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| share_code | string | 是 | 分享码，从链接中提取 |
| receive_code | string | 是 | 提取码 |
| offset | number | 是 | 偏移量，默认 `0` |
| limit | number | 是 | 每页数量，默认 `100` |
| cid | string | 是 | 子目录ID，根目录传空 |

**响应**

```json
{
  "state": true,
  "data": {
    "userinfo": {
      "user_id": "10204712",
      "user_name": "分享者昵称"
    },
    "shareinfo": {
      "snap_id": "310925575",
      "file_size": 292911078140,
      "share_title": "分享标题",
      "create_time": 1747138075,
      "receive_count": 539,
      "expire_time": -1
    },
    "count": 1,
    "list": [
      {
        "cid": "3163525225962798897",
        "fid": null,
        "n": "文件或文件夹名称",
        "s": 292911078140,
        "t": "1747138069"
      }
    ]
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| shareinfo.share_title | string | 分享标题 |
| shareinfo.file_size | number | 总大小（字节） |
| shareinfo.receive_count | number | 被转存次数 |
| list[].cid | string | 文件夹ID（文件夹存在） |
| list[].fid | string | 文件ID（文件存在） |
| list[].n | string | 名称 |
| list[].s | number | 大小（字节） |

---

### 5. 转存文件

将分享的文件保存到自己的网盘。

**请求**

```http
POST /share/receive

cid={targetCid}&share_code={shareCode}&receive_code={receiveCode}&file_id={fileIds}
```

**参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| cid | string | 是 | 目标目录ID，根目录为 `0` |
| share_code | string | 是 | 分享码 |
| receive_code | string | 是 | 提取码 |
| file_id | string | 是 | 文件ID列表，多个用逗号分隔 |

**响应**

```json
{
  "state": true,
  "data": {
    "pid": 0,
    "recv_folder_count": 1,
    "recv_file_count": 0,
    "receive_title": "转存标题",
    "receive_size": 292911078140
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| recv_folder_count | number | 转存的文件夹数 |
| recv_file_count | number | 转存的文件数 |
| receive_title | string | 转存标题 |
| receive_size | number | 转存大小（字节） |

---

## 错误响应

```json
{
  "state": false,
  "error": "错误信息",
  "errno": 10001
}
```

| errno | 说明 |
|-------|------|
| -1 | 未登录或Cookie失效 |
| 10001 | 提取码错误 |
| 10002 | 分享链接无效 |
| 10003 | 分享已过期 |

---

## 链接解析

### 分享链接格式

```
https://115.com/s/{shareCode}?password={password}
https://115cdn.com/s/{shareCode}?password={password}
```

### 解析正则

```javascript
// 提取分享码
const codeMatch = url.match(/\/s\/([a-z0-9]+)/i);
const shareCode = codeMatch[1];

// 提取密码
const pwdMatch = url.match(/[?&]password=([^&#]+)/);
const password = pwdMatch ? pwdMatch[1] : "";
```

---

## Cookie 字段说明

| 字段 | 说明 |
|------|------|
| UID | 用户ID |
| CID | 客户端ID |
| SEID | 会话ID |
| KID | 密钥ID |
| GST | 全局会话Token |
| USERSESSIONID | 用户会话ID |

---

## 使用示例

### Node.js

```javascript
const axios = require('axios');
const qs = require('querystring');

const cookie = 'UID=xxx; CID=xxx; SEID=xxx; ...';

const headers = {
  'Cookie': cookie,
  'User-Agent': 'Mozilla/5.0 ... MicroMessenger/6.8.0 ...',
  'Content-Type': 'application/x-www-form-urlencoded'
};

// 获取分享信息
const shareInfo = await axios.get('https://webapi.115.com/share/snap', {
  headers,
  params: {
    share_code: 'swwhsp53hiv',
    receive_code: 'c9d0',
    offset: 0,
    limit: 100,
    cid: ''
  }
});

// 提取文件ID
const fileIds = shareInfo.data.data.list
  .map(item => item.cid || item.fid)
  .join(',');

// 转存到根目录
const result = await axios.post(
  'https://webapi.115.com/share/receive',
  qs.stringify({
    cid: '0',
    share_code: 'swwhsp53hiv',
    receive_code: 'c9d0',
    file_id: fileIds
  }),
  { headers }
);

console.log(result.data);
```

### cURL

```bash
# 获取分享信息
curl -s "https://webapi.115.com/share/snap?share_code=swwhsp53hiv&receive_code=c9d0&offset=0&limit=100&cid=" \
  -H "Cookie: UID=xxx; CID=xxx; SEID=xxx" \
  -H "User-Agent: Mozilla/5.0 ... MicroMessenger/6.8.0 ..."

# 转存文件
curl -X POST "https://webapi.115.com/share/receive" \
  -H "Cookie: UID=xxx; CID=xxx; SEID=xxx" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "cid=0&share_code=swwhsp53hiv&receive_code=c9d0&file_id=3163525225962798897"
```
