# SSH 连接指南

## 服务器配置

从您的配置文件可以看到：
- **SSH 地址**: `localhost:23231`
- **公共 URL**: `ssh://localhost:23231`
- **主机密钥**: `C:\Users\limn\.ssh\soft_serve_host_ed25519`

## 连接方式

### 方式 1: 命令行指定密钥

#### 使用 ssh 命令

```bash
# 基本连接（需要输入用户名）
ssh -p 23231 -i ~/.ssh/your_private_key username@localhost

# 连接到 Soft Serve TUI 界面
ssh -p 23231 -i ~/.ssh/your_private_key git@localhost
```

#### 使用 git clone

```bash
# 使用 SSH URL 克隆
git clone ssh://git@localhost:23231/repo-name.git

# 或使用 SCP 格式
git clone git@localhost:23231:repo-name.git
```

### 方式 2: 配置 SSH Config（推荐）

编辑 `~/.ssh/config` 文件：

```ssh-config
# Soft Serve 服务器配置
Host soft-serve
    HostName localhost
    Port 23231
    User git
    IdentityFile ~/.ssh/your_private_key
    StrictHostKeyChecking no
    UserKnownHostsFile ~/.ssh/known_hosts.soft-serve
    # 可选：指定主机密钥
    UserKnownHostsFile C:\Users\limn\.ssh\soft_serve_host_ed25519.pub
```

然后就可以简化连接：

```bash
# 连接 TUI
ssh soft-serve

# 克隆仓库
git clone soft-serve:repo-name.git
```

### 方式 3: 使用环境变量

```bash
# Windows PowerShell
$env:GIT_SSH_COMMAND="ssh -i ~/.ssh/your_private_key -p 23231"

# Windows CMD
set GIT_SSH_COMMAND=ssh -i ~/.ssh/your_private_key -p 23231

# Git Bash/Linux
export GIT_SSH_COMMAND="ssh -i ~/.ssh/your_private_key -p 23231"

# 然后使用 git
git clone ssh://git@localhost:23231/repo.git
```

## 身份验证说明

### 用户认证

Soft Serve 使用 **公钥认证**：

1. **服务器端**：需要将您的公钥添加到服务器
   - 对于 FileStore：将公钥放到 `data/users/{username}/.ssh/` 目录
   - 对于 DBStore：通过 `initial_admin_keys` 配置或 TUI 添加

2. **客户端**：使用对应的私钥连接

### 主机密钥验证

首次连接时，SSH 客户端会要求验证服务器主机密钥：

```
The authenticity of host '[localhost]:23231 ([::1]:23231)' can't be established.
ED25519 key fingerprint is SHA256:...
Are you sure you want to continue connecting (yes/no)?
```

**选项 1 - 自动接受（开发环境）**：
```bash
ssh -o StrictHostKeyChecking=no -p 23231 git@localhost
```

**选项 2 - 手动验证（生产环境）**：

1. 从服务器获取主机公钥：
```bash
cat C:\Users\limn\.ssh\soft_serve_host_ed25519.pub
```

2. 添加到您的 `known_hosts`：
```bash
# 在 ~/.ssh/known_hosts 中添加
[localhost]:23231 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...
```

## 常见场景

### 场景 1: 访问 TUI 界面

```bash
ssh -p 23231 -i ~/.ssh/my_key git@localhost
```

### 场景 2: 克隆仓库

```bash
# 使用完整 URL
git clone ssh://git@localhost:23231/my-repo.git

# 使用 SCP 语法
git clone git@localhost:23231:my-repo.git

# 使用 SSH Config（如果配置了）
git clone soft-serve:my-repo.git
```

### 场景 3: 推送到仓库

```bash
cd my-repo
git remote add origin ssh://git@localhost:23231/my-repo.git
git push -u origin main
```

### 场景 4: 使用聊天功能

```bash
# 连接到服务器后，在 TUI 中选择 Messages 页面
ssh -p 23231 -i ~/.ssh/my_key git@localhost
```

## 证书说明

### 什么是"定制证书"?

在 SSH 上下文中，"证书"可能指：

1. **SSH 密钥对**（最常见）:
   - 私钥：`~/.ssh/id_ed25519` 或 `~/.ssh/id_rsa`
   - 公钥：`~/.ssh/id_ed25519.pub`

2. **SSH 证书签名**（高级）:
   - 使用 CA 签发的用户证书
   - 证书有效期通常有限制

### 生成新的 SSH 密钥对

```bash
# Ed25519 密钥（推荐）
ssh-keygen -t ed25519 -C "your_email@example.com" -f ~/.ssh/soft_serve_user

# RSA 密钥（兼容性更好）
ssh-keygen -t rsa -b 4096 -C "your_email@example.com" -f ~/.ssh/soft_serve_user
```

### 将公钥添加到服务器

#### 方法 1: 通过配置文件（FileStore）

```bash
# 创建用户目录
mkdir -p data/users/myuser/.ssh

# 复制公钥
cp ~/.ssh/soft_serve_user.pub data/users/myuser/.ssh/id_ed25519.pub
```

#### 方法 2: 通过配置文件（所有后端）

编辑 `data/config.yaml`：

```yaml
initial_admin_keys:
  - "ssh-ed25519 AAAA... your_email@example.com"
```

## 故障排查

### 问题 1: Permission denied

```bash
# 检查密钥权限
chmod 600 ~/.ssh/your_private_key

# 确认使用了正确的私钥
ssh -v -p 23231 -i ~/.ssh/your_private_key git@localhost
```

### 问题 2: Host key verification failed

```bash
# 临时禁用主机密钥检查
ssh -o StrictHostKeyChecking=no -p 23231 git@localhost

# 或手动添加主机密钥
ssh-keyscan -p 23231 localhost >> ~/.ssh/known_hosts
```

### 问题 3: Connection refused

```bash
# 确认服务器正在运行
netstat -an | grep 23231

# 或在 Windows 上
netstat -an | findstr 23231
```

### 问题 4: 使用 TortoiseGit 或其他 GUI 工具

在工具的 SSH 配置中指定：
- **SSH 密钥路径**: 指向您的私钥文件
- **端口**: 23231
- **用户**: git

## 完整示例

```bash
# 1. 生成密钥对
ssh-keygen -t ed25519 -f ~/.ssh/soft_serve_ed25519

# 2. 将公钥添加到服务器
mkdir -p data/users/myuser/.ssh
cp ~/.ssh/soft_serve_ed25519.pub data/users/myuser/.ssh/id_ed25519.pub

# 3. 启动服务器
./soft.exe

# 4. 连接测试
ssh -p 23231 -i ~/.ssh/soft_serve_ed25519 myuser@localhost

# 5. 克隆仓库（如果有）
git clone ssh://myuser@localhost:23231/test.git
```

## 安全建议

1. **生产环境**：
   - 始终验证主机密钥
   - 使用强密钥类型（Ed25519 或 RSA 4096+）
   - 定期轮换密钥
   - 限制用户访问权限

2. **开发环境**：
   - 可以使用 `StrictHostKeyChecking no` 简化开发
   - 使用不同的密钥对区分环境

3. **密钥保护**：
   - 私钥文件权限应为 `600`
   - 使用密钥密码保护
   - 不要将私钥提交到版本控制
