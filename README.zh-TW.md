# NodeSweep

**伺服器空間，一目了然。**

輕量級 Linux VPS 磁碟分析與日誌清理面板。同一個執行檔支援單機、中央管理端及 Agent。透過可逐層展開的矩形樹圖尋找空間占用，使用內建或自訂方案預覽後，永久刪除過期的日誌封存檔。

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [繁體中文](README.zh-TW.md)

> **Alpha**：請先在測試 VPS 驗證規則，仍需實際 VPS 部署驗證。不直接清空使用中的日誌、不刪除 Docker 日誌、不處理資料庫與備份。

![磁碟分析介面](docs/images/disk-analysis.png)

截圖來自本機測試環境：一個管理端、兩個 Agent 及測試日誌目錄。介面支援上述五種語言。

## 功能

- 統一查看多台 VPS 的 CPU、記憶體、負載、磁碟容量及 inode。
- 矩形樹圖、目錄清單、逐層進入及返回上層。
- 一個清理方案包含多條規則，自訂路徑、比對模式、排除項目及保留天數。
- Linux、Nginx、寶塔、1Panel 預設規則，以及靜態安裝路徑和日誌路徑辨識；可設定 `panelRoots`。
- 方案 JSON 匯入／匯出、整體驗證、重複處理及交易式匯入。
- 節點獨立憑證、雜湊儲存、可撤銷權限與持久化任務紀錄。

預設規則與自動辨識不會擴大 Agent 的本機清理白名單。排程清理、批次執行、歷史指標曲線及 journal／Docker 原生清理屬於後續功能。

## 快速開始

從 [Releases](https://github.com/While-Shark/NodeSweep/releases) 下載 Linux amd64／arm64 壓縮檔，核對 `SHA256SUMS` 後解壓縮：

```bash
./nodesweep -version
./nodesweep -init
./nodesweep -config config.json
```

預設只監聽 `127.0.0.1:9780`。使用 `config.json` 中的 `adminToken` 登入。權杖不會寫入啟動日誌或瀏覽器本機儲存空間。設定檔權限必須保持 `0600`。

遠端 VPS 可透過 SSH 通道存取：

```bash
ssh -L 9780:127.0.0.1:9780 root@your-server
```

在本機開啟 `http://127.0.0.1:9780`。公開存取或多節點部署請使用現有的 HTTPS 反向代理；NodeSweep 本身不需占用 443。

## 加入其他 VPS

在面板新增節點並下載專用 Agent 設定，將 `hub` 改為管理端的 HTTPS 網址，再把對應架構的執行檔與設定上傳到 VPS。檢查本機 `cleanupRoots` 後執行：

```bash
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json
```

Agent 約每五秒主動向管理端連線，任務執行期間仍持續回報狀態，無需開放入站連接埠。拒絕重新導向與包含憑證、查詢參數或片段的網址；HTTP 僅限開發用的迴路位址。

## 清理之前

預設清理白名單只有 `/var/log`。可加入確認過的寶塔日誌目錄，例如 `/www/wwwlogs`、`/www/server/panel/logs`，然後重新啟動 Agent。不要把網站根目錄、資料庫或整個 `/opt` 加入白名單。

僅處理超過保留天數的壓縮封存檔、數字輪替日誌及通過日期驗證的 Lumberjack 封存檔。候選檔案須為一般檔案、單一硬連結且未偵測到開啟的描述元。使用中的 `.log`、符號連結及跨掛載點項目會跳過。每條規則最多預覽 5,000 個檔案，預覽十分鐘後失效且不可重放。刪除為永久操作，暫存隔離不是備份。

需要 Linux `/proc/self/fd` 及其他程序檔案描述元的可見性。無法完成檢查時會停止清理；檢查後仍可能被其他程序開啟。完整限制見 [安全審計](docs/security.md)（英文）、[部署說明](docs/deployment.md)及[架構](docs/architecture.md)（簡體中文）。

## 多國語言與自動發布

支援英文、日文、韓文、簡體及繁體中文，不必重新整理即可切換。首次依瀏覽器語言選擇，之後只記住語言偏好。使用者自訂名稱、路徑及設定鍵保持原值；未知診斷及任務 JSON 保留原文。

Nightly 在 `master` 的 CI 通過後及每日 UTC 02:17 更新，供測試使用。版本 Release 依 `VERSION` 自動建立，已發布版本不會覆寫；也支援相符的 `v*` 標籤。兩個管道都包含 amd64／arm64 壓縮檔、`SHA256SUMS`、多語言文件及建置資訊。`./nodesweep -version` 可查看版本、提交及建置時間。

## 原始碼建置

使用受支援且已修補的 Go（CI 為 1.27.x）、Node.js 22.18+ 或 24+。執行時只需二進位檔，不需外部資料庫、Redis 或訊息佇列。

```bash
cd web && npm ci && npm run build && cd ..
go test -race ./...
go vet ./...
go build -o nodesweep ./cmd/nodesweep
```

完整驗證與打包指令見 [英文 README](README.md)。參考 [Beszel](https://github.com/henrygd/beszel) 的輕量監控及 [gdu](https://github.com/dundee/gdu) 的磁碟探索體驗，為獨立實作，未包含兩者原始碼。矩形佈局使用 d3-hierarchy。[MIT 授權](LICENSE)。
