# Order Plugin Developer Kit · 注文プラグイン開発キット · 주문 플러그인 개발키트

Choose your language / 言語を選択 / 언어를 선택하세요.

- [English — Getting started](en.md)
- [日本語 — スタートガイド](ja.md)
- [한국어 — 시작 가이드](ko.md)

All three guides cover the same workflow: current scope, running the template,
read-only checks, adapter development, simulator tests, safety, and local/remote/mobile use.
各言語版は同じ構成です。세 언어 가이드는 동일한 구성으로 제공합니다.

v1 is experimental and paper-only, not a ready-to-use live-trading product.
v1は実験段階で、実口座取引には未対応です。
v1은 실험 단계이며, 바로 실계좌 주문을 할 수 있는 기능은 아닙니다.

## Shared reference / 共通仕様 / 공통 규격

- [Technical reference (English)](protocol.md)
- [OpenAPI specification](../api/order-plugin-v1.openapi.yaml)
- [Standalone template](../../examples/order-plugin/)
- [Public Go SDK](../../pkg/brokerplugin/)
- [Read-only checker](../../cmd/plugin-check/)

## Maintaining these guides / 文書の更新 / 문서 관리

Update `en.md`, `ja.md`, and `ko.md` together when behavior or commands change.
Keep command arguments, API paths, field names, and safety guarantees consistent.
The shared technical reference and OpenAPI remain the protocol definitions;
do not maintain separate language-specific API schemas.

仕様やコマンドを変更した場合、3言語のガイドを同時に更新してください。
동작이나 명령이 달라지면 세 언어 가이드를 함께 갱신하고, API 규격은 공통 파일로 관리합니다.
