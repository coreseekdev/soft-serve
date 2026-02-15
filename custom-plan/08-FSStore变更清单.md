# FS Store 变更清单

## 概述

本文档整理实现 FileStore 后端所需的所有变更，并对每个变更进行必要性和简化评估。

---

## 1. 配置变更

### 1.1 用户目录路径 (无配置项)

**设计决策**:
- `UsersPath` 不作为 Config 结构体字段
- 通过函数获取，仅 FileStore 激活时可访问
- 环境变量 `SOFT_SERVE_USER_HOME` 覆盖默认值

**实现**: `pkg/store/file/store.go`

```go
// GetUsersPath 返回用户目录路径
// 优先使用 SOFT_SERVE_USER_HOME 环境变量
// 默认为 {DATA_PATH}/users
func GetUsersPath(cfg *config.Config) string {
    if path := os.Getenv("SOFT_SERVE_USER_HOME"); path != "" {
        return expandPath(path)
    }
    return filepath.Join(cfg.DataPath, "users")
}

// FileStore 中使用
type FileStore struct {
    cfg       *config.Config
    usersPath string  // 运行时计算，不存储在 Config 中
    // ...
}

func NewStore(cfg *config.Config) (store.Store, error) {
    usersPath := GetUsersPath(cfg)

    fs := &FileStore{
        cfg:       cfg,
        usersPath: usersPath,
        // ...
    }
    // ...
}
```

| 评估项 | 结论 |
|--------|------|
| **必要性** | ✅ 必要。用户目录需要可配置 |
| **现有替代** | ❌ 无 |
| **简化方案** | 不修改 Config 结构体，通过函数获取 |

**决策**: ✅ 采用此方案，Config 结构体无变更

---

### ~~1.2 支持配置文件 JSON 格式~~

**决策**: ❌ **无需变更**

`SOFT_SERVE_CONFIG_LOCATION` 已支持指定任意 YAML 格式文件：

```bash
# 默认: {DATA_PATH}/config.yaml
soft serve

# 自定义配置文件
SOFT_SERVE_CONFIG_LOCATION=/etc/soft-serve/config.yaml soft serve
```

---

### 1.3 FileStore 覆盖默认 DataPath

**位置**: `pkg/store/file/store_filestore.go`

```go
func NewStore(cfg *config.Config) (store.Store, error) {
    if cfg.DataPath == "data" && os.Getenv("SOFT_SERVE_DATA_PATH") == "" {
        cfg.DataPath = "."  // 改为当前目录
    }
    // ...
}
```

| 评估项 | 结论 |
|--------|------|
| **必要性** | ✅ 必要。FileStore 设计目标是在当前目录服务 |
| **现有替代** | ❌ 无。DBStore 需要 `data` 目录 |
| **简化方案** | 可通过文档引导用户设置 `SOFT_SERVE_DATA_PATH=.` |

**简化方案评估**:
```bash
# 方案 A: 代码中覆盖 (当前方案)
soft serve  # 自动使用当前目录

# 方案 B: 用户手动设置 (简化方案)
SOFT_SERVE_DATA_PATH=. soft serve
# 或在 config.yaml 中设置
```

**决策**: ✅ 保留代码覆盖，用户体验更好

---

## 2. 存储层变更

### 2.1 新增 FileStore 实现

**位置**: `pkg/store/file/`

```
pkg/store/file/
├── store_filestore.go    # 主结构
├── user.go               # 用户管理
├── repo.go               # 仓库发现
├── collab.go             # 协作者
├── webhook.go            # Webhook
├── lfs.go                # LFS
├── token.go              # 访问令牌 (ErrNotSupported)
└── settings.go           # 系统设置 (ErrNotSupported)
```

| 评估项 | 结论 |
|--------|------|
| **必要性** | ✅ 核心。这是整个功能的实现 |
| **现有替代** | ❌ 无 |
| **简化方案** | 见下方详细分析 |

---

### 2.2 用户管理实现分析

**当前设计**: 从目录加载用户
```
{USERS_PATH}/
├── alice/.ssh/id_ed25519.pub
└── bob/.ssh/id_ed25519.pub
```

**简化方案 A**: 使用单个文件
```
{DATA_PATH}/users.yaml
```
```yaml
users:
  alice:
    keys:
      - ssh-ed25519 AAAA...
  bob:
    keys:
      - ssh-ed25519 BBBB...
```

| 对比 | 目录方案 | 单文件方案 |
|------|----------|-----------|
| 用户管理 | `ssh localhost user create alice` | 手动编辑文件 |
| 公钥管理 | 自动检测 `.ssh/*.pub` | 手动添加 |
| 多公钥 | 自动支持 | 需要数组 |
| Git 友好 | ✅ | ✅ |

**决策**: ✅ **保持目录方案**。更符合 Unix 哲学，支持 SSH 命令管理

---

### 2.3 仓库发现实现分析

**当前设计**: 自动扫描 `{DATA_PATH}` 下的 Git 仓库

**简化方案**: 使用配置文件显式声明
```yaml
repos:
  - name: project-a
    path: ./project-a.git
  - name: project-b
    path: ./project-b
```

| 对比 | 自动发现 | 显式配置 |
|------|----------|----------|
| 易用性 | ✅ 零配置 | 需要配置 |
| 灵活性 | 所有子目录 | 可选择性 |
| 性能 | 启动时扫描 | 直接读取 |
| 隐藏仓库 | 需额外配置 | 默认支持 |

