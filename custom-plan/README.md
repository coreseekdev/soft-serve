# Soft Serve 定制化重构计划

## 概述

本计划旨在重构 Soft Serve，增加纯文件系统后端选项，与现有数据库后端作为互斥的编译选项，并简化部署和使用方式。

## 设计原则

1. **向后兼容**: 保留现有数据库后端代码
2. **编译时选择**: 文件系统后端 vs 数据库后端作为互斥的编译选项 (build tags)
3. **环境变量优先**: 环境变量优先级高于配置文件
4. **简化默认值**: 默认数据路径改为当前目录

## 需求清单

| # | 需求 | 优先级 |
|---|------|--------|
| 1 | 新增文件系统后端，与数据库后端互斥（编译选项） | P0 |
| 2 | `SOFT_SERVE_CONFIG_LOCATION` 沿用，支持 JSON/YAML | P0 |
| 3 | `SOFT_SERVE_DATA_PATH` 保持 `data`，FileStore 后端默认改为当前路径 | P0 |
| 4 | 环境变量优先级高于配置文件 | P0 |
| 5 | 自动发现当前目录下 Git 仓库，支持 bare 和普通仓库 | P0 |
| 6 | LFS 对象存储在 serve 目录下 `.lfs/`，可配置 | P1 |
| 7 | 用户管理基于目录结构（文件系统后端） | P0 |
| 8 | Admin 认证默认使用 `~/.ssh/id_*.pub`，可配置 | P0 |

## 文档索引

- [01-架构设计.md](./01-架构设计.md) - 整体架构设计
- [02-配置系统.md](./02-配置系统.md) - 配置系统重构
- [03-存储层.md](./03-存储层.md) - 双后端存储实现
- [04-用户管理.md](./04-用户管理.md) - 基于目录的用户管理
- [05-仓库发现.md](./05-仓库发现.md) - 自动仓库发现机制
- [06-修改清单.md](./06-修改清单.md) - 详细代码修改清单
- [10-聊天功能.md](./10-聊天功能.md) - IRC 风格聊天功能

## 编译选项

```bash
# 使用文件系统后端（默认）
go build -tags filestore ./cmd/soft

# 使用数据库后端
go build -tags dbstore ./cmd/soft

# 或通过 Makefile
make build-filestore   # 文件系统版本
make build-dbstore     # 数据库版本
```

## 目录结构（文件系统后端）

```
./                                    # SOFT_SERVE_DATA_PATH (默认当前目录)
├── config.yaml                       # 配置文件 (或 config.json)
├── ssh/
│   ├── host_ed25519                  # 服务器主机密钥
│   └── host_ed25519.pub
├── log/
│   └── soft-serve.log                # 日志文件
├── users/                            # 用户目录
│   ├── alice/
│   │   └── .ssh/
│   │       ├── id_ed25519.pub
│   │       └── id_rsa.pub
│   └── bob/
│       └── .ssh/
│           └── id_ed25519.pub
├── project-a.git/                    # bare repo
├── project-b/                        # 普通 repo
├── internal-tool.git/
└── .lfs/                             # LFS 对象存储
```

## 环境变量

### 沿用环境变量

| 变量 | 说明 | 变更 |
|------|------|------|
| `SOFT_SERVE_CONFIG_LOCATION` | 配置文件路径 | 沿用，支持 JSON/YAML |
| `SOFT_SERVE_DATA_PATH` | 数据目录 | 沿用 (默认 `data`)，FileStore 后端自动改为 `.` |

### 配置优先级

```
环境变量 > 配置文件 > 默认值
```

例如：
- 配置文件中 `ssh.listen_addr: ":2222"`
- 环境变量 `SOFT_SERVE_SSH_LISTEN_ADDR=":3333"`
- 最终使用 `:3333`

## 配置示例

```yaml
# config.yaml (或 config.json)
name: My Git Server

# 用户目录 (可选，默认 {DATA_PATH}/users)
users_path: ./users

# SSH 配置
ssh:
  listen_addr: :23231
  public_url: ssh://localhost:23231

# HTTP 配置
http:
  listen_addr: :23232
  public_url: http://localhost:23232

# LFS 配置
lfs:
  enabled: true
```

**固定路径 (不可配置):**
- SSH 密钥: `{DATA_PATH}/ssh`
- 日志: `{DATA_PATH}/log`
- LFS: `{DATA_PATH}/.lfs`
- Admin 公钥: `~/.ssh`

## 后端对比

| 特性 | 文件系统后端 | 数据库后端 |
|------|-------------|-----------|
| 依赖 | 无 | SQLite/PostgreSQL |
| 用户管理 | 目录结构 | 数据库表 |
| 仓库元数据 | 自动发现 + 文件 | 数据库表 |
| Webhook | 仓库配置文件 | 数据库表 |
| 访问令牌 | 不支持 | 支持 |
| 适合场景 | 个人/小团队 | 团队/生产环境 |

## 快速开始

### 文件系统后端

```bash
# 1. 在包含 git 仓库的目录下
cd ~/my-projects

# 2. 直接运行（默认文件系统后端）
soft serve

# 3. 或指定数据目录
SOFT_SERVE_DATA_PATH=/var/git soft serve
```

### 数据库后端

```bash
# 编译数据库版本
go build -tags dbstore -o soft-db ./cmd/soft

# 运行
SOFT_SERVE_DATA_PATH=/var/soft-serve soft-db serve
```
