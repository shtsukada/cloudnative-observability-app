# Contributing Guide

本ドキュメントは、Issue/PRの進め方、開発ルールに関するガイドです。

## 行動規範/セキュリティ
- 参加者は[Code of Conduct](CODE_OF_CONDUCT.md)に従ってください。
- 脆弱性の疑いは公開Issueではなく、[SECURITY.md](SECURITY.md)の手順で報告してください。

## リポジトリ構成
- 本リポジトリはルート(ポータル)です。実装は子リポジトリ(infra/operator/monitoring/app/proto)に行います。
- ルートでは共通ポリシー（`.github/`・docs）を扱います。

## スタイルとポリシー
- 改行/LF, 末尾改行, スペースインデント等は [.editorconfig](.editorconfig) に準拠。
- **`:latest` タグ禁止**（Docker/Helm/ドキュメント例外は注記付きで）。
- 重大なポリシー・命名は [docs/contracts.md](docs/contracts.md) を参照。

## Lint / テスト（root）
- ルートでは以下の軽量チェックを CI が実行します：
  - YAML/JSON/Markdown 文法
  - 必須ファイルの存在
  - `:latest` の使用検知
