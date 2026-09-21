# ADR-012: TypeScriptとGoのFormatterおよびLinter

- Status: Accepted
- Date: 2026-09-21
- Owners: Human project owner
- Related requirements: FR-012, FR-013, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

Accepted ADR-001、ADR-002、ADR-006により、初回Vertical SliceはTypeScript/ReactとGoで実装し、型チェック、層別テスト、HTTP統合テスト、ブラウザE2Eテストを行う。実装を開始する前に、両言語のソースコードを一貫した形式に保ち、テストや型チェックだけでは検出しにくい問題を早期に検出するFormatterおよびLinterを決定する必要がある。

Formatterは、レビューで本質的ではない書式差分を減らすために使用する。Linterは、コンパイル可能でも疑わしい構文、誤用しやすいAPI、保守性を損なうパターンを検出するために使用する。これらは型チェック、Go test、HTTP/DB統合テスト、UIテスト、E2Eテストを置き換えない。

ADR-007はJavaScript依存関係をpnpmで厳格に管理することを定め、`docs/development/toolchain.md`はGo 1.27.1、Node.js 26.9.0、pnpm 12.5.1、TypeScript 7.0.2を固定している。本ADRでは、追加するJavaScript開発依存関係の最小化と、Go標準toolchainを利用する可否も評価する。formatter/linterの設定ファイル、package script、CI設定、および依存関係の追加は、本ADRがAcceptedになるまで実施しない。

## Decision drivers

- TypeScript/ReactとGoのコードを、ローカル開発とCIで再現可能かつ一貫して検査できること
- 書式の自動修正と、CIでの非破壊的な検査を明確に分離できること
- TypeScriptの型チェックおよびGoのコンパイル・テストと役割が重複しすぎないこと
- 初回Sliceに不要な設定、プラグイン、実行バイナリ、JavaScriptサプライチェーン上の依存を増やさないこと
- 開発者が診断の理由と修正方法を把握しやすいこと
- 将来、より強い静的解析が必要になったときに置換・追加を評価できること

## Options considered

### TypeScript Option A: BiomeをFormatterとLinterに共用する

`@biomejs/biome`を単一の開発dependencyとして導入し、TypeScript、TSX、JavaScript、JSONを対象にformatterとlinterを実行する。TypeScriptの型検査は既存の`tsc -b`で継続する。

**利点**

- FormatterとLinterを単一の設定・CLI・開発dependencyで運用できる
- PrettierとESLintの設定、実行方法、書式規則の重複を避けられる
- TypeScriptとReactの初回Sliceに必要な書式化と基本的な静的検査を高速に実行できる

**欠点**

- ESLint固有pluginを前提とする既存資産や組織標準が将来必要になった場合、追加評価または移行が必要になる
- 型情報を利用するESLint rule群と同等の検査をすべて提供するものではない

### TypeScript Option B: PrettierとESLint（`typescript-eslint`を含む）を併用する

PrettierをFormatter、ESLintと`typescript-eslint`をLinterとして導入する。必要に応じてReact向けpluginも追加する。

**利点**

- TypeScript/React向けの成熟したplugin ecosystemと、多数の既存設定を利用できる
- 型情報を利用するruleを段階的に追加できる

**欠点**

- 少なくともPrettier、ESLint、`typescript-eslint`という複数の開発dependencyと設定を保守する必要がある
- formatter ruleとlint ruleの重複・競合を避けるための設定判断が必要になる
- 初回Sliceに対し、plugin選定とアップグレード追従の負荷が大きい

### Go Option A: Go標準の`gofmt`と`go vet`を使用する

Go 1.27.1に含まれる`gofmt`で書式を統一し、`go vet ./...`でGo標準の静的解析を実行する。外部のGo formatter/linter binaryは導入しない。

**利点**

- 追加のGo module、実行binary、設定ファイルを必要とせず、固定済みGo toolchainだけで再現できる
- Goコミュニティの標準書式である`gofmt`を使用できる
- `go vet`により、コンパイルは通るが問題になり得る構文・APIの利用を検出できる

**欠点**

- `go vet`の検査範囲は意図的に限定され、保守性、複雑さ、命名などを広範には検査しない
- `gofumpt`やStaticcheck、golangci-lintが提供する追加診断は得られない

### Go Option B: `gofumpt`とStaticcheckを使用する

`gofumpt`で`gofmt`より厳しい書式を適用し、Staticcheckで追加の静的解析を実行する。

**利点**

- 標準toolchainより厳格な書式と、より広い不具合・不要コードの検出を得られる
- 各ツールの責務が比較的明確である

**欠点**

- tool binaryまたはGo moduleとversion固定、更新、CI配布を追加で保守する必要がある
- 初回Sliceに必要な規約を超える診断への対応・抑制設定が発生する可能性がある

### Go Option C: golangci-lintを集約Linterとして使用する

`gofmt`に加え、golangci-lintを導入して複数のGo analyzerを一つの設定と実行コマンドで管理する。

**利点**

- 多数の静的解析器を統一した実行とレポートで利用できる
- 将来、検査を追加する際の導入経路がある

**欠点**

- analyzer集合、設定、version更新により診断内容が大きく変わり得る
- 初回Sliceには不要なbinary、設定、除外ルールを先行して保守することになる
- 個別の診断が必要な理由を判断せずに有効化しやすい

