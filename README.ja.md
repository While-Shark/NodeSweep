# NodeSweep

**サーバーの容量を可視化し、古いログアーカイブを整理。**

Linux VPS 向けの軽量なセルフホスト管理画面です。同じ実行ファイルを単体、中央ハブ、Agent として利用できます。複数サーバーの状態を確認し、矩形ツリーマップでディレクトリを掘り下げ、削除前に対象をプレビューします。

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [繁體中文](README.zh-TW.md)

> **Alpha:** まずテスト用 VPS で検証してください。実環境での検証は引き続き必要です。稼働中のログ、Docker ログ、データベース、バックアップは削除対象にしません。

![ディスク分析画面](docs/images/disk-analysis.png)

画像はテスト用ハブと 2 台の Agent の画面です。UI は上記 5 言語に対応しています。

## 機能

- CPU、メモリ、負荷、ディスク容量、inode の使用状況を複数 VPS で確認。
- 矩形ツリーマップと一覧によるディレクトリの階層表示。
- 複数ルールを持つ整理プラン：パス、パターン、除外、保持日数。
- Linux/Nginx、宝塔、1Panel のプリセットと静的なログパスの検出。独自の設置場所は `panelRoots` で指定可能。
- プランの JSON インポート／エクスポート、検証、重複処理。
- ノードごとの失効可能な認証情報、ハッシュ保存、永続的なタスク履歴。

自動検出やプリセットは Agent のローカル許可リストを拡張しません。定期整理、一括実行、過去のメトリクス、journal／Docker の整理は今後の機能です。

## 開始方法

[Releases](https://github.com/While-Shark/NodeSweep/releases) から Linux amd64／arm64 用アーカイブを取得し、`SHA256SUMS` を確認して展開します。

```bash
./nodesweep -version
./nodesweep -init
./nodesweep -config config.json
```

初期状態では `127.0.0.1:9780` のみを使用します。`config.json` の `adminToken` でログインしてください。トークンは起動ログやブラウザーのローカルストレージに保存しません。設定ファイルの権限は `0600` に保ちます。

```bash
ssh -L 9780:127.0.0.1:9780 root@your-server
```

手元のブラウザーで `http://127.0.0.1:9780` を開きます。公開アクセスや複数ノードには既存の HTTPS リバースプロキシを利用してください。NodeSweep 自体は 443 番ポートを必要としません。

## 複数ノード

管理画面でノードを追加し、専用の Agent 設定をダウンロードします。`hub` を中央ハブの HTTPS URL に変更し、対象 VPS に実行ファイルと設定を配置します。`cleanupRoots` を確認して開始します。

```bash
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json
```

Agent は約 5 秒ごとに外向き接続し、タスク実行中も状態を報告します。受信ポートは不要です。HTTPS を使用し、リダイレクトと URL 内の認証情報・クエリ・フラグメントは拒否します。HTTP は開発用のループバックに限ります。

## 削除前の確認

初期許可リストは `/var/log` のみです。確認済みのログディレクトリだけを追加し、Agent を再起動してください。サイト全体、データベース、`/opt` 全体を許可しないでください。

保持期限を過ぎた圧縮アーカイブ、番号付きローテーションログ、検証済みの Lumberjack 日時付きアーカイブが対象です。通常ファイル、単一ハードリンク、開かれていないことを確認します。稼働中の `.log`、シンボリックリンク、別マウントは除外します。1 ルールのプレビュー上限は 5,000 件、有効期限は 10 分です。削除は永久的で、一時隔離はバックアップではありません。

Linux の `/proc/self/fd` とプロセスのファイル記述子への可視性が必要です。検査できない場合は削除を停止します。検査後に別プロセスがファイルを開く可能性は残ります。[安全性と監査](docs/security.md)（英語）と [配置手順](docs/deployment.md)（中国語）を参照してください。

## 言語とリリース

UI は英語、日本語、韓国語、簡体字、繁体字に対応し、再読み込みなしで切り替えられます。ユーザー定義の名前、パス、設定キーは変更しません。不明な診断とタスク JSON は原文です。

Nightly は `master` の CI 成功後と毎日 02:17 UTC に更新するテスト用プレリリースです。バージョン Release は `VERSION` に基づき自動作成し、公開済みバージョンを上書きしません。対応する `v*` タグも利用できます。両方に amd64／arm64 アーカイブ、`SHA256SUMS`、多言語文書、ビルド情報を含みます。

## ソースからのビルド

修正済みでサポート対象の Go（CI は 1.27.x）、Node.js 22.18+ または 24+ を使用します。実行時に必要なのはバイナリだけです。

```bash
cd web && npm ci && npm run build && cd ..
go test -race ./...
go vet ./...
go build -o nodesweep ./cmd/nodesweep
```

完全な検証・パッケージ手順は [English README](README.md) を参照してください。[Beszel](https://github.com/henrygd/beszel) と [gdu](https://github.com/dundee/gdu) から着想を得た独立実装です。レイアウトには d3-hierarchy を使用します。[MIT](LICENSE)。
