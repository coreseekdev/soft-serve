# FileStore 后端使用指南

## 概述

FileStore 是 Soft Serve 的一个轻量级后端实现，使用文件系统而非数据库存储数据。适合个人使用、小型团队或需要简单部署的场景。

## 构建方式

### 构建 FileStore 版本

```bash
go build -tags filestore -o soft ./cmd/soft
```

### 构建 DBStore 版本（默认）

```bash
go build -tags dbstore -o soft ./cmd/soft
```

### 默认行为

如果不指定 build tag，默认使用 FileStore：

```bash
go build -o soft ./cmd/soft  # 等同于 filestore
```

## 目录结构

```
{DATA_PATH}/
├── users/                          # 用户目录 (可通过 SOFT_SERVE_USER_HOME 覆盖)
│   ├── alice/
│   │   └── .ssh/
│   │       ├── id_ed25519.pub      # Alice 的公钥
│   │       └── id_rsa.pub          # Alice 的另一个公钥
│   └── bob/
│       └── .ssh/
│           └── id_ed25519.pub
├── repos/                          # 仓库发现路径
│   ├── my-project.git/             # Bare 仓库
│   │   └── .soft-serve.json        # 仓库元数据
│   └── another-repo/               # 普通仓库 (含 .git 目录)
├── .lfs/                           # LFS 对象存储
├── hooks/                          # 自定义 Git hooks
│   └── update.sample
├── log/                            # 日志目录
├── ssh/                            # SSH 主机密钥
└── config.yaml                     # 配置文件
```

## 环境变量

### 专用环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SOFT_SERVE_USER_HOME` | 用户目录路径 | `{DATA_PATH}/users` |
| `SOFT_SERVE_DATA_PATH` | 数据目录路径 | 当前工作目录 |

### 标准环境变量

FileStore 同样支持 Soft Serve 的标准环境变量：

| 变量名 | 说明 |
|--------|------|
| `SOFT_SERVE_CONFIG_LOCATION` | 配置文件路径 |
| `SOFT_SERVE_SSH_LISTEN_ADDR` | SSH 监听地址 |
| `SOFT_SERVE_HTTP_LISTEN_ADDR` | HTTP 监听地址 |
| `SOFT_SERVE_GIT_LISTEN_ADDR` | Git 监听地址 |

## 用户管理

### 创建用户

用户目录结构：

```
users/{username}/.ssh/*.pub
```

只需创建目录并添加公钥文件即可：

```bash
# 创建用户目录
mkdir -p data/users/alice/.ssh

# 添加公钥
echo "ssh-ed25519 AAAA... alice@example.com" > data/users/alice/.ssh/id_ed25519.pub
```

### 用户名规则

- 必须以字母开头
- 只能包含字母、数字和连字符
- 不区分大小写（内部转换为小写）

有效用户名示例：
- `alice`
- `bob123`
- `user-name`

无效用户名示例：
- `123user` (以数字开头)
- `user@name` (包含特殊字符)
- `user/name` (包含斜杠)

### 管理员权限

管理员的公钥从 `~/.ssh/id_*.pub` 自动加载。拥有匹配公钥的用户自动获得管理员权限。

## 仓库管理

### 自动发现

FileStore 启动时自动扫描数据目录：

1. **Bare 仓库**: `{name}.git/` - 必须包含 `HEAD` 文件
2. **普通仓库**: `{name}/.git/` - 必须包含 `.git/HEAD` 文件

隐藏目录（以 `.` 开头）会被忽略。

### 仓库元数据

仓库设置存储在 `.soft-serve.json` 文件中：

```json
{
  "description": "My awesome project",
  "private": false,
  "hidden": false,
  "mirror": false,
  "project_name": "My Project",
  "collaborators": {
    "alice": "read-only",
    "bob": "read-write"
  },
  "webhooks": [
    {
      "url": "https://example.com/webhook",
      "secret": "webhook-secret",
      "events": [1, 2],
      "active": true
    }
  ]
}
```

### 协作者权限级别

- `read-only`: 只读访问
- `read-write`: 读写访问
- `admin`: 管理员访问

## 安全特性

### 路径遍历保护

FileStore 验证所有路径操作，防止：
- 通过 `../` 访问上级目录
- 通过符号链接逃逸

### 用户名验证

所有用户名都经过 `utils.ValidateUsername()` 验证，防止：
- 路径注入攻击
- 特殊字符滥用

### 文件权限

- SSH 密钥文件: `0600` (仅所有者可读写)
- SSH 目录: `0700` (仅所有者可访问)
- 用户目录: `0755`

## 限制

以下功能在 FileStore 中不支持（返回 `ErrNotSupported`）：

- **Access Tokens**: 不支持令牌认证
- **密码认证**: 不支持用户密码
- **LFS 锁**: 不支持 LFS 文件锁定

## 热重载

调用 `Reload()` 方法可以重新加载用户和仓库：

```go
store.Reload()
```

这在以下场景有用：
- 手动添加/删除用户目录后
- 手动添加/删除仓库后
- 修改配置后

## 迁移指南

### 从 DBStore 迁移到 FileStore

1. 导出现有用户和公钥
2. 创建 FileStore 目录结构
3. 将公钥复制到对应用户的 `.ssh/` 目录
4. 复制仓库到数据目录
5. 重新创建仓库元数据（如需要）

### 示例迁移脚本

```bash
#!/bin/bash
# 假设已从 DBStore 导出用户列表

# 创建用户目录
for user in alice bob charlie; do
    mkdir -p data/users/$user/.ssh
    # 复制公钥（需要从 DBStore 导出）
    # cp $user.pub data/users/$user/.ssh/id_ed25519.pub
done

# 复制仓库
cp -r /path/to/repos/*.git data/
```

## 常见问题

### Q: 如何添加新用户？

A: 创建 `users/{username}/.ssh/` 目录并添加 `.pub` 公钥文件，然后重启服务或调用热重载。

### Q: 如何设置管理员？

A: 将管理员的公钥放到运行 Soft Serve 的用户的 `~/.ssh/` 目录下，文件名格式为 `id_*.pub`。

### Q: 如何更改用户目录位置？

A: 设置 `SOFT_SERVE_USER_HOME` 环境变量。

### Q: 仓库不显示怎么办？

A: 确保：
1. 仓库目录名符合格式（`.git` 后缀或含 `.git` 子目录）
2. 仓库包含有效的 `HEAD` 文件
3. 目录不是隐藏目录（不以 `.` 开头）

### Q: 支持 Windows 吗？

A: 支持，但建议使用 Git Bash 或 WSL 环境。
