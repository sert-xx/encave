# encave

*This document is also available in [English](README.md).*

> チューニングしたコーディングエージェントの設定を、**隔離された**・**再現可能な**・
> 自己完結したエージェントホームとして配布・実行する。GitHub 経由で共有し、
> 受け取る側自身の環境には一切手を触れない。

**encave** という名前は *en-* + *cave* /  *enclave*（飛び地）に由来します。各エージェントは
それ専用の隔離された箱に封じ込められ（*encaved*）、その中で実行されます。正式なコマンドは
`encave` で、公式な短縮エイリアスは意図的に用意していません。

対応ターゲットは **Codex CLI** と **Claude Code** です。中核となる考え方 — 隔離された
エージェントホーム + git clone とタグによる配布、そして認証情報をリポジトリの外に保つこと
— は対象非依存で、アダプタインターフェースの背後にあり、他のエージェント CLI も後から
追加できます。認証情報の渡し方はターゲットごとに異なります（Codex は encave が注入、
Claude Code はターゲット自身のログイン）— [ターゲット](#ターゲット)を参照。

## なぜ

コーディングエージェントを本格的に使う人は、ホームディレクトリ全体
（`~/.codex` / `CODEX_HOME`）をチューニングします。モデル設定だけでなく、
*オーケストレーション*層 — サブエージェントの役割定義、プロファイル、レビュー用スキル、
MCP サーバー — まで含めて作り込みます。これを設定ファイルのコピーで同僚に共有すると
相手のセットアップを壊しますし、プラグインではオーケストレーション層を運べず、
隔離されないまま受け取り側の設定にマージされてしまいます。

encave は **エージェントホーム全体を 1 つの再現可能な単位として** 配布し、隔離して
インストールし、受け取る側の個人ホームを一切変更せずに実行します。認証情報は共有
リポジトリの外に保たれます。

## 仕組み

- **再現可能**: エージェントは `git clone` + タグのチェックアウトで配布されます。タグは
  提供者の設定をバイト単位で再現します。
- **隔離**: インストールされた各エージェントは `<root>/<owner>/<repo>` 配下に置かれ、
  それ専用のホームディレクトリ（Codex なら `CODEX_HOME`、Claude Code なら
  `CLAUDE_CONFIG_DIR`）で起動されます。あなたの `~/.codex` / `~/.claude` には触れません。
- **秘密情報は git に入れない**: コミットされるのは秘密でない設定だけです。Codex ターゲットでは
  実際の認証情報は **OS キーリング** に保存され、起動された子プロセスの環境にのみ注入されます
  （リポジトリ・ログ・標準出力には書き出されません）。Claude Code ターゲットは Claude Code 自身の
  ログインを使い、encave は何も注入しません（[ターゲット](#ターゲット)を参照）。

Codex ターゲットでは、保存された認証情報を **表示するコマンドは意図的に用意していません**。
それは恒常的な認証情報ダンプの窓口になってしまうからです。認証情報がキーリングから出るのは、
起動された 1 つの子プロセスの環境としてのみで、そのプロセスが生きている間だけです。

## インストール

### 前提条件

- **Go 1.25.11+** — 1.25.10+ には encave のコードパスが依存する標準ライブラリの
  セキュリティ修正が含まれています。`GOTOOLCHAIN=auto`（Go のデフォルト）なら、
  `go install` が適切なツールチェインを自動取得します。
- `PATH` 上の **git**（`install` と `publish` で使用）。
- 実行したい **対象エージェント CLI** — **Codex CLI** または **Claude Code**。
- **Codex** ターゲットでは、動作する **OS キーリング**（encave が認証情報を注入）が必要:
  macOS Keychain と Windows Credential Manager はそのまま使えます。**Linux** では
  `gnome-keyring` や KeePassXC の Secret Service のような、動作中の **Secret Service**
  が必要です。**Claude Code** ターゲットはキーリングを使いません — [ターゲット](#ターゲット)
  を参照。

### `go install` でインストール（推奨）

```sh
# 特定のリリースをインストール — Releases ページの最新タグを使ってください:
# https://github.com/sert-xx/encave/releases
go install github.com/sert-xx/encave@v0.8.0
```

> `@latest` より最新のリリースタグを使ってください。モジュールプロキシの `@latest` は
> 最新タグに追いついていないことがよくあります。

`go install` は `encave` バイナリを `$(go env GOBIN)`（なければ
`$(go env GOPATH)/bin`）に配置します。そのディレクトリが `PATH` に入っていることを
確認してください。

```sh
export PATH="$(go env GOPATH)/bin:$PATH"   # シェルのプロファイルに追記してください
```

### ソースからビルド

```sh
git clone https://github.com/sert-xx/encave && cd encave
make build      # ./encave を生成
make install    # $GOBIN へ go install
make test       # テストスイートを実行
```

### 確認

```sh
encave version
```

### 最新の状態を保つ

encave 自身とエージェントの両方を、控えめなプロンプトで最新に保ちます（裏で勝手に
インストールすることはありません）:

- **encave 自身:** コマンド実行時に新しい encave リリースがあれば、その特定バージョンを
  `go install`（`@latest` ではなく解決済みのタグ）でインストールするか確認します。
  インストールされた新しいバイナリは次回の実行から有効になります。
- **エージェント:** [共有エージェントを使う](#共有エージェントを使う)を参照 —
  `encave run` 時にエージェントの origin に新しいリリースタグがあれば更新を確認します。

チェックはベストエフォートかつ静かに動作します: 対象ごとに 1 日 1 回まで、端末に接続
されている場合のみ実行され、自己チェックにはリリース版（`make build` の開発用バイナリ
ではない）が必要です。処理をブロックせず、ネットワークやプロキシのエラーは黙って
スキップします。encave のチェックは `GOPROXY` を尊重します。すべて無効にするには
`ENCAVE_NO_UPDATE_CHECK=1` を設定してください。

## クイックスタート（共有エージェントを使う）

```sh
# 1. (Codex エージェント) 認証情報を一度だけ保存する（例: プロキシ用の PAT）。期限切れ時に
#    再実行する。Claude Code エージェントは不要 — Claude Code 自身のログインを使う。
encave auth set --global

# 2. エージェントをリリース版に固定してインストールする。
encave install github.com/dai/review-agent --tag v1.0.0

# 3. 起動する — 専用の隔離ホームで。
encave dai/review-agent
#    （または `encave run` を実行して一覧から選ぶ）
```

あなたの個人の `~/.codex` / `~/.claude` には触れません。`install` はリポジトリが encave 管理の
エージェントであること（`encave new` が書き出す有効な `.encave.toml` を持つこと）を
検証し、そうでなければ拒否します。信頼できるリポジトリに対しては `--no-verify` で
上書きできます。認証情報の渡し方はターゲットごとに異なります — [ターゲット](#ターゲット)を参照。

## コマンド

```
encave new <owner>/<repo>      ドラフトエージェントを生成（秘密情報を除外、README テンプレートを生成）
encave publish [<owner>/<repo>]  スキャン（fail-closed）→ コミット → タグ付け →（リモートがあれば）プッシュ
encave install <github-url>    エージェントを clone + チェックアウト（encave 管理であることを検証）
encave update [<owner>/<repo>] エージェントをタグへ更新（デフォルト: 最新）。--all で全エージェント
encave run [<owner>/<repo>]    エージェントを起動（"default" = 自分のホーム、省略で選択）
encave auth set|status|clear   OS キーリングの認証情報を管理（値は表示しない）
encave list                    インストール済みエージェントとローカルドラフトを一覧表示
encave remove [<owner>/<repo>] インストール済みエージェントのディレクトリを削除（エイリアス: rm）
encave version | help
```

`run` は **デフォルトコマンド** です。`encave dai/review-agent` は
`encave run dai/review-agent` と同じです。`--` 以降はすべて対象 CLI にそのまま
転送されます。

## 使い方

### 共有エージェントを使う

インストール済み（とローカルドラフト）を確認:

```sh
encave list
```

参照を指定して起動するか、省略して対話メニューから選ぶ（↑/↓ で移動、Enter で決定、
`q` でキャンセル）:

```sh
encave dai/review-agent            # run はデフォルトコマンド
encave run                         # インストール済みエージェント（+ 自分のホーム）の矢印キーメニュー
```

`--` 以降はすべて対象 CLI に転送されます。起動せずに実際のコマンドを確認
（認証情報は伏せられます）:

```sh
encave dai/review-agent --dry-run -- exec "review this diff"
```

`update` でエージェントを最新に保つ（エージェントの origin を fetch してタグを
チェックアウト）:

```sh
encave update dai/review-agent              # 最新のリリースタグ
encave update dai/review-agent --tag v1.2.0 # 特定のタグ
encave update --all                         # 全インストール済みエージェントを最新へ
```

手動で覚えておく必要はあまりありません: `encave run` でエージェントを起動する際、
origin に新しいリリースタグがあれば encave がまず更新するか確認します（承認すると
`update` を実行し、拒否すると手元のバージョンで起動します）。

不要になったエージェントを削除（ディレクトリを削除。まず確認するか、`--force` を渡す）:

```sh
encave remove dai/review-agent   # エイリアス: encave rm
```

### 自分の（encave 管理でない）ホームを起動する

encave を唯一の入口にできるよう、`default` は対象の自分のデフォルトホーム
（例: `~/.codex`）を直接起動します。隔離なし・認証情報注入なし・`CODEX_HOME` は
そのままです。対象 CLI を自分で実行するのと完全に同じ挙動で、`encave run` のピッカーにも
選択肢として現れます:

```sh
encave default                     # `encave run default` と同じ
encave run default -- exec "quick one-off in my own setup"
```

### 認証情報の管理

認証情報は OS キーリングに保存され、まずエージェント固有のエントリ、次にグローバルの
順で解決され、起動された（隔離された）プロセスにのみ注入されます:

```sh
encave auth set --agent dai/review-agent   # 1 エージェントにスコープ
encave auth set --global                   # または全エージェントで共有
encave auth status --global                # "set" / "not set" のみ表示 — 値は出さない
encave auth clear --global
```

### 個人設定（`rules`）

ホームのサブディレクトリの一部は、エージェント作者ではなく *あなた自身* の設定を
保持します。Codex では `rules`（あなたがローカルで承認したコマンド）です。encave は
これらを決してパッケージしません。`new` は除外し、`publish` は gitignore します。
代わりに、エージェントの `rules` をあなたのベースホーム `~/.codex/rules` へ symlink
します。これにより、あなたの個人的な承認がすべてのエージェントに適用され、新しい承認も
1 か所に蓄積されます。（既に実体の `rules/` ディレクトリを同梱しているエージェントは
そのまま残します。）

symlink は `new`・`install`・`run` によって（再）作成されます。だからエージェントを
生成・インストールした直後から有効で、編集中も見えています（あなたの rules を誤って
コピーすることもありません）。これは **コミットされません**。symlink はマシン間で
可搬にできない（OS はリンク先の `~` や `$HOME` を展開せず、絶対パスでは作者のホームを
指してしまう）ため、encave がマシンごとにあなたの実際のホームパスで作り直します。

### エージェントを作って共有する

エージェントの名前は **その GitHub アイデンティティ**（`<owner>/<repo>`）そのものです。
だから `new` と `publish` はその形式で受け取ります。`new` は `<root>/<owner>/<repo>`
へ直接スキャフォールドします — `install` が使うのと同じ場所です。だから別途「ドラフト」
領域はなく、公開する前に自分のエージェントを実行して反復できます。

```sh
encave new dai/review-agent        # ターゲットを尋ねてから、そのホームをコピー（秘密情報/状態を除外）
encave dai/review-agent            # 公開前にローカルで試す
# ... agents/, skills/, base 設定を調整 ...
```

`--target` を省略すると、端末ではターゲット CLI（Codex か Claude Code）を対話的に選びます。
`--target claude-code`（または `--target codex`）を渡せばプロンプトをスキップできます（スクリプト用）。
端末でない場合は既定ターゲット（`codex`）にフォールバックします。コピー元ホームは選んだ
ターゲットに従います（`~/.codex` / `~/.claude`）。`--from` で上書き可能です。

`encave new` はエージェント内に `README.md` テンプレートも生成します（`--no-readme` を
渡さない限り）。ベースホームからコピーされた README は置き換えられます — あの汎用的な
`~/.codex` の README が特定のエージェントに合うことはまずないからです。生成される README は
利用者向けに install/auth/run の流れを記し、エージェントの `<owner>/<repo>` と、設定から
検出した認証用環境変数で埋められます。

続いて `new` は `git init` を実行し、**README だけ** を含む最初のコミットを作ります
（git が無ければスキップ）。エージェントの残りは後で `publish` が、秘密情報スキャンの後に
コミットします — スキャンされていないものがコミットに入らないようにするためです。

```sh
# 先に GitHub で空のリポジトリを作り、リモートを指定して publish:
encave publish dai/review-agent --tag v1.0.0 --remote git@github.com:dai/review-agent.git
```

ターミナル上では `encave publish` を実行するだけで、足りないものを対話的に尋ねます —
どのエージェントか（一覧から選択）、リリースタグ、リモート（`git@github.com:<owner>/<repo>.git`
をデフォルト候補に）です。ターミナルでない場合はこれらをフラグで渡す必要があります。

`encave publish` は fail-closed の秘密情報スキャンを実行し、コミットしてタグを付けます。
その後:

- **リモートがある場合**（`--remote`、または既存の `origin`）: `Push to <url> now? [y/N]`
  と尋ね、確認するとブランチとタグをプッシュします。`-y`/`--yes` でプロンプトを省略、
  `--no-push` でタグ付けまでで停止します。非対話実行では `--yes` がない限りプッシュしません。
- **リモートがない場合**: プッシュせずに停止し、設定方法を案内します（コミットとタグは
  既にローカルで作成済みです）。

タグがプッシュされた後、[GitHub CLI](https://cli.github.com/)（`gh`）がインストールされていて
リポジトリにアクセスできる場合、encave はそのタグの **GitHub リリース** 作成を提案します
（確認あり。`--yes` なら確認せず作成）。`gh` が無い、またはリモートが到達可能な GitHub
リポジトリでない場合、この手順は静かにスキップされます。

## ターゲット

*ターゲット* は、エージェントホームが対象とするエージェント CLI です。
`encave new <owner>/<repo> --target <name>` で選び、`.encave.toml` に記録されるので
`run` は自動で正しい挙動を選択します。両者の主な違いは **認証情報がどこから来るか** です:

| | **Codex**（`--target codex`、既定） | **Claude Code**（`--target claude-code`） |
|---|---|---|
| ホーム変数 | `CODEX_HOME` | `CLAUDE_CONFIG_DIR` |
| 設定ファイル | `config.toml`（TOML） | `settings.json`（JSON） |
| 梱包する base | `config_base.toml` | `settings_base.json` |
| 認証情報 | **encave 管理**: OS キーリングに保存し起動時に注入 | **encave 非管理** — Claude Code 自身のログインを使う（下記） |

**Codex** は保存ログインが `CODEX_HOME` に紐づくため、ホームを隔離すると失われます。
そのため `encave auth set` で一度トークンを保存し、起動時に encave が注入します。

**Claude Code** は macOS では認証情報を config dir の外（グローバルな **Keychain**）に
保存するので、隔離した `CLAUDE_CONFIG_DIR` でも通常の `claude /login` のままログイン状態を
保てます（encave は何も注入しません）。**Linux/Windows** では認証情報ファイルが config dir
の *内側* にあるため、隔離ホームはログアウト状態で始まります。中で一度認証するだけです
（`claude` を起動して `/login`、または `claude setup-token` で得た `CLAUDE_CODE_OAUTH_TOKEN`
を設定）。いずれの場合も encave は Claude の認証情報を保存も表示もしません。接続先
（`ANTHROPIC_BASE_URL`）は環境固有で、梱包されたエージェントではなくあなた自身の環境から来ます。

## セキュリティモデル

1. **秘密情報はリポジトリに入らない** — キーリング + `.gitignore` + fail-closed の publish
   スキャン（既知の認証情報ファイル名、トークン形状の文字列、高エントロピーの値）。
   検出されると `encave publish` は中止します。
2. **恒常的なダンプ窓口を作らない** — キーリングの値を表示するコマンドは存在せず、
   注入は起動された子プロセスの環境に限定されます。
3. **認証情報は一時的** — トークンが環境に存在するのは、起動されたプロセスが生きている
   間だけです。
4. **残存リスクは認識している** — 同一ユーザーのコードは実行中の子プロセスの環境を
   読めます。これはローカルの子プロセスに秘密を渡す際の最小限の下限であり、恒常的な
   窓口よりはるかに小さいリスクです。
5. **エンタープライズのポリシーが優先** — MDM / 管理ポリシーは起動時設定を上書きできます。
   encave は自身の上書きが常に有効になると仮定しません。
6. **Claude Code の注意** — Claude エージェントホームで encave が生成する実効 `settings.json`
   は、あなたの `~/.claude/settings.json`（`env` ブロック含む）をマージします。そこに認証情報の
   *値* を書いていると、そのファイルに書き出されます（gitignore 済み・パーミッション 0600・
   `publish` でコミットされない）。認証情報は `settings.json` の `env` ではなく、シェルや
   Keychain に置くことを推奨します。

## レイアウト

```
<root>/                          # ~/.encave（ENCAVE_ROOT で上書き）
└── <owner>/<repo>/              # 1 エージェント = 1 つの隔離されたエージェントホーム
    │                            #   （`new` で作成、または `install` で取得）
    ├── config_base.toml         # エージェント所有の設定（ホワイトリストのキー）— コミットされる
    ├── config.toml              # 起動時に生成（base ⊕ あなたの ~/.codex）— gitignore される
    ├── .encave.toml             # 秘密でないエージェントメタデータ（対象アダプタ）
    ├── agents/ prompts/ skills/ # オーケストレーション・プロンプト・スキル — 同梱、コミットされる
    ├── AGENTS.md                # 作者の指示 — コミットされる
    ├── rules -> ~/.codex/rules  # 個人設定: 起動時に symlink、パッケージされない
    └── (除外: auth.json, history.jsonl, sessions/, *.sqlite の状態/ログ DB,
         logs, caches, version.json — Codex が機械生成する状態)
```

パッケージされるのは作者がチューニングした設定だけです。Codex が機械生成する状態 —
認証情報、履歴、セッションのトランスクリプト、`state_*.sqlite` / `logs_*.sqlite`
データベース（とその WAL/SHM サイドカー）、ログ、キャッシュ、`version.json` — は
`new` が除外し、`publish` が gitignore します。

### config: エージェント所有 vs. 環境

Codex の `config.toml` は、エージェント作者の設定（モデル、プロバイダ、
サンドボックス/権限、agents など）と、実行者自身の環境固有の状態を混在させます。
最も重要なのは `[projects]` の信頼レベルで、これはあなたがプロジェクトを承認するたびに
Codex が自動追記する絶対ローカルパスを記録します。ほかに UI、通知、テレメトリ、
ローカルパスもあります。これらを正しい側に置くため:

- `new` は **エージェント所有のトップレベルキーのホワイトリスト** だけを含む
  **`config_base.toml`** を書き出します。それ以外はすべてユーザーのホームに委ねます。
- 起動時、`run` は `config_base.toml` をユーザー自身の `~/.codex/config.toml` に重ねて、
  Codex が読む **生成された `config.toml`** にマージします。エージェントのキーが勝ち、
  プロジェクトの信頼・UI・その他の個人設定はユーザー由来になります。`config.toml` は
  gitignore されているため、Codex が実行時に新しい信頼を追記しても差分には現れません。

これにより、各実行者の既存のプロジェクト信頼の判断が自動的に適用され、作者のローカル
パスや信頼がエージェントに同梱されることはありません。

**MCP サーバーとモデルプロバイダはパッケージされません。** `mcp_servers` と
`model_provider`/`model_providers`（およびローカルパスを含む `sandbox_workspace_write`）は
意図的にホワイトリストから外しています。他人の MCP 設定やプロバイダ配線（内部の
base URL、認証用環境変数）を流用するのは危険で環境固有だからです。これらは起動時に
ユーザー自身の `~/.codex/config.toml` から取られ、`new` は作者の MCP サーバーとモデル
プロバイダをセットアップ要件として生成 README に列挙します。

**認証の配線は encave が所有します。** 実効的な `config.toml` を生成するとき、encave は
Codex 自身の認証情報ストア（`cli_auth_credentials_store`）を取り除き、すべてのモデル
プロバイダの `env_key` を、キーリングのトークンを注入する固定の変数に強制します。
これにより、ユーザーのプロバイダ設定が `env_key` を宣言していなくても起動は認証され、
エージェントは Codex の保存済みログインに依存しません。トークンは常に `encave auth set`
から来ます。

認証情報は OS キーリングの `encave` サービス配下にのみ保存されます。

## アーキテクチャ

- `internal/adapter` — 対象 CLI の抽象化。`codex.go`（ホーム環境変数 `CODEX_HOME`、
  `env_key` / `env_http_headers` を読んで注入すべき環境変数を判断し、`codex -c key=value`
  のオーバーライドを構築）と `claude.go`（ホーム環境変数 `CLAUDE_CONFIG_DIR`、JSON 設定、
  認証情報は注入しない — ターゲット節を参照）。
- `internal/secrets` — キーリングのラッパー。値を返す唯一の呼び出し `Resolve` は
  起動パスでのみ使われます。
- `internal/scan` — `publish` が使う fail-closed の秘密情報スキャナ。
- `internal/fsutil` — 除外付きの再帰コピー。`new` で使用。
- `internal/gitutil` — `git` CLI の薄いラッパー。
- `internal/semver` — `vX.Y.Z` バージョンの解析・比較（リリースタグと encave 自身の
  バージョン）。
- `internal/modproxy` — 自己更新チェック用に、Go モジュールプロキシから encave の最新
  バージョンをベストエフォートで取得。
- `internal/cli` — コマンドのディスパッチ（暗黙の `run` を含む）とハンドラ。

## ステータス

ターゲット: **Codex CLI**（静的/長命の認証情報、例: 30 日の PAT を起動時に注入する単一
カスタムプロバイダ）と **Claude Code**（隔離した `CLAUDE_CONFIG_DIR` で Claude Code 自身の
ログインを使用 — encave は何も注入しない）。次の予定: 汎用 Codex 認証（ChatGPT ログイン /
API キー）、エージェントごとの複数認証情報、取得側の信頼性（来歴・更新差分）。

## コントリビューション & セキュリティ

バグ報告や機能要望は [issues](https://github.com/sert-xx/encave/issues) で歓迎します。
セキュリティ上の脆弱性については、公開 issue ではなく非公開での報告をお願いします
（[SECURITY.md](SECURITY.md) を参照）。

## ライセンス

[MIT License](LICENSE) の下で公開しています。
