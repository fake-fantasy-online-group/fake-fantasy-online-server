# Fake Fantasy Online 服务端学习项目

这是一个参考吊想的服务端学习项目，与吊想及其运营方没有官方隶属、授权或背书关系。项目包含 Go 游戏服务端、PostgreSQL 持久化、跨平台桌面控制器和公开资料调研，原创代码采用 MIT 协议。

地图、碰撞、NPC 问候和其他业务配置统一存入 PostgreSQL。仓库不提供客户端程序、私钥或玩家存档；运行需要自行准备兼容客户端和与之配对的服务端握手密钥。

## 获取源码

```sh
git clone https://github.com/fake-fantasy-online-group/fake-fantasy-online-server.git
cd fake-fantasy-online-server
```

## 使用控制器启动

当前仓库提供源码，需要先在自己的操作系统上构建一次控制器。准备 Go 1.22 或更新版本、运行中的 Docker（含 Compose v2），以及 [Fyne 图形界面构建环境](https://docs.fyne.io/started/)。

### macOS

```sh
./controller/build-app.sh
```

生成仓库根目录下的 `服务端控制器.app`，双击打开。也可以用 `make controller-mac` 构建。

### Linux

```sh
./controller/build-linux.sh
```

生成仓库根目录下的 `服务端控制器` 和 `.desktop` 启动项。

### Windows

在仓库根目录打开 PowerShell：

```powershell
./controller/build-windows.ps1
```

生成 `服务端控制器.exe`。如果本机脚本执行策略禁止运行，请按本机策略允许该脚本后再构建。

### 首次启动

准备好下文的握手密钥后，打开控制器并点击「全部启动」。控制器会通过仓库自带的 Docker Compose 启动数据库，在空库上加载初始化数据，再启动地址分发与游戏服务。点击「打开数据库网页」可使用资料查询和 GM 工作台。

**控制器和命令行共用同一套数据库配置**：`server/docker-compose.yml`。需要自定义时，将 `server/.env.example` 复制为 `server/.env` 后修改；已有配置不要覆盖。控制器会读取实际容器信息生成连接串。

| 配置 | 本地开发默认值 |
| --- | --- |
| 数据库容器 | `fantasy-postgres` |
| 持久化数据卷 | `fantasy-postgres-data` |
| PostgreSQL 地址 | `127.0.0.1:5432` |
| 数据库 / 用户 | `fantasy` / `fantasy` |
| 开发密码 | `fantasy_dev` |

停止服务会保留数据卷。以上密码仅是本地开发默认值，实际配置文件、密钥与数据库备份均不应提交到仓库。

## 握手密钥

服务端默认从 `server/internal/crypto/keys/server_private.pem` 读取私钥。已有与客户端配对的密钥时，放到此路径即可，不要覆盖它。

创建一套新的开发密钥（Bash/Zsh）：

```sh
mkdir -p server/internal/crypto/keys
openssl genrsa -out server/internal/crypto/keys/server_private.pem 2048
chmod 600 server/internal/crypto/keys/server_private.pem
openssl rsa -in server/internal/crypto/keys/server_private.pem -pubout -out server/internal/crypto/keys/server_public.pem
```

客户端必须使用与该私钥配对的公钥；新生成的密钥不会使任意客户端自动兼容。本仓库不提供客户端修改、注入或解包工具。

## 内置幻想数据库与 GM

dispatch 内置「幻想数据库」，在浏览器访问 `http://127.0.0.1:8088/fantasy-db/` 即可使用，也可以在控制器中点击「打开数据库网页」。控制器会准备数据库并启动 dispatch；单独打开网页不需要启动游戏服。

- **资料查询**：搜索、筛选物品与装备、NPC、任务、技能，查看属性、地图坐标、奖励、商店及关联资料。
- **GM 工作台**：通过表单生成、预览并复制命令，再粘贴到游戏内聊天执行。网页不会直接执行游戏命令。
- **账号权限**：本机管理面板支持查询账号并设置 GM 等级 `0–4`，修改后须退出账号并重新登录。
- **数据库边界**：业务资料查询来自 PostgreSQL，网页不提供任意 SQL 执行和业务配置编辑。账号管理接口仅允许本机访问；目录查询接口没有登录鉴权，本机使用建议将 dispatch 绑定到 `127.0.0.1:8088`。

详细操作见 [幻想数据库使用说明](docs/幻想数据库.md)。GM 命令、参数、等级要求及常见失败原因见 [GM 命令手册](docs/GM手册.md)。GM 命令以 `/` 开头，要求账号有对应权限且角色已经进入游戏。

## 可选：命令行启动

开发者可以直接启动同一套数据库，不必打开控制器。下面命令使用 Bash/Zsh；配置文件不存在时 Compose 使用表格中的本地默认值：

```sh
docker compose --project-directory server -f server/docker-compose.yml up -d postgres
export DATABASE_URL='postgres://fantasy:fantasy_dev@127.0.0.1:5432/fantasy?sslmode=disable'
make migrate
```

如修改了数据库账号、密码或端口，连接串也应与配置一致。`make migrate` 只用于空数据库；控制器已经初始化过的库无需再执行。

在两个终端分别启动服务：

```sh
# 终端一
(cd server && go run ./cmd/dispatch -addr 127.0.0.1:8088 -dsn "$DATABASE_URL")
```

```sh
# 终端二
(cd server && go run ./cmd/gameserver -addr :19000 -dsn "$DATABASE_URL")
```

也可运行 `make build` 将服务端程序构建到 `bin/`。命令行和控制器共用端口，不要重复启动同一个服务。

## 数据库初始化与升级

地图目录复用 `map_defs`，碰撞网格保存在 `game_map_collision`，NPC 问候保存在 `game_npc_greeting_text` 与 `game_map_npc_hints`。碰撞数据在库内压缩保存，服务端按地图首次使用时读取、解压并缓存，不依赖 `assets/client` 或 `-mapdir`。

如果数据库已经按旧版 `0–45` 分片初始化，请先停止 dispatch 和 gameserver 并备份，再执行这次新增的 `46–49` 分片；不要对非空库重新运行完整初始化：

```sh
pg_dump "$DATABASE_URL" --format=custom --file=before-map-assets.dump
cat server/migrations/000000_init-{46,47,48,49}.sql \
  | psql "$DATABASE_URL" -X --single-transaction -v ON_ERROR_STOP=1
```

上述命令使用 Bash/Zsh 的花括号展开。全新数据库只需执行前面的 `make migrate`，它会按数字顺序一次性加载全部分片。姻缘谷的原始资料没有碰撞网格，数据库保留明确的缺失记录；读取失败或其他配置缺失不会被当作可通行地图。

## 目录

- `server/internal`：领域逻辑、场景、协议、会话与持久化。
- `server/cmd`：游戏服务、地址分发和数据库初始化入口。
- `server/migrations`：按数字顺序执行的初始化 SQL 与业务数据。
- `controller`：桌面控制器及三端构建脚本。
- `docs`：GM 手册和内置幻想数据库说明。
- [references](references/README.md)：公开资料来源与调研摘要。

## 许可与资料权利

本项目原创代码按根目录 `LICENSE` 中的 MIT License 授权。游戏名称、客户端相关数据、图像、协议资料及其他第三方参考材料仍归其权利人所有；本项目的 MIT License 不会重新授权这些材料。使用或再分发前请自行确认相应权利与许可。代码中保留的说明性注释仅用于理解实现。
