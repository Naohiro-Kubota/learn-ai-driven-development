# ADR-014: Codexによる作業ブランチのGit参照書込み

- Status: Proposed
- Date: 2026-09-22
- Owners: Human project owner
- Related requirements: NFR-001, NFR-002, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

プロジェクトローカルの`.codex/config.toml`は`workspace-write` sandboxを使っている。この標準policyではworkspace内であっても`.git`は再帰的にread-onlyであるため、Codexが`git switch -c`、`git commit`、`git push`で作業ブランチを作成・更新できない。実際に`refs/heads/...lock`の作成が拒否された。

開発者がCodexへ変更を依頼した際、変更を専用branchとPull Requestに記録できることはNFR-006のローカル開発フローとNFR-002のトレーサビリティに寄与する。一方で`.git`への書込みを許可すると、commit、branch、index、local configurationを変更できる範囲が広がる。特に`main`と`develop`への直接操作を防ぐ統制を維持する必要がある。

OpenAI公式ドキュメントは、通常の`workspace-write`で`.git`をread-onlyとし、filesystem ruleを持つカスタムpermission profileを設定できることを説明している。

## Decision drivers

- Codexがfeature branchの作成、commit、pushをローカルGit経由で実行できること
- `main`および`develop`への直接変更・push・mergeを引き続き防ぐこと
- workspace外、秘密情報、ネットワーク許可範囲を必要以上に拡大しないこと
- 権限の理由と有効範囲をrepository内でレビュー可能にすること
- macOS sandbox上で実際に強制される設定だけを採用すること

## Options considered

### Option A: 現在の`workspace-write`を維持する

`.git`をread-onlyのままとし、GitHub APIやユーザー操作を通じてbranch・commit・Pull Requestを作成する。

**利点**
- Git metadataへの書込みをCodexに許可しない。
- 現在のsandbox境界と互換性がある。

**欠点**
- 通常のGit workflowを実行できず、API経由の代替実装や手動作業が必要になる。
- commit作成の操作経路が分散し、ローカルでの検証結果とcommit対象の対応を確認しにくい。

### Option B: `.git`だけを書込み可能にするカスタムpermission profileを導入する

旧来の`approval_policy` / `sandbox_mode` / `sandbox_workspace_write`設定を、`:workspace`を継承する`project-git-write` profileへ移行する。そのprofileのfilesystem ruleでworkspaceは書込み可能なまま、`.git`を明示的に書込み可能にする。既存のnetwork proxy allowlistをprofileへ移し、`.codex`と`.agents`はread-onlyのままとする。version管理済みのPreToolUse hookは、`main`と`develop`を直接変更・push・mergeするGit/GitHub commandを拒否し続ける。

**利点**
- feature branch上で通常のGit workflowを実行できる。
- `danger-full-access`を使わず、許可範囲を`.git`へ限定できる。
- repository内の設定とhookを同じPull Requestでレビューできる。

**欠点**
- Codexがfeature branchのcommit、branch、index、Git設定を変更可能になる。
- hookはCodex shell commandを対象とする防御であり、GitHub branch protectionや人間によるPR reviewの代替にはならない。
- permission profileの実効性はCodex versionとmacOS sandbox実装に依存するため、実機検証が必要になる。

### Option C: `danger-full-access`を使う

sandboxとapprovalを全面的に迂回する。

**利点**
- `.git`を含むすべてのローカル操作が可能になる。

**欠点**
- workspace外を含むsandbox境界を失い、必要な権限を大幅に超える。
- network、秘密情報、削除操作への露出が増える。
- 最小権限と明示的な保護branch統制の目的に反する。

## Decision

**Option Bを採用することを提案する。**

承認後、`.codex/config.toml`をcustom permission profileへ移行し、`.git`に限ったwrite ruleを追加する。`main`および`develop`に対するPreToolUse hookの拒否規則は維持し、GitHub上のbranch protectionとPull Request reviewをauthoritative controlとして扱う。`danger-full-access`は採用しない。

このADRがAcceptedになるまで、`.codex/config.toml`の権限設定は変更しない。

## Rationale

Option Bは、branch作成拒否の原因である`.git`の保護だけを狭く緩和し、workspace外へのwriteやsandbox全体の無効化を避ける。protected branch hookとGitHubのserver-side controlsを重ねることで、Codexがfeature branchで通常の開発作業を行える一方、主要branchの直接変更を防ぐ。

## Consequences

### Positive

- Codexはfeature branchをローカルで作成し、検証済みの変更を通常のGit commitとして記録できる。
- Pull Request作成がGitHub APIへの代替経路に依存しなくなる。
- 許可範囲と保護branch統制をversion管理された設定・hookとして監査できる。

### Negative / trade-offs

- feature branch上のGit metadataがCodexの書込み対象になる。
- Codex configの更新は新しいsessionから有効になるため、現在のsessionのsandbox制限は変わらない。
- hookのpattern検出をすり抜ける操作を完全に防ぐものではないため、GitHub側のbranch protectionを必須とする。

## Validation

ADRがAcceptedとなり設定を実装した後、macOSで次を確認する。

- `codex sandbox macos --permissions-profile project-git-write git switch -c codex/sandbox-write-probe`がGit refを作成できること
- feature branch上で空のtest commitを作成・削除できること
- `main`または`develop`上の`git update-ref`、`git push origin HEAD:develop`、PR merge commandをhookが拒否すること
- workspace外、`.codex`、`.agents`、秘密情報にwrite accessを与えないこと
- network accessが既存allowlistのhostへ限定されること

検証用branch・commitは確認後に削除し、実在のproduct変更へ混在させない。

## Revisit conditions

- Codexのpermission profileが`.git`の明示的write ruleをサポートしない、またはmacOS sandboxで強制できない場合
- protected branch hookの誤検知・見逃しが発生した場合
- GitHub branch protectionまたはorganization policyが変更された場合
- CodexがGit metadata以外の広いwrite accessを必要とする要件が追加された場合
