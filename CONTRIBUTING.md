# Volm への貢献

バグの報告、機能の提案、プルリクエストを歓迎します。この文書では、開発環境の準備から、プルリクエストを送るまでの手順と決まりを説明します。

## 始める前に

- Volm の設計は [docs/DESIGN.md](docs/DESIGN.md) にあります。機能を足すときや動きを変えるときは、先に該当する節を読んでください。
- 大きな変更は、作業を始める前に Issue で相談してください。設計書の「対象にしないもの」に入っている機能は、取り込まないことがあります。
- ほかのアプリのコード、画像、アイコン、文言を持ち込まないでください。Volm の見た目と文言は、このリポジトリで作ったものだけを使います。

## 開発環境

- Windows 11 (x64)。Volm は Windows の API を直接呼ぶので、ほかの OS ではビルドもテストもできません。
- Go 1.27 以降
- Git

MyGo の CLI は、`go.mod` の `tool` に登録してあります。`go tool mygo` で呼び出せるので、別にインストールする必要はありません。

## ビルドと実行

開発中は `go run` で起動できます。

```sh
go run ./cmd/volm              # 常駐プロセス (トレイ、フック、OSD)
go run ./cmd/volm --settings   # 設定画面だけを開く
```

配布用の実行ファイルは `mygo build` で作ります。`mygo.json` の名前、バージョン、アイコンが実行ファイルに入ります。

```sh
go tool mygo build -skip-installer
```

`build/windows-amd64/Volm.exe` ができます。Volm はすでに起動していると 2 つ目が常駐しないので、試す前にトレイのメニューの「終了」で前の Volm を終了してください。

## テスト

```sh
go test ./...
```

音量を実際に変えるテストは、環境変数 `VOLM_INTEGRATION=1` を付けたときだけ動きます。再生デバイスのある PC で実行してください。システムの音量とミュートを一瞬だけ変え、テストの最後に元に戻します。

```sh
VOLM_INTEGRATION=1 go test ./internal/audio/
```

タスクバーの上のホイール、OSD、トレイ、設定画面は自動テストがないので、手で確かめてください。プルリクエストには、確かめた操作と結果を書いてください。

## 生成するファイル

次のファイルはスクリプトで作ります。手で編集せず、元のスクリプトを直してから生成し直してください。

| ファイル | 生成するコマンド | 生成し直すとき |
|---|---|---|
| `assets/icon.svg`、`assets/icon.png`、`assets/volm.ico`、`assets/logo.svg`、`internal/tray/icons/*.png`、`internal/settings/*.png` | `go run ./tools/genicons` | アイコンやロゴの絵を変えたとき。図形は `tools/genicons/art.go` で定義している |
| `THIRD_PARTY_NOTICES.md` | `go run ./tools/gennotices` | 依存するモジュールを足したり、更新したりしたとき |

設定画面のアイコン (`internal/settings/icons/*.svg`) は、[Material Symbols](https://github.com/google/material-design-icons) の Rounded スタイルの 24px の SVG をそのまま置いています。足すときは同じスタイルのものを選んでください。

## コードの決まり

- `gofmt` で整形し、`go vet ./...` を通してください。COM のオブジェクトや低レベルフックで Windows から渡されたポインターを扱う箇所では、`possible misuse of unsafe.Pointer` の警告が出ます。この警告は想定どおりです。
- ソースファイルの先頭には、次の 2 行を入れてください。

  ```go
  // Copyright (C) 2026 otoneko.
  // SPDX-License-Identifier: GPL-3.0-only
  ```

- コード、コメント、識別子、テスト名は英語で書きます。画面に出る文言と、`docs/` の文書は日本語で書きます。
- 名前で意味が伝わるようにし、コメントは名前だけではわからないことにだけ書きます。外から使う型と関数には、godoc の形式で使い方を書きます。
- コメントと文書は、文の途中で改行しません。改行するのは、段落や箇条書きの区切りだけにします。
- 低レベルフックのコールバックには、時間のかかる処理を書かないでください。Windows は、一定の時間内に戻らないフックを予告なしに外します。COM の呼び出しなどは、フックからチャネルでイベントを受け取る goroutine で行います。

## コミットとプルリクエスト

- コミットメッセージは英語で、`Add settings window` のように命令形で書きます。
- プルリクエストには、変えた理由、変えた内容、確かめた方法を書いてください。画面の見た目を変えたときは、変更前と変更後のスクリーンショットを付けてください。
- 送っていただいた変更は、Volm と同じ GPL-3.0-only で公開します。
