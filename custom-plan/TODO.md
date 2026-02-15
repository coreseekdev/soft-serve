# FileStore 后端实现 TODO

## 1. 实现完整性检查

### 已完成 ✅

| 功能 | 文件 | 状态 |
|------|------|------|
| FileStore 主结构 | `pkg/store/file/store.go` | ✅ |
| GetUsersPath 函数 | `pkg/store/file/store.go` | ✅ |
| DataPath 覆盖逻辑 | `pkg/store/file/store.go` | ✅ |
| 用户目录加载 | `pkg/store/file/user.go` | ✅ |
| 仓库自动发现 | `pkg/store/file/repo.go` | ✅ |
| 协作者管理 | `pkg/store/file/collab.go` | ✅ |
| Webhook 管理 | `pkg/store/file/webhook.go` | ✅ |
| LFS 基础支持 | `pkg/store/file/lfs.go` | ✅ |
| Token (ErrNotSupported) | `pkg/store/file/token.go` | ✅ |
| Settings | `pkg/store/file/settings.go` | ✅ |
| Build Tags | 所有文件 | ✅ |
| 初始化命令拆分 | `cmd/*.go` | ✅ |
| Serve 命令拆分 | `cmd/soft/serve/*.go` | ✅ |
| E2E 测试 | `testscript/testdata/` | ✅ |
| 单元测试 | `pkg/store/file/*_test.go` | ✅ |
| Reload 函数 | `pkg/store/file/store.go` | ✅ |

### 未完成/需补充 ⚠️

| 功能 | 问题 | 优先级 |
|------|------|--------|
| 仓库元数据保存 | `saveRepoMeta` 未处理 collaborator/webhook 更新时的并发问题 | P2 |
| LFS 锁功能 | 返回 `ErrNotSupported`，但 plan 中说支持 | P2 |
| 日志路径 | 未使用配置中的日志路径 | P3 |

---

## 2. 安全问题 ✅ 已修复

### 2.1 已修复的高风险问题

| 问题 | 位置 | 修复方案 | 状态 |
|------|------|----------|------|
| **路径遍历** | `validatePath()` | 添加路径白名单验证，检查是否在 basePath 内 | ✅ |
| **符号链接** | `checkSymlinks()` | 检查路径组件是否为符号链接 | ✅ |
| **用户名注入** | `CreateUser()` | 使用 `utils.ValidateUsername()` 验证 | ✅ |
| **公钥文件权限** | `.ssh/` 目录 | 修改为 0600 (keyPerms) | ✅ |
| **Admin 公钥** | `loadAdminKeys()` | 加载所有 id_*.pub 文件 | ✅ |

### 2.2 中风险

| 问题 | 位置 | 风险 | 建议 |
|------|------|------|------|
| **Webhook Secret** | `.soft-serve.json` | Secret 明文存储 | 可考虑加密存储 |
| **并发写入** | `saveRepoMeta()` | 多进程同时写入可能损坏文件 | 使用文件锁 `flock` |

### 2.3 低风险

| 问题 | 位置 | 风险 | 建议 |
|------|------|------|------|
| ID 碰撞 | `hashUsername()` | 哈希冲突可能性低 | 使用更强的哈希或 UUID |

---

## 3. 可重构/优化点

### 3.1 架构优化

| 优化点 | 当前 | 建议 |
|--------|------|------|
| **接口适配** | FileStore 直接实现 Store 接口，忽略 `db.Handler` 参数 | 创建适配层或修改接口 |
| **缓存机制** | 启动时一次性加载，支持 Reload | 添加 Watch 监听目录变化 |
| **错误处理** | 返回通用错误 | 包装为具体错误类型 |

### 3.2 代码优化

| 位置 | 问题 | 建议 |
|------|------|------|
| `store.go:87` | DataPath 覆盖逻辑硬编码 "data" | 使用常量或配置 |
| `user.go` | `hashUsername` 简单哈希 | 使用 `xxhash` 或 `fnv` |
| `repo.go` | `repoMeta` 与 `repoInfo` 重复 | 统一结构体 |
| `webhook.go` | 大量 `ErrNotSupported` | 考虑是否真的不支持 |
| `lfs.go` | LFS Lock 完全不支持 | 实现基于文件的锁 |

