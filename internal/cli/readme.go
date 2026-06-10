package cli

import (
	"fmt"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
)

// renderAgentReadme produces a README.md tailored to a freshly scaffolded agent.
// It documents the encave consumer workflow (install / auth / run) using the
// agent's GitHub identity and discovered auth environment variables, lists the
// MCP servers the agent expects (which are not bundled), and leaves clearly
// marked TODOs for the maintainer to describe what the agent does.
//
// owner/repo are the agent's GitHub identity; target is the adapter name (e.g.
// "codex"); authVars are the credential env var names; providers and mcps are the
// model providers and MCP servers the source config referenced (not packaged —
// listed as setup requirements). Any of these may be empty.
func renderAgentReadme(owner, repo, target string, authVars []string, providers []adapter.ProviderInfo, mcps []adapter.MCPServerInfo) string {
	ref := owner + "/" + repo

	// Resolve target capabilities so the auth guidance matches how this target
	// actually authenticates. Unknown targets default to the encave-managed model.
	managedAuth := true
	baseCfg := ""
	if ad, err := adapter.Get(target); err == nil {
		managedAuth = ad.ManagedAuth()
		baseCfg, _ = ad.ConfigLayout()
	}

	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", repo)
	b.WriteString("> **TODO:** このエージェントが何向けにチューニングされているか、1〜2文で説明してください\n")
	b.WriteString("> （例:「セキュリティと性能の観点を備えた、丁寧なコードレビュー用エージェント」）。\n\n")

	b.WriteString("これは [**encave**](https://github.com/sert-xx/encave) のエージェントです。")
	fmt.Fprintf(&b, "**%s** CLI 用の自己完結・隔離されたエージェントホーム", target)
	b.WriteString("（設定＋オーケストレーション＋skill）を GitHub 経由で配布します。encave は専用の\n")
	if managedAuth {
		b.WriteString("ホームディレクトリで起動し、起動時に認証情報を注入するため、あなたの個人環境には\n")
		b.WriteString("一切触れず、秘密情報もこのリポジトリには保存されません。\n\n")
	} else {
		b.WriteString("ホームディレクトリで起動するため、あなたの個人環境には一切触れず、秘密情報も\n")
		b.WriteString("このリポジトリには保存されません。\n\n")
	}

	// 要件
	b.WriteString("## 要件\n\n")
	b.WriteString("- [encave](https://github.com/sert-xx/encave):\n")
	b.WriteString("  `go install github.com/sert-xx/encave@latest`\n")
	fmt.Fprintf(&b, "- ターゲット CLI: **%s**\n", target)
	if managedAuth {
		b.WriteString("- Linux では keyring 用に稼働中の Secret Service（例: gnome-keyring）\n\n")
	} else {
		b.WriteString("\n")
	}

	// インストール
	b.WriteString("## インストール\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave install github.com/%s --tag <tag>\n", ref)
	b.WriteString("```\n\n")
	b.WriteString("リリース済みの `<tag>` を指定すると、バイト単位で再現可能なインストールになります。\n\n")

	// 認証情報
	b.WriteString("## 認証情報\n\n")
	if !managedAuth {
		b.WriteString("このターゲットの認証情報は encave では管理しません。ターゲット CLI 自身のログインを\n")
		b.WriteString("使います:\n\n")
		b.WriteString("- **macOS**: 通常の `claude /login`（OS の Keychain に保存）がそのまま使えます。\n")
		b.WriteString("  encave が隔離ホームを使っても、Keychain はグローバルなのでログイン状態を保てます。\n")
		b.WriteString("- **Linux / Windows**: 隔離ホームは最初ログアウト状態です。中で一度だけ認証してください\n")
		b.WriteString("  （`encave " + ref + " -- /login`、または `claude setup-token` で得た\n")
		b.WriteString("  `CLAUDE_CODE_OAUTH_TOKEN` を設定）。\n\n")
		b.WriteString("> **TODO:** 接続先ゲートウェイがある場合は `ANTHROPIC_BASE_URL`（環境固有・非梱包）の\n")
		b.WriteString("> 設定方法を記載してください。\n\n")
	} else if len(providers) > 0 || len(authVars) > 0 {
		b.WriteString("このエージェントのモデルプロバイダはベアラートークンを必要とします。OS の keyring に\n")
		b.WriteString("一度保存すれば、encave が起動時にプロバイダへ注入します（プロバイダの `env_key` を\n")
		b.WriteString("強制設定するので、環境変数を自分で設定する必要はありません）:\n\n")
		b.WriteString("```sh\n")
		b.WriteString("encave auth set --global              # 全エージェントで共有\n")
		fmt.Fprintf(&b, "encave auth set --agent %s   # またはこのエージェント専用に\n", ref)
		b.WriteString("```\n\n")
		b.WriteString("> **TODO:** このトークンの入手方法（どのプロキシ/PAT か）や、必要なスコープ・\n")
		b.WriteString("> 有効期限を記載してください。\n\n")
	} else {
		b.WriteString("このエージェントはトークンを要するモデルプロバイダを宣言していません。必要であれば\n")
		b.WriteString("ここに記載し、`encave auth set` で保存してください。\n\n")
	}

	// モデルプロバイダ（パッケージに含めない＝環境固有）
	if len(providers) > 0 {
		b.WriteString("## モデルプロバイダ\n\n")
		b.WriteString("このエージェントのモデルプロバイダはパッケージに**含まれていません**（base URL や\n")
		b.WriteString("wire protocol は環境固有のため）。あなた自身の `~/.codex/config.toml` に対応する\n")
		b.WriteString("プロバイダを設定してください。認証トークンの配線は encave が起動時に行います。\n")
		b.WriteString("作者は次の設定で構築しました:\n\n")
		for _, p := range providers {
			fmt.Fprintf(&b, "- **%s**", p.Name)
			var bits []string
			if p.BaseURL != "" {
				bits = append(bits, "base_url `"+p.BaseURL+"`")
			}
			if p.WireAPI != "" {
				bits = append(bits, "wire_api `"+p.WireAPI+"`")
			}
			if len(bits) > 0 {
				b.WriteString(" — " + strings.Join(bits, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n> **TODO:** あなたの環境でこのプロバイダにアクセスする方法を記載してください。\n\n")
	}

	// 必要な MCP サーバー（パッケージに含めない＝各自のホームで設定）
	if len(mcps) > 0 {
		b.WriteString("## 必要な MCP サーバー\n\n")
		b.WriteString("このエージェントは次の MCP サーバーを前提とします。これらはパッケージに**含まれません**\n")
		b.WriteString("（他人の MCP 設定の流用は危険なため）。利用前に、あなた自身の `~/.codex/config.toml`\n")
		b.WriteString("にインストール・設定してください:\n\n")
		for _, m := range mcps {
			switch {
			case m.URL != "":
				fmt.Fprintf(&b, "- **%s**（リモート） — `%s`\n", m.Name, m.URL)
			case m.Command != "":
				cmd := m.Command
				if len(m.Args) > 0 {
					cmd += " " + strings.Join(m.Args, " ")
				}
				fmt.Fprintf(&b, "- **%s** — `%s`\n", m.Name, cmd)
			default:
				fmt.Fprintf(&b, "- **%s**\n", m.Name)
			}
		}
		b.WriteString("\n> **TODO:** 各サーバーの導入・設定メモ（パッケージ、認証、環境変数）を追記してください。\n\n")
	}

	// 実行
	b.WriteString("## 実行\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave %s                  # 起動（run は省略可能なデフォルトコマンド）\n", ref)
	b.WriteString("encave run                           # または一覧から対話的に選択\n")
	b.WriteString("```\n\n")
	b.WriteString("`--` 以降の引数はターゲット CLI へそのまま渡されます。`--dry-run` で、起動せずに\n")
	b.WriteString("実行コマンド（認証情報はマスク）を確認できます:\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave %s --dry-run -- exec \"do the task\"\n", ref)
	b.WriteString("```\n\n")

	// メンテナ向け
	b.WriteString("## メンテナ向け\n\n")
	b.WriteString("このエージェントは encave で構築・公開されています:\n\n")
	b.WriteString("```sh\n")
	newCmd := "encave new " + ref
	if target != "" && target != adapter.DefaultName {
		newCmd += " --target " + target
	}
	fmt.Fprintf(&b, "%s   # ベースホームから雛形作成（秘密情報は除外）\n", newCmd)
	editTargets := "agents/、skills/"
	if baseCfg != "" {
		editTargets += "、" + baseCfg
	}
	fmt.Fprintf(&b, "# ...%s を調整...\n", editTargets)
	fmt.Fprintf(&b, "encave publish %s --tag <tag> --remote git@github.com:%s.git\n", ref, ref)
	b.WriteString("```\n\n")
	b.WriteString("`encave publish` はコミット前に fail-closed の秘密スキャンを実行します。認証情報は\n")
	b.WriteString("keyring に保管し、このリポジトリには絶対に置かないでください。\n\n")

	b.WriteString("---\n\n")
	b.WriteString("<sub>`encave new` により生成。上記の TODO を埋めて、このエージェントを説明してください。</sub>\n")

	return b.String()
}
