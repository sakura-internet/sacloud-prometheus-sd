# Prometheus file_sd_config for sakura cloud

さくらのクラウドのAPIを利用し、Prometheusのfile_sd_configで読み込み可能なYamlファイルを生成します。
自動的にサーバの情報を収集してPrometheusの監視対象へ追加することが出来ます。


## インストール

todo


## 設定

### 引数

```
Usage of sacloud-prometheus-sd:
  -config string
        config file path (default "config.yml")
  -generated string
        generated file path (default "./generated.yml")
  -i int
        refresh interval. if not specified once run.
  -secret string
        sakura cloud API secret
  -token string
        sakura cloud API token
  -zone string
        sakura cloud zone name
```

### 設定ファイル `config.yml`

```yaml
# コントロールパネルから生成したAPIトークン・シークレット情報
# 環境変数 SAKURACLOUD_ACCESS_TOKEN, SAKURACLOUD_ACCESS_TOKEN_SECRET でも指定可能
# 取得方法: https://manual.sakura.ad.jp/cloud/api/apikey.html
sacloud_token: ""
sacloud_token_secret: ""

# 対象のゾーン名 / tk1a(東京第1), is1a(石狩第1), is1b(石狩第2), tk1v(Sandbox)
# 環境変数 SAKURACLOUD_ZONE でも指定可能
sacloud_zone: "is1b"

# 収集するサーバに共通して付与されているタグ
# 「develop, production」等が指定されることを想定・未指定で全てのサーバを対象
base_tags:
  - "develop"

# 収集するサーバの情報・複数指定可能
targets:
  - service: "front-lb"       # Prometheus側に渡すサービス名
    tags: ["nginx", "lb"]     # 対象とするタグ・複数指定可能
    ignore_tags: ["sandbox"]  # 指定されていれば無視するタグ・複数指定可能
    ports: [1000, 2000]       # exporterが動作しているTCPポート番号・複数指定可能
    interface_index: 0        # IPアドレスを取得するNICのIndex番号・デフォルト0
  - service: "api-server"
    tags: ["api"]
    ports: [3000]
```
