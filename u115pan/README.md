# u115pan

这是项目内置的 115 开放平台接口层，供 Meta2Cloud 的 `pan115` 存储后端复用。

已提供的能力：

- PKCE 扫码登录
- access token / refresh token 持久化
- 进程内共享 115 client runtime 管理
- 目录列表、路径查询、创建目录、删除、移动、复制、重命名
- 搜索
- 通过 `pick_code` 获取下载链接并下载文件
- 上传前置接口：`fileid/preid` 计算、初始化上传、二次认证辅助、STS 凭证、续传信息
- 空间查询、回收站列表
- 云下载接口独立封装在 `offline.py`

## 目录说明

- [client.py](./client.py): 主客户端
- [auth.py](./auth.py): PKCE 与 token 持久化工具
- [models.py](./models.py): 数据模型
- [errors.py](./errors.py): 异常定义
- [offline.py](./offline.py): 云下载接口
- [runtime.py](./runtime.py): 共享 client 运行时管理

## 推荐用法

对于 Meta2Cloud 这类长期运行的服务组件，推荐通过 runtime manager 获取 client，而不是在每个入口直接重新构造 `Pan115Client`。

这样做的目的：

- 统一管理同一进程内的 115 client 生命周期
- 在 token 文件更新后自动感知并重建 client
- 避免旧 `refresh_token` 长时间滞留在内存对象里
- 按 `(client_id, resolved_token_path)` 对不同配置分桶隔离，互不污染

示例：

```python
import u115pan

runtime = u115pan.get_runtime_manager()
client = runtime.get_client(
    client_id="100197847",
    token_path="/abs/path/to/config/115-token.json",
)

files = client.list_all_files(cid=0)
for item in files:
    print(item.name, item.is_folder, item.pick_code)
```

云下载示例：

```python
import u115pan

runtime = u115pan.get_runtime_manager()
client = runtime.get_client(
    client_id="100197847",
    token_path="/abs/path/to/config/115-token.json",
)
offline = u115pan.OfflineClient(client)

quota = offline.get_quota_info()
tasks = offline.get_all_tasks()
print(quota.surplus, len(tasks))
```

## 低级 API 示例

如果你只是写一次性调试脚本、临时 CLI 工具，或者明确不需要进程内共享状态，也可以直接构造 `Pan115Client`。

```python
from pathlib import Path
import importlib.util


spec = importlib.util.spec_from_file_location(
    "u115pan",
    Path("u115pan/__init__.py"),
)
u115pan = importlib.util.module_from_spec(spec)
spec.loader.exec_module(u115pan)

client = u115pan.Pan115Client.from_token_file(
    client_id="100197847",
    token_path="config/115-token.json",
)

files = client.list_all_files(cid=0)
for item in files:
    print(item.name, item.is_folder, item.pick_code)
```

## Token 生命周期

当前实现中的 token 生命周期如下：

1. 扫码授权成功后，将 token 写入 `token_json`
2. 后续请求如发现 `access_token` 过期，会自动调用 `refresh_token()`
3. 刷新成功后，新的 `access_token` 和 `refresh_token` 会写回 `token_json`
4. 如果当前 client 来自 runtime manager，进程内共享状态也会同步更新

这意味着：

- 正常情况下不需要手工刷新 token 文件
- 重新授权后，后续请求应能自动感知新 token
- 对于服务进程，推荐始终通过 runtime manager 拿 client

## 路径约定

- 推荐传入稳定、唯一的 `token_path`
- 最好使用绝对路径，或确保所有入口都指向同一个实际文件
- runtime manager 内部会按 `(client_id, resolved_token_path)` 管理共享 client
- 如果你故意传入不同的 `client_id` 或不同的 `token_path`，runtime 会把它们视为不同配置

## 故障排查

### 云下载正常，但媒体库刷新报 `refresh token error`

优先检查：

- 当前运行进程实际读取的 `client_id` 是否正确
- `token_json` 是否是当前 `client_id` 授权生成的
- 容器内实际读取到的 `config.yaml` 与 `115-token.json` 是否和宿主机一致

### 更换 Client ID 后仍然报 `refresh token error`

这通常意味着：

- `refresh_token` 仍然来自旧的 `client_id`
- 或者进程里仍持有旧 client / 旧 token 状态

建议做法：

- 删除旧 `token_json`
- 使用新的 `client_id` 重新扫码授权
- 确认授权后 token 文件确实更新

### 容器环境排查重点

- 先看容器内的 `config.yaml`
- 再看容器内实际使用的 `token_json`
- 不要只看宿主机文件
- 如果需要调试，优先在容器内确认 token 文件路径、时间戳、内容是否一致

## 说明

- 当前目录名为 `u115pan`，可直接作为合法 Python 包名使用。
- Meta2Cloud 已通过 `storage/pan115.py` 接入此接口层。
- WebUI、媒体库刷新、离线下载与 `pan115` 存储后端已统一到 runtime 管理思路下。
- 如需调用云下载能力，可直接 `import u115pan`，或通过 `Pan115Provider.raw_client` 访问底层客户端。
