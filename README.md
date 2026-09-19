# taggo

ローカルファイルのドキュメントとアセット（画像・音声）をタグ管理できるデスクトップアプリです。

## タグ管理方式

タグはファイル内埋め込み方式を採用しています。以下のファイル形式に対応しています。

- Markdown
- 画像ファイル (jpg, png, webp, gif, svg)
- 音声ファイル (mp3, wav, aac, flac)

## Markdown

ドキュメントは Markdown 形式で、Front Matter（YAML）形式をサポートしています。

```markdown
---
tags:
  - golang
  - 開発メモ
created: 2026-09-18
---

# ここから本文
```

### 画像ファイル

画像ファイルは .jpg, .png, .webp 形式です。

Exif.Image.XPKeywords や XMP:Subject などの標準タグ領域に書き込みます。

### 音声ファイル

音声ファイルは .mp3, .wav 形式です。

.mp3 は ID3v2 タグ（GenreやComment、Keywords欄）にタグ文字列を書き込みます。

.wav は INFO chunk や ID3 chunk に埋め込みます。