**决策**: ✅ **保持自动发现**。符合 "约定优于配置" 原则

---

### 2.4 协作者/Webhook 存储分析

**当前设计**: 存储在仓库目录下的 `.soft-serve.json`

**简化方案 A**: 不支持 (FileStore 专注简单场景)
**简化方案 B**: 使用全局配置文件

| 对比 | 仓库配置 | 全局配置 | 不支持 |
|------|----------|----------|--------|
| 可移植性 | ✅ 随仓库迁移 | ❌ 分离 | - |
| 复杂度 | 中 | 低 | 最低 |
| 功能完整 | ✅ | ✅ | ❌ |

**决策**: ✅ **保持仓库配置文件**。Git 友好，可版本控制

---

### 2.5 访问令牌 - 不支持

```go
func (s *FileStore) CreateAccessToken(...) (*models.AccessToken, error) {
    return nil, store.ErrNotSupported
}
```

| 评估项 | 结论 |
|--------|------|
| **必要性** | ✅ 返回 ErrNotSupported 是必要的 |
| **现有替代** | 使用 SSH 密钥认证 |
| **简化方案** | 可考虑简单的令牌文件，但增加复杂度 |

**决策**: ✅ 保持不支持，返回明确错误

---

## 3. 编译选项变更

### 3.1 Build Tags

**位置**: 多个文件

```go
// pkg/store/database/store.go
//go:build dbstore

package database

// pkg/store/file/store.go
//go:build filestore

package file

// pkg/store/file/store_default.go (默认选择)
//go:build !dbstore

package file
// 导出别名或默认初始化
```

```go
// cmd/soft/serve/serve_filestore.go
//go:build filestore || !dbstore

package serve

func getStore(cfg *config.Config) (store.Store, error) {
    return file.NewStore(cfg)
}

// cmd/soft/serve/serve_dbstore.go
//go:build dbstore

package serve

func getStore(cfg *config.Config) (store.Store, error) {
    return database.NewStore(cfg)
}
```

| 评估项 | 结论 |
|--------|------|
| **必要性** | ✅ 必要。后端互斥，编译时确定 |
| **现有替代** | ❌ 无 |
| **简化方案** | 无更简方案 |

**编译命令**:
```bash
# 默认 FileStore
go build -o soft ./cmd/soft

# DBStore
go build -tags dbstore -o soft-db ./cmd/soft
```

---

## 4. 简化后的变更清单

### 必要变更

| # | 变更 | 位置 | 优先级 |
|---|------|------|--------|
| 1 | FileStore 主结构 + GetUsersPath | `pkg/store/file/store.go` | P0 |
| 2 | FileStore 用户管理 | `pkg/store/file/user.go` | P0 |
| 3 | FileStore 仓库发现 | `pkg/store/file/repo.go` | P0 |
| 4 | FileStore 协作者/Webhook | `pkg/store/file/collab.go`, `webhook.go` | P1 |
| 5 | FileStore LFS | `pkg/store/file/lfs.go` | P1 |
| 6 | FileStore 不支持功能 | `pkg/store/file/token.go`, `settings.go` | P1 |

### 移除的变更

| # | 变更 | 原因 |
|---|------|------|
| 1 | 配置文件格式变更 | `SOFT_SERVE_CONFIG_LOCATION` 已支持指定任意 YAML 文件 |
| 2 | `SOFT_SERVE_STORAGE_BACKEND` 环境变量 | 后端由编译时确定 |

### 可选简化

| # | 方案 | 影响 |
|---|------|------|
| 1 | 移除 `UsersPath`，固定为 `{DATA_PATH}/users` | 减少配置项 |

---

## 5. 最终建议

### 最小实现 (MVP)

```
必须实现:
├── pkg/store/file/
│   ├── store.go          # 主结构 + GetUsersPath + DataPath 覆盖
│   ├── user.go           # 用户管理 (目录结构)
│   ├── repo.go           # 仓库发现
│   ├── collab.go         # 协作者 (.soft-serve.json)
│   ├── webhook.go        # Webhook (.soft-serve.json)
│   ├── lfs.go            # LFS (文件系统)
│   ├── token.go          # ErrNotSupported
│   └── settings.go       # ErrNotSupported
│
└── cmd/soft/serve/
    ├── serve_filestore.go  # //go:build filestore || !dbstore
    └── serve_dbstore.go    # //go:build dbstore
```

### 编译命令

```bash
# FileStore (默认)
go build -o soft ./cmd/soft

# DBStore
go build -tags dbstore -o soft-db ./cmd/soft
```

### 新增环境变量 (最终: 仅 1 个)

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SOFT_SERVE_USER_HOME` | 用户目录 (仅 FileStore) | `{DATA_PATH}/users` |

---

## 6. 文件变更汇总

### 新增文件

```
pkg/store/file/
├── store.go
├── user.go
├── repo.go
├── collab.go
├── webhook.go
├── lfs.go
├── token.go
└── settings.go

cmd/soft/serve/
├── serve_filestore.go    # //go:build filestore || !dbstore
└── serve_dbstore.go      # //go:build dbstore
```

### 修改文件

| 文件 | 修改内容 |
|------|----------|
| `pkg/store/database/*.go` | 添加 `//go:build dbstore` |

### 无需修改

- `pkg/config/config.go` - 保持不变 (无新增字段)
- `pkg/db/` - 保持不变
- `pkg/backend/` - 保持不变 (通过 Store 接口)
