# u115pan

这是项目内置的 115 开放平台接口层，供 Meta2Cloud 的 `pan115` 存储后端复用。

已提供的能力：

- PKCE 扫码登录
- access token / refresh token 持久化
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

## 使用示例

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

云下载示例：

```python
import u115pan

client = u115pan.Pan115Client.from_token_file(
    client_id="100197847",
    token_path="config/115-token.json",
)
offline = u115pan.OfflineClient(client)

quota = offline.get_quota_info()
tasks = offline.get_all_tasks()
print(quota.surplus, len(tasks))
```

## 说明

- 当前目录名为 `u115pan`，可直接作为合法 Python 包名使用。
- Meta2Cloud 已通过 `storage/pan115.py` 接入此接口层。
- 如需调用云下载能力，可直接 `import u115pan` 或通过 `Pan115Provider.raw_client` 访问底层客户端。
