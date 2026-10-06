# 游泳赛事泳道电子计时复核系统

面向游泳比赛现场的**电子计时漏记 / 成绩争议复核**系统：电子计时设备按比赛与泳道上报分段与触壁时间；
裁判提交手记与改判依据；**总裁判**裁定采用电子成绩、手动成绩、重赛结果或撤回；录入员只能补充材料，不能改变名次。
成绩页同时呈现原始读数与裁定结果；公示榜单带**更正时间与原因**，并区分**并列 / 重赛 / 撤回**。
发布新榜单只新增版本，**旧榜单与原始读数永久保留、不可覆盖**。

## 技术栈

- 后端：Go 1.23 + Gin + GORM + SQLite（`glebarez/sqlite` 纯 Go 驱动，免 CGO）
- 前端：Vue 3 + Vite（无额外 UI 框架，原生组件）
- 电子计时：本地模拟接口 `POST /api/device/readings`（页面内置「设备模拟台」）

## 一条命令启动

```bash
./start.sh          # 开发模式：前端 http://localhost:5173 ，后端 http://localhost:8080
./start.sh --prod   # 生产模式：构建前端，Go 单端口托管 http://localhost:8080
```

演示账号（密码均为 `123456`）：

| 账号 | 岗位 | 能做什么 |
|---|---|---|
| `chief` | 总裁判 | 赛事编排、**裁定（电子/手动/重赛/撤回）**、**发布榜单**、设备上报 |
| `judge` | 裁判 | 立案复核、提交手记、向案件补充材料 |
| `clerk` | 录入员 | 仅可补充手记/材料，**不能立案、裁定、发布** |
| `device` | 电子计时设备 | 仅可上报分段/触壁读数（含漏记），漏记自动立案 |

## 内置演示数据

「2026年城市游泳冠军赛 · 男子100米自由泳决赛」：

- **第4道触壁漏记**（设备报文 `PAD-TOUCH-TIMEOUT`），系统已自动立案，裁判手记 52.94 在卷；
- **第2、6道成绩同为 51.88**，用于演示并列排序（名次 1、1、3 顺延，显示 `1=`）；
- 女子50米蛙泳数据完整，可演示正常发布；另有空项目供手动编排。

建议复核流程：以 `device` 看设备模拟台 → `judge` 补手记/立案 → `chief` 裁定并发布 → 切换「公示榜单 / 历史版本」核对。

## 业务规则

1. **原始读数只追加**：同一泳道多次触壁读数（漏记、补传）全部保留，榜单取最新一条有效 `pad` 读数。
2. **漏记自动立案**：设备上报 `missed=true` 时自动创建 `open` 复核案件。
3. **裁定四选一**（仅总裁判）：
   - `electronic`：采用最新有效电子触壁读数（无有效读数会被拒绝）；
   - `manual`：采用手动成绩（请求带值或取最新手记）；
   - `swimoff`：必须录入重赛成绩，榜单标注「重赛」；
   - `withdraw`：成绩撤回，榜单标注「撤回」且不参与排名。
4. **并列排序**：同成绩同名次，显示 `1=` 并标「并列」，后续名次顺延（1,1,3…）。
5. **榜单版本化**：发布生成新版本并快照全部条目；上一版 `is_current` 置否但不删除；
   新版本逐行比对标注「本版更正」，并自动生成人读更正摘要（谁、第几道、X→Y）。
6. **待裁定拦截发布**：仍有泳道无终局结果时，后端拒绝发布（409）。
7. **历史可查**：`GET /api/events/:id/boards/:version` 可取任意旧版快照；原始读数在项目详情中始终可查。

## 主要接口

| 方法 & 路径 | 允许岗位 | 说明 |
|---|---|---|
| `POST /api/auth/login` | 公开 | 登录换令牌 |
| `POST /api/device/readings` | device/chief | 上报分段/触壁读数（missed 漏记） |
| `POST /api/lanes/:id/manual` | judge/clerk/chief | 提交手记（证据，不改名次） |
| `POST /api/events/:id/cases` | judge/chief | 立案复核 |
| `POST /api/cases/:id/append` | judge/clerk/chief | 补充材料 |
| `POST /api/cases/:id/decision` | **chief** | 电子/手动/重赛/撤回 裁定 |
| `POST /api/cases/:id/reopen` | **chief** | 重新开启已决定/撤回案件 |
| `POST /api/events/:id/publish` | **chief** | 发布新版榜单 |
| `GET /api/events/:id` | 登录用户 | 项目原始读数 + 手记 + 案件 + 当前榜单 |
| `GET /api/events/:id/live` | 登录用户 | 裁判席实时未发布排名 |
| `GET /api/events/:id/boards` / `…/boards/:v` | 登录用户 | 版本列表 / 任意历史版本快照 |

## 测试

```bash
cd backend
go test ./...
```

覆盖：漏记复核全链路、并列名次顺延、重赛/撤回、更正版本化与历史不可覆盖、各岗位权限矩阵、裁定依据校验。

## 目录结构

```
start.sh                 一条命令启动（dev / --prod）
backend/  Go + Gin + GORM + SQLite
  models.go              数据模型（赛事/泳道/读数/手记/案件/榜单版本）
  service.go             成绩汇总、并列排序、版本发布（核心规则）
  auth.go                HMAC 令牌登录与岗位中间件
  main.go router.go       HTTP 接口
  seed.go                 演示数据
  main_test.go           集成测试
frontend/ Vue 3 + Vite
  src/components/
    EventView.vue          公示榜单 / 原始数据 / 实时排名 / 历史版本
    LaneCard.vue          单泳道复核（读数、手记、立案、裁定）
    DevicePanel.vue       本地电子计时设备模拟台
    PublishDialog.vue     总裁判发布（含待裁定拦截提示）
```

数据文件默认在 `backend/data/swim.db`（删除即可重置演示数据）。
