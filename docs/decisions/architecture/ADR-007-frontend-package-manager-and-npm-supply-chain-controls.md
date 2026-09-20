# ADR-007: フロントエンドのパッケージマネージャーとJavaScriptサプライチェーン対策

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-012, NFR-001, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-001はReactとViteを提案している。これらとテスト用依存関係はJavaScript package ecosystemから取得するため、パッケージマネージャー、依存関係の固定、install-time lifecycle script、registry外依存の扱いを決める必要がある。悪意ある更新、typosquatting、アカウント侵害、依存関係のinstall script実行は、JavaScriptサプライチェーン攻撃の代表的な経路である。

このADRはフロントエンドのJavaScript package managerと、その利用時に必須とする防御策を決める。依存関係ごとの採否、SBOM生成、脆弱性修正のSLA、private registryは初回Sliceには必要ないため対象外とする。

## Decision drivers

- React/Viteと開発用テスト依存関係を再現可能に取得できること
- lockfileを使い、レビューしていない依存解決の変化をCIへ持ち込まないこと
- dependencyのinstall scriptとregistry外のtarball/URL依存を明示的に制限できること
- 新たなpackage managerのbootstrap・保守負荷を最小化できること
- ローカル開発とCIで同一の依存関係ツリーを利用できること

## Options considered

### Option A: npm CLIと`package-lock.json`

Node.jsに同梱されるnpm CLIを使用し、コミットした`package-lock.json`と`npm ci`でCIと再現可能なローカルinstallを行う。

**利点**
- React/Vite ecosystemの標準的なlockfileとCIコマンドを使用できる
- 別package managerのbootstrapを必要としない
- npm 12以降では、project-scopedなinstall script allowlistと未許可scriptを失敗にする設定を利用できる

**欠点**
- Node.jsとnpmの組合せを明示的に固定しなければ、ローカルとCIの挙動がずれる
- npm registryを利用するため、lockfileだけでは新規依存の悪意・脆弱性を判定できない

### Option B: pnpmと`pnpm-lock.yaml`

pnpmを導入し、content-addressable storeとpnpmのlockfileを利用する。

**利点**
- 厳格な依存解決レイアウトにより、未宣言依存への偶発的な依存を発見しやすい
- release-age制御、依存パッケージのbuild script許可リスト、registry外のtransitive dependency制限を設定できる

**欠点**
- Node.js以外にpnpmの配布・version固定・更新手順を保守する必要がある
- チームとCIにnpmとは異なる運用規約を導入する

### Option C: Yarn Berryと`yarn.lock`

Yarn Berryを導入し、Yarnのlockfileおよび必要に応じてPlug'n'Playを利用する。

**利点**
- dependency treeの制約や高度なworkspace機能を利用できる
- lockfileを通じた再現可能installを提供できる

**欠点**
- 初回Sliceに不要なYarn固有設定・互換性判断が増える
- React/Viteの単一アプリケーションに対して導入・学習コストを正当化しにくい

## Decision

**pnpm 12系とコミット済みの`pnpm-lock.yaml`を採用することを推奨する。** Node.jsとpnpmは、ローカル開発・CIともに同一の正確なversionへ固定する。CIは`pnpm install --frozen-lockfile`だけを実行し、manifestとlockfileの不一致、またはlockfile不在を失敗とする。

以下を必須のサプライチェーン防御とする。

- 直接依存は正確なversionで宣言し、`pnpm-lock.yaml`を常にコミット・レビューする
- 依存関係の追加・更新は、変更された`package.json`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`を同一変更としてレビューする
- registryはHTTPSのnpm registryに限定し、直接のgit、file、URL、別hostのtarball依存は、別ADRで明示的に承認されるまで禁止する。`blockExoticSubdeps: true`によりtransitive dependencyからの同種依存も拒否する
- `allowBuilds`を空のallowlistから開始し、`strictDepBuilds: true`を有効にする。build/install scriptが必要なdependencyは、名前、version、必要理由をレビューしallowlistへ追加する。未確認のscriptはCIを失敗させる
- `minimumReleaseAge: 1440`（24時間）と`minimumReleaseAgeStrict: true`を有効にし、公開直後のpackage versionを依存解決から除外する。例外は名前・version・理由をレビューした上で最小限に限定する
- `strictStorePkgContentCheck: true`を有効にし、lockfileに記録したintegrityと異なるtarballを失敗させる。checksumを更新する操作は、変更理由と取得元をレビューする
- `pnpm dlx`や一時的な`pnpm exec`で未固定packageを実行しない。ツールはproject dependencyとしてlockfileへ固定する
- `pnpm audit`の結果は依存更新レビューで確認するが、単独で安全性を保証する根拠にはしない

## Rationale

pnpmは、lockfileを更新しないfrozen install、lockfile integrityの不一致を失敗させる検証、依存パッケージのbuild scriptを明示許可する設定を提供する。24時間のrelease-age制御を加えることで、公開直後の侵害やtyposquattingを検知・撤回する時間を確保する。pnpmを別途配布・固定する負担は生じるが、初回Sliceのnpmサプライチェーン対策としてその負担を上回る。これらの統制は依存自体の安全性を保証しないため、最小依存・変更レビューと組み合わせる。

## Consequences

### Positive

- ローカルとCIで同じlockfileから再現可能な依存関係ツリーを構築できる
- 未レビューのbuild script、URL依存、公開直後の依存、アドホックな実行を防げる
- pnpmの厳格な依存解決により、未宣言dependencyへの偶発的な依存を発見しやすい

### Negative / trade-offs

- Node.js/pnpmの正確なversionを開発環境とCIで保守する必要がある
- build scriptを必要とする正当なdependencyも、事前レビューとallowlist更新が必要になる
- 24時間を超えて検知されない侵害済みpackageは、lockfile・release-age・`pnpm audit`だけでは防げない

## Validation

- lockfileがない、または`package.json`と一致しない状態で`pnpm install --frozen-lockfile`が失敗することを確認する
- allowlist外のbuild scriptを含むdependencyを追加した場合、`strictDepBuilds`によりinstallが失敗することを確認する
- 公開から24時間未満のpackage versionを追加した場合、`minimumReleaseAgeStrict`によりinstallが失敗することを確認する
- CIが`pnpm install --frozen-lockfile`だけを用い、lockfileを書き換えないことを確認する
- URLまたはgit依存が直接またはtransitiveに追加された場合、`blockExoticSubdeps`またはreview policyで検出・拒否されることを確認する

## Revisit conditions

- pnpmのversion固定またはallowlist運用が、得られるサプライチェーン防御を上回る過剰な開発・運用負荷になった場合
- monorepo、複数package、またはworkspace要件がnpmの運用を大幅に複雑化する場合
- private registry、組織のartifact proxy、署名検証、SBOM提出が必須になった場合
