# TenantForge

[English](README.md)

面向 Go/Vue 开发者的多租户后台框架，附带完整的企业内部客服工单示例。

做一套服务多家公司的企业软件时，可以复用这里的账号、组织、角色、数据隔离和字段权限，集中开发自己的业务。内置示例将客服登记、主管分配、个人队列、沟通/内部备注、解决说明与关闭重开串成实际流程。

[从管理员到客服：跑通一张工单](docs/support-desk.md)

权限绑定具体操作。租户范围的读取权限与仅本人的修改权限分别计算；查询、详情、统计、导出和任务执行使用同一授权规则。PostgreSQL RLS 提供租户隔离，运行账号不能拥有受保护的数据表或绕过 RLS。

![TenantForge 管理后台](docs/assets/dashboard-zh.png)

[权限矩阵](docs/assets/roles-zh.png) · [字段与数据范围](docs/assets/roles-grant-zh.png) · [深色主题](docs/assets/dashboard-dark.png) · [移动端](docs/assets/mobile-dark.png)

[安装资产扩展](docs/extensions.md)后，可自动生成
[任务表单](docs/assets/extension-task-create-zh.png)与
[中文结果展示](docs/assets/extension-task-zh.png)。

## 启动

需要 Docker Compose 与 Python 3.12+。本地开发另需 Go 1.26.8、Node 24.21 或兼容的更新 LTS 版本。

```sh
python3 scripts/init-env.py
docker compose --profile app up -d --build --wait
```

访问 http://localhost:8090，使用账号 `admin`、初始密码 `Admin@123456` 登录。首次登录修改初始密码后，即可在用户管理中直接新增账号，分配部门与角色。系统自动创建默认租户，租户管理位于后台菜单中。脚本生成私有 `.env`，保存数据库密码与加密密钥；初始管理员可通过环境变量配置，已有账号不会被覆盖。

本地开发：

```sh
make init
make deps
make dev
```

管理页面：http://localhost:5178；开发邮箱：http://localhost:8029。数据库只监听本机 55439 端口。

## 功能

- 租户创建、停用、配额、切换；成员、邀请、部门树与岗位。
- 资源与操作权限矩阵、授权预设及结果预览，角色编辑与复制、有效权限查看；本人、本部门、部门及子部门、指定部门、整个租户五种数据范围。
- 每个操作独立授权，后端逐条返回可操作能力；支持条件、字段读写权限、敏感字段脱敏与显式查看权限。
- 真实待办工作台，中文/英文、浅色/深色主题及移动端布局。
- 默认超级管理员、直接新增用户、Cookie 会话、CSRF、找回密码、会话撤销、TOTP 与恢复码；可配置 OIDC 登录。
- 客服/主管角色模板、个人工单队列、独立分配权限、受控处理记录与内部备注、必填解决说明、关闭/重开及并发冲突；字典、设置、通知、CSV 导入导出、文件、持久任务与审计。
- 公共 Go 扩展接口，可注册业务操作和后台任务；按权限发现任务、自动生成输入表单、展示中文结果，资产审批与盘点示例无需修改核心。
- 短事务上传及配额预留，任务、邮件、清理独立执行；诊断错误码、Prometheus 指标、版本化迁移、备份恢复与容器部署。

## 文档

- [客服工单使用与开发复用](docs/support-desk.md)

- [安装配置](docs/installation.md)
- [权限与安全规则](docs/security.md)
- [API 契约](docs/api-contract.md)
- [OpenAPI 3.1 定义](docs/openapi.json)
- [运维、备份恢复与升级](docs/operations.md)
- [扩展业务模块](docs/extensions.md)
- [依赖选型](docs/dependencies.md)
- [验证记录](docs/verification.md)

```sh
make check
make integration
python3 -m unittest discover -s tests
```

数据库集成测试仅重置独立的 `tenantforge_test`，不会使用业务库。浏览器测试使用独立数据库，具体方法见验证文档。

采用 MIT 许可证，详见 [LICENSE](LICENSE)。
