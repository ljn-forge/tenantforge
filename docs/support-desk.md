# 用 TenantForge 跑通企业客服工单

TenantForge 面向开发企业软件的 Go/Vue 开发者。账号、组织、角色、数据隔离、字段权限、文件、任务和审计由框架提供；工单示例把这些能力连成一个可以操作的业务流程。

比如，你维护一个客服 SaaS，甲公司和乙公司都在使用它。平台管理员建立两个租户；每家公司管理自己的客服部门和员工。客服只处理本人负责的工单，主管处理所属部门工单，两家公司数据互相隔离。

本示例是员工使用的客服后台。客服代客户登记问题并记录沟通内容；“客户沟通记录”表示记录一次沟通，不会发邮件给客户。它没有客户自助登录门户。现有文件和通知模块不代表工单已经接入附件或自动通知。

## 管理员准备

按 [安装说明](installation.md) 启动，在 http://localhost:8090 用初始管理员登录并修改初始密码。已有管理员继续使用原密码，升级不会重置。

1. 在“部门与岗位”创建“客户服务部”。
2. 在“角色与授权”点击“新建客服处理员角色”，查看预填权限，保存为“客服”。
3. 点击“新建客服主管角色”，查看权限，保存为“客服主管”。
4. 在“用户管理”添加两个客服和一个主管，将他们的部门设置为“客户服务部”，分别分配对应角色。初始密码由管理员设置，员工首次登录必须修改。
5. 使用独立浏览器窗口或退出后登录这些员工账号。不要共用管理员会话验证员工权限。

模板只填入新建角色的草稿，保存前可以修改；不会自动覆盖已有角色。预览会明确列出客户邮箱和内部备注的敏感字段访问权，请按团队实际职责调整。

| 角色 | 工单范围 | 操作 | 敏感内容 |
| --- | --- | --- | --- |
| 客服 | 本人负责 | 登记、查看、编辑、回复、状态流转 | 模板明确授权客户邮箱及内部备注 |
| 客服主管 | 所属部门 | 上述操作，加分配负责人 | 模板明确授权客户邮箱及内部备注 |
| 只读观察者（管理员自行配置） | 按角色设置 | 查看 | 只授予普通字段时，不返回邮箱和内部备注 |

客服模板没有分配、删除、导入、导出或账号管理权限。主管模板补充所属部门的成员及部门读取用于选择负责人，也没有用户管理写权限。

## 处理一张工单

1. 客服进入“工单”，新建记录，例如“客户无法下载发票”。填写问题描述、客户名称、客户邮箱和优先级。新单为“待处理”，负责人默认是登记客服，部门来自该员工的当前部门。
2. 主管进入部门工单列表，打开详情，点击“分配工单”，选择另一位客服。保存后，新负责人在“我的待办”队列中看到它；原负责人失去本人范围访问。
3. 新负责人点击“开始处理”。在处理记录中选择“客户沟通记录”，记录排查或沟通结果；选择“内部备注”，记录仅获授权员工可查看的信息。每次保存都使用服务端版本，过期编辑会提示刷新，不会悄悄覆盖同事的工作。
4. 点击“标记解决”，填写“解决说明”，例如“补齐税务信息后重新生成发票，客户已确认下载成功”。缺少说明不能解决。
5. 获授权的员工点击“关闭工单”。关闭后不能继续追加回复；需要处理新情况时先“重新打开”。已解决的单也可以返回处理中，每次重新解决都需要新的说明。
6. 查看处理记录，可以追踪登记、分配、状态变化和沟通。记录按当前权限读取；改变负责人、部门、角色或租户后，旧处理记录不会成为绕过权限的入口。

![实际验收的客服处理记录](assets/support-ticket-zh.png)

“我的待办”是本人负责且处于待处理/处理中的工单。“进行中”“已解决”“已关闭”“全部”按当前可见数据筛选，数量由服务端分页查询计算。没有读取状态字段的角色不能通过队列推断隐藏状态。已有导出仍采用独立的列与权限契约，不声称自动导出当前队列。

## 多公司使用

平台管理员在“平台租户”创建公司并指定已有用户邮箱作为租户所有者；该租户的 Owner 再设置部门和员工。平台超级管理员身份不会自动给予另一租户业务数据读取权，仍需有效成员关系和租户角色。

客户端选择当前组织，每次请求携带租户上下文；服务端验证成员、操作、记录范围和字段。PostgreSQL 强制 RLS 作为第二道隔离。切换组织会清空上一组织的处理草稿和列表。

## 开发者怎么复用

| 业务需求 | 平台已有能力 | 示例专属代码 |
| --- | --- | --- |
| 一套系统服务多家公司 | 租户、成员、运行账户 RLS、配额 | 公司业务规则 |
| 不同员工能做不同事 | 角色、操作授权、服务器逐记录能力 | 客服/主管角色模板 |
| 员工只看本人或部门数据 | 统一查询与记录范围谓词 | 个人工单队列 |
| 联系方式和内部备注受控 | 字段读写、脱敏、显式查看授权 | 回复/备注与父工单的字段映射 |
| 处理业务状态 | 版本冲突、事务、审计 | 工单状态图、解决说明约束、处理记录 |
| 增加另一类业务 | 公共 Go 模块/操作/任务注册、生成页面 | [资产审批与盘点扩展](extensions.md) |

工单逻辑集中在 `internal/platform/ticket_workflow.go`、`internal/platform/support_workflow.go` 和前端对应 support 文件中，通用资源和权限服务供它复用。增加设备管理、客户资料或其他资源时，优先使用公共扩展接口，不复制工单服务或改写核心授权。

## 升级与验收

011 迁移新增受租户隔离的处理记录表和分页索引；保留既有工单、账号与角色。旧工单不会生成虚构历史，从升级后的操作开始积累记录。受保护的 Owner 增补新操作并留下激活审计，自定义角色需要管理员显式添加 `reply`。解决状态新增非空解决说明要求，API 调用方需要同步提交。

部署前备份，并在恢复库验收迁移。具体步骤见 [运维说明](operations.md)。

```sh
make check
make integration
python3 scripts/browser-check.py --built
make recovery
```

浏览器用独立的 `tenantforge_browser` 完成管理员建角色/员工、主管分配、客服处理、敏感备注隔离和跨租户验收；不会重置业务数据库。查看 [验证记录](verification.md) 获取实际测试结果。

## English overview

TenantForge is a Go/Vue foundation for developers building multi-tenant enterprise software. This internal support reference flow covers employee-entered customer issues, department-scoped supervisor assignment, agent queues, plain-text replies and private notes, mandatory resolution summaries, closure and reopening. Replies record communication; they do not deliver customer email or enable a customer portal.

An administrator creates a support department, reviews the New support agent role and New support supervisor role drafts, saves them, and adds employees with the appropriate department and role. Each employee changes their initial password. Agents handle their own tickets; supervisors handle their department. Templates explicitly disclose access to customer email and internal notes. Read-only roles can omit these fields, and the activity API enforces the same parent-record and field boundary.

A ticket starts open. The supervisor assigns an active department member; the agent opens My active tickets, starts work, records replies/notes, supplies a resolution summary, and closes it. Reopening preserves history; resolving again requires a fresh summary. Expected versions protect concurrent changes. Tenant switching and permission changes invalidate drafts and late responses.

Reuse the public extension registry for new modules and the core tenant/authentication/authorization/file/job services. Ticket-specific code is an example of composing those services, not a replacement authorization system. Migration 011 preserves existing records and begins new activity history without inventing past events. Custom roles must explicitly gain reply authority; protected Owner activation is audited. API callers moving into resolved must supply a nonblank resolution summary.