### 3.3 性能优化

| 优化点 | 建议 |
|--------|------|
| **启动速度** | 并行加载用户和仓库 |
| **内存占用** | 按需加载用户公钥而非全部加载 |
| **文件 I/O** | 批量写入而非逐个文件 |
| **缓存** | 添加 TTL 或 LRU 缓存 |

### 3.4 可维护性

| 优化点 | 建议 |
|--------|------|
| **重复代码** | `serve_filestore.go` 和 `serve_dbstore.go` 代码重复 |
| **错误消息** | 统一错误消息格式 |
| **日志** | 添加更多调试日志 |
| **注释** | 添加更多中文/英文注释 |

---

## 4. 测试覆盖 ✅ 已完成

### 已添加的测试

| 测试类型 | 文件 | 状态 |
|----------|------|------|
| Store 单元测试 | `pkg/store/file/store_test.go` | ✅ |
| User 单元测试 | `pkg/store/file/user_test.go` | ✅ |
| Repo 单元测试 | `pkg/store/file/repo_test.go` | ✅ |
| 集成测试 | `testscript/testdata/` | ✅ 基础覆盖 |

### 测试覆盖范围

- ✅ Store 初始化
- ✅ GetUsersPath 路径处理
- ✅ expandPath 路径扩展
- ✅ validatePath 路径验证
- ✅ checkSymlinks 符号链接检测
- ✅ hashUsername 用户名哈希
- ✅ loadAdminKeys 管理员密钥加载
- ✅ Reload 热重载
- ✅ User CRUD 操作
- ✅ User 公钥管理
- ✅ User 重命名
- ✅ User 查找
- ✅ 路径遍历保护测试
- ✅ Repo 发现
- ✅ Repo CRUD 操作
- ✅ Repo 元数据
- ✅ Repo 私有/描述设置

---

## 5. 文档补充

| 文档 | 状态 |
|------|------|
| 用户文档 | ✅ `FILESTORE_USAGE.md` |
| 配置说明 | ✅ `FILESTORE_USAGE.md` |
| 迁移指南 | ✅ `FILESTORE_USAGE.md` |
| API 文档 | ⚠️ 代码注释 |

---

## 6. 优先级排序

### P0 - 已完成 ✅

1. [x] 路径遍历漏洞
2. [x] 用户名验证
3. [x] 符号链接检查

### P1 - 已完成 ✅

4. [x] Admin 公钥加载所有 keys
5. [x] 文件权限修复
6. [x] 添加单元测试

### P2 - 可以优化

7. [x] 文件锁 (原子写入模式)
8. [x] 代码去重 (保持独立 - build tags 互斥)
9. [x] 热重载支持 (Reload 函数)
10. [x] LFS Lock 支持 (基于 JSON 文件)

### P3 - 未来改进

11. [x] Watch 目录变化 (轮询模式)
12. [ ] 性能优化
13. [x] 完整文档

---

## 7. 构建状态

### 最新构建结果

```
soft-filestore.exe: 42.0 MB ✅
soft-dbstore.exe:   42.0 MB ✅
```

### 测试结果

```
=== 测试通过 ===
pkg/store/file/store_test.go  - 12 tests PASS
pkg/store/file/user_test.go   - 15 tests PASS
pkg/store/file/repo_test.go   - 9 tests PASS
```

---

## 8. 下一步行动

### 已完成

- [x] 修复安全问题（路径验证、用户名验证、符号链接检查）
- [x] 修复文件权限
- [x] 加载所有管理员公钥
- [x] 添加单元测试
- [x] 添加 Reload 函数

### 短期目标

- [ ] 完善文档
- [ ] 添加更多边界测试
- [ ] 实现文件锁

### 长期目标

- [ ] 实现热重载（Watch 目录变化）
- [ ] 性能优化
- [ ] 完整测试覆盖