## Decision

**TypeScriptにはBiome、GoにはGo標準の`gofmt`と`go vet`を採用することを推奨する。**

TypeScriptでは、`@biomejs/biome` **2.5.14**を正確なversionの直接開発dependencyとして追加し、formatterとlinterの両方に使用する。設定は、初回Sliceで実装するTypeScript、TSX、JavaScript、JSONだけを対象にし、生成物、dependency directory、テストの出力directoryを検査対象から除外する。初期rule setはBiomeの推奨ruleを基礎とし、プロダクト固有のrule追加または例外は、検出した具体的な問題と理由を同じ変更でレビューする。`tsc -b`による型チェックは独立して継続する。

Goでは、`gofmt`を唯一のFormatter、`go vet ./...`を初期SliceのLinter相当の静的解析として使用する。`go test ./...`は`go vet`の代替とみなさず、CIでは明示的に`go vet ./...`を実行する。`gofumpt`、Staticcheck、golangci-lintは導入しない。

ローカルではFormatterによる修正を許可する。一方、CIではファイルを書き換えず、TypeScriptのformat/lint、Goの`gofmt`未適用ファイル、`go vet ./...`をそれぞれ失敗として報告する。実装時には、これらに加えて既存の型チェックとテストを個別に実行できるpackage scriptまたはrepository scriptを定義する。CI上の`gofmt`検査は、対象のGo source fileを列挙し、`gofmt -l`の出力が空でない場合に失敗するrepository scriptで実装する。

Biomeの追加はADR-007に従う。すなわち、`package.json`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`を同一変更としてレビューし、正確なversionを固定する。Biomeまたはそのdependencyがbuild/install scriptの実行を要求した場合、必要性がレビューされ、`allowBuilds`への最小限の明示許可がAccepted ADR-007に従って追加されるまで導入しない。

## Rationale

TypeScriptでは、BiomeがFormatterとLinterを一つの開発dependencyと設定にまとめるため、React/Viteの単一アプリケーションでPrettierとESLint ecosystemを同時に管理するよりも依存関係と設定上の重複を抑えられる。型に関する正しさは、すでに固定済みのTypeScript compilerによる`tsc -b`で検査する。初回Sliceに必要なのは、広範なplugin ecosystemを先取りすることではなく、書式と基本的な静的検査を一貫して実行することである。

Goでは、`gofmt`が言語標準の書式を提供し、`go vet`が標準toolchainに含まれる高信頼度の静的解析を提供する。状態遷移、認可、監査、並行性というプロダクト固有の重要リスクは、ADR-006で定めたdomain/application test、HTTP/DB integration test、E2E testで検証する。初回から集約Linterの多数の診断を導入するより、`gofmt`、`go vet`、テストの責務を明確に分離する方が、依存関係と運用規約を最小に保てる。

## Consequences

### Positive

- TypeScriptとGoで、開発者ごとの書式差分を減らし、CIで一貫して検査できる
- TypeScriptのFormatter/Linterは単一の開発dependencyと設定で開始できる
- Goの書式・基本的な静的解析は、追加binaryなしに固定済みtoolchainで再現できる
- 型チェック、静的解析、各層のテストを別々に実行でき、失敗原因を局所化しやすい
- Biomeの導入はADR-007のlockfile、release-age、build script統制の対象になる

### Negative / trade-offs

- BiomeのruleまたはESLint plugin固有の検査は、必要になった時点で追加評価が必要になる
- `go vet`だけでは検出できないGoの保守性・性能・バグの兆候が残る可能性がある
- Formatterの自動修正により、意図しない大きな差分が生じないよう、変更対象を確認する必要がある
- TypeScriptとGoのCI検査コマンドおよび設定を、固定toolchainの更新時に維持する必要がある

## Validation

- Biome設定とpackage script導入後、未整形のTypeScript/TSX/JSON fileをFormatter検査が検出し、`pnpm`経由のLinterが意図的なrule違反を検出することを確認する
- `tsc -b`がBiomeとは独立して型エラーを検出することを確認する
- 未整形のGo source fileがCI用`gofmt`検査で失敗し、ローカルの`gofmt`適用後に成功することを確認する
- `go vet ./...`が意図的に導入した検出可能な問題で失敗し、`go test ./...`とは別のコマンドとして実行されることを確認する
- CIのformat/lint検査がworking treeを書き換えず、固定したNode.js、pnpm、Go versionで再現可能に実行できることを確認する
- Biome導入時に、`pnpm install --frozen-lockfile`、`strictDepBuilds`、`allowBuilds`、`minimumReleaseAge`を含むADR-007の統制が満たされることを確認する

## Revisit conditions

- TypeScript/Reactで必要な検査がBiomeのrule・parser・editor integrationでは満たせず、ESLint pluginまたは別ツールの導入便益が追加依存・設定コストを上回る場合
- 本番障害、レビュー上の反復的な問題、またはセキュリティ要件により、`go vet`を超えるGo静的解析が必要と判明した場合
- 生成コード、monorepo、複数package、または組織共通のlint規約が追加され、対象範囲・設定の共有方法を見直す必要が生じた場合
- Formatter/Linterの実行時間または誤検知が開発フィードバックを損ない、rule setまたはツール選定の見直しが必要になった場合
