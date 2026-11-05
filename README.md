# go-vips

[purego](https://github.com/ebitengine/purego) で libvips（8.12 以上）の共有ライブラリを動的に読み込みます。cgo は不要です。64bit プラットフォーム（amd64 / arm64）のみ対応します。

## libvips のインストール

| OS | 共有ライブラリ | 入手方法 | 検索パス |
|---|---|---|---|
| Windows | `libvips-42.dll` | 公式のビルド済みバイナリ | `PATH` |
| Linux | `libvips.so.42` | ディストリビューションのパッケージ | 標準の場所（`LD_LIBRARY_PATH`） |
| macOS | `libvips.42.dylib` | Homebrew | `DYLD_LIBRARY_PATH` |

libvips に加えて、その依存である GLib の `libgobject-2.0` と `libglib-2.0` も読み込みます（Windows では `libgobject-2.0-0.dll` / `libglib-2.0-0.dll`）。どれも libvips と一緒に入ります。

**Windows**

[libvips/build-win64-mxe のリリース](https://github.com/libvips/build-win64-mxe/releases) から `vips-dev-w64-all-<バージョン>.zip`（すべての形式に対応した版）を展開し、`bin` を `PATH` に追加します。

```powershell
$env:PATH = "C:\vips-dev-8.18\bin;$env:PATH"   # 現在のセッションのみ
# ユーザー環境変数に永続的に追加（新しいターミナルから有効）
[Environment]::SetEnvironmentVariable("PATH", "C:\vips-dev-8.18\bin;" + [Environment]::GetEnvironmentVariable("PATH", "User"), "User")
```

配布時は `bin` の DLL（`vips-modules-*` ディレクトリを含む）を実行ファイルと同じディレクトリに置けば `PATH` の設定は不要です。

**Linux**

```sh
sudo apt install libvips42     # Debian / Ubuntu 22.04（Ubuntu 24.04 以降は libvips42t64）
sudo dnf install vips          # Fedora
```

**macOS**

```sh
brew install vips
export DYLD_LIBRARY_PATH="$(brew --prefix)/lib:$DYLD_LIBRARY_PATH"
```

`DYLD_*` は SIP により `/bin/sh` などのシステム付属プログラムを経由すると消えるため、シェルスクリプトや `make` から実行する場合はスクリプト内で再度 `export` してください。

### インストールの確認

検索パスを設定したターミナルでリポジトリのルートから `go test -count=1 -v .` を実行し、すべて `--- PASS` になれば使えます。ライブラリがないとスキップではなく `--- FAIL` になります（構造体レイアウトのテストを除く）。`TestLibrary` は libvips のバージョンを `-v` でログに出します。

| 出力 | 原因と対処 |
|---|---|
| `libvips is required by this test: vips: cannot load library ...` | 共有ライブラリが見つかりません。検索パスを確認してください。 |
| `libvips is required by this test: vips: minimum required libvips version is 8.12.0` | 古い libvips が先に見つかっています。新しい libvips のディレクトリを先にしてください。 |

自分のプログラムからは `vips.Dynamic()` で確認できます。

```go
if err := vips.Dynamic(); err != nil {
    log.Fatal("libvips を使えません:", err)
}
```

## 使い方

```go
import "github.com/f0reth/go-vips"

// 幅 800 ピクセルにリサイズして WebP にする（高さは縦横比から決まる）
thumb, err := vips.ThumbnailBuffer(buf, 800, 0, nil)
if err != nil {
    return err
}
defer thumb.Close()
webp, err := thumb.SaveBuffer(".webp", vips.Options{"Q": 80})

// ファイルの読み込み、操作、保存
im, err := vips.Load("in.jpg", nil)
defer im.Close()
small, err := im.Resize(0.5, vips.Options{"kernel": vips.KernelLanczos3})
defer small.Close()
err = small.Save("out.png", nil)
```

画像は不変で、操作は新しい画像を返します。libvips は画素を遅延評価し、保存や `ToImage` のときにまとめて計算します。

### リサイズ

ピクセル数で指定するときは `Thumbnail`（ファイル）/ `ThumbnailBuffer`（メモリ上のファイル）/ `ThumbnailImage`（読み込み済みの画像）を使います。`width` x `height` に収まるように縦横比を保って拡大縮小し、`0` を指定した方向は制限しません。

```go
vips.ThumbnailBuffer(buf, 800, 0, nil)                                            // 幅 800（768x512 → 800x533）
vips.ThumbnailBuffer(buf, 0, 600, nil)                                            // 高さ 600
vips.ThumbnailBuffer(buf, 800, 800, nil)                                          // 800x800 に収める（768x512 → 800x533）
vips.ThumbnailBuffer(buf, 800, 800, vips.Options{"size": vips.SizeDown})          // 縮小のみ（小さい画像はそのまま）
vips.ThumbnailBuffer(buf, 300, 300, vips.Options{"crop": vips.InterestingCentre}) // 300x300 を埋めて中央を切り抜く
vips.ThumbnailBuffer(buf, 300, 100, vips.Options{"size": vips.SizeForce})         // 縦横比を変えて 300x100 ちょうどに
```

`Thumbnail` / `ThumbnailBuffer` は JPEG や WebP などで読み込み時に縮小するため最も速く、さらに次の処理をします。

- EXIF の回転情報に従って正しい向きにします（スマートフォンの縦向き写真など）。`"no_rotate": true` で無効にできます。
- アルファチャンネルを乗算してから縮小するため、透明部分の色が縁ににじみません。
- CMYK の画像は sRGB に変換します。ICC プロファイル（Display P3 など）付きの画像は、画素もプロファイルもそのまま保ちます（保存してもプロファイルが付くので正しい色で表示されます）。sRGB に変換するには `"output_profile": "srgb"` を指定します。
- 16bit の画像は 8bit になります。16bit のまま縮小するには `Resize` を使います。
- アニメーション（GIF / WebP）は `"option_string": "n=-1"` で全フレームをリサイズします。指定しないと先頭のフレームだけになります。

`Thumbnail` / `ThumbnailBuffer` は結果をメモリ上に計算してから返すため、何度でも保存できます。ファイルは 1 つのスレッドが先頭から順にデコードし、ほかのスレッドはそれを待つだけなので、この計算は最大 2 スレッド（デコードと並行して縮小する）で行います。1 枚でも多数を並列に処理するときでも、`SetConcurrency` のスレッド数すべてを使うより速くなります。結果から計算する画像はすべてのスレッドを使います。多数の画像を並列に処理するときは、さらに `SetConcurrency(1)` にすると速くなります（[スレッド数の推奨設定](#スレッド数の推奨設定)）。

```go
thumb, err := vips.ThumbnailBuffer(buf, 800, 0, nil)
defer thumb.Close()
jpg, err := thumb.SaveBuffer(".jpg", vips.Options{"Q": 85})
webp, err := thumb.SaveBuffer(".webp", vips.Options{"Q": 80})
```

`"access": "sequential"` で読み込んだ画像も同じくファイルを読みながら計算するため 1 回しか保存できません。複数回使うときは `CopyMemory` でメモリ上にコピーしてください。

`Resize` は倍率で拡大縮小します（`vscale` で縦の倍率を別にできます）。補間方式は `"kernel"` で選べます（`nearest`、`linear`、`cubic`、`mitchell`、`lanczos2`、`lanczos3`（デフォルト）、`mks2013`、`mks2021`）。縮小ではすべて異なる結果になりますが、拡大では `nearest` / `linear` 以外はすべて `cubic` と同じ結果になります。`Thumbnail` 系は補間方式を選べません（縮小の結果は `Resize` のデフォルトとほぼ同じです）。倍率に `目標の幅 / 元の幅` を渡せば目標の幅ちょうどになります。`Thumbnail` 系と違い、回転・色空間・ビット深度は変えません。アルファ付きの画像は `Thumbnail` 系と同じく乗算してから縮小します（乗算した画像を元の形式に戻してから縮小するため、8bit の画像は高速なベクトル命令で処理されます。半分にすると `ThumbnailImage` と同じ画素になります）。アニメーションはフレームの高さが正しく変わらないためエラーになります（`ThumbnailImage` を使ってください）。

### 画像の解放

`*vips.Image` は libvips の画像への参照を 1 つ持ちます。libvips のメモリは Go の GC から見えないため、使い終わったら `Close` してください。`Close` しなかった画像も GC 時に解放されますが、いつになるかはわかりません。`Close` を 2 回呼んでも問題ありません。解放した画像のメソッドを呼ぶと panic します。

画像のメソッドは複数のゴルーチンから同時に呼べます（`Close` と同時には呼べません）。

### 操作

よく使う操作は、libvips の操作名を CamelCase にした `Image` のメソッドです。必須引数は引数で、省略可能な引数は `Options`（なしなら `nil`）で渡します。

| メソッド / 関数 | libvips の操作 |
|---|---|
| `Load` / `LoadBuffer` | 形式に合ったローダー（`jpegload` など） |
| `Save` / `SaveBuffer` | 拡張子に合ったセーバー（`jpegsave` など） |
| `Thumbnail` / `ThumbnailBuffer` / `ThumbnailImage` | `thumbnail` / `thumbnail_buffer` / `thumbnail_image`（幅と高さは引数、`0` は制限なし。`Thumbnail` / `ThumbnailBuffer` は `concurrency` を最大 2 にして `vips_image_copy_memory`） |
| `Resize` | `resize`（アルファ付きの画像は `premultiply` → `cast` と `unpremultiply` → `cast` で挟む） |
| `CopyMemory` | `vips_image_copy_memory` |
| `ExtractArea` / `Smartcrop` | `extract_area` / `smartcrop` |
| `Embed` / `Gravity` / `Insert` | `embed` / `gravity` / `insert` |
| `Rot` / `Rotate` / `Flip` / `Autorot` | `rot` / `rotate` / `flip` / `autorot` |
| `Colourspace` / `Cast` / `Copy` | `colourspace` / `cast` / `copy` |
| `Flatten` / `Composite2` | `flatten` / `composite2` |
| `Gaussblur` / `Sharpen` / `Invert` / `Linear` / `ExtractBand` | `gaussblur` / `sharpen` / `invert` / `linear` / `extract_band` |

それ以外の操作は `Call` で名前を指定して実行します。必須引数も省略可能な引数も `Options` で渡し、操作が設定した出力引数を名前で受け取ります。出力の画像は呼び出し側が `Close` します。

```go
out, err := vips.Call("max", vips.Options{"in": im})
// out = {"out": 255.0, "x": 226, "y": 496, "out_array": []float64{255}, ...}

out, err = vips.Call("bandjoin", vips.Options{"in": []*vips.Image{r, g, b}})
rgb := out["out"].(*vips.Image)
defer rgb.Close()
```

引数名と値は [libvips の API ドキュメント](https://www.libvips.org/API/current/) と同じです。名前の `_` と `-` は区別しません（`output_profile` と `output-profile` は同じ）。出力の名前は `_` で書きます。値は引数の型に合わせて変換されます。

| Go の値 | libvips の型 |
|---|---|
| `bool` | `gboolean` |
| 整数型 | `int`、`double`、列挙型・フラグの値 |
| 浮動小数点型 | `double` |
| 文字列型 | 文字列、列挙型・フラグのニックネーム（`"lanczos3"`、`"exif\|xmp"` など） |
| `*vips.Image` / `[]*vips.Image` | `VipsImage` / `VipsArrayImage` |
| `[]byte` | `VipsBlob`（コピー） |
| `[]float64` / `[]int`（数値 1 つは要素 1 つの配列） | `VipsArrayDouble` / `VipsArrayInt` |

出力は `bool`、`int`、`uint64`、`float64`、`string`（列挙型はニックネーム）、`*vips.Image`、`[]byte`、`[]float64`、`[]int` になります。存在しない引数名、変換できない値、範囲外の値はエラーになります。

列挙型（`Kernel`、`Interpretation`、`BlendMode` など）は libvips のニックネームを値とする文字列型です。定数のない値も `vips.Options{"kernel": "mks2021"}` のように文字列で指定できます。

### メタデータ

```go
im.Width(); im.Height(); im.Bands(); im.Format(); im.Interpretation(); im.HasAlpha()
im.Fields()                   // メタデータのフィールド名
v, err := im.Get("n-pages")   // 値は Call の出力と同じ型
```

### image.Image との変換

```go
im, err := vips.FromImage(m)   // image.Image → *vips.Image（コピー）
m, err := im.ToImage()         // *vips.Image → image.Image（sRGB またはグレーに変換）
```

| `FromImage` の入力 | 結果 |
|---|---|
| `*image.Gray` などグレーの色モデル | 1 バンド `b-w` / 16bit なら `grey16` |
| RGBA64 / NRGBA64 の色モデル | `rgb16`、不透明なら 3 バンド、そうでなければ 4 バンド |
| その他（`*image.RGBA`、`*image.YCbCr`、`*image.Paletted` など） | `srgb`、不透明なら 3 バンド、そうでなければ 4 バンド |

`ToImage` は `rgb16` / `grey16` の画像を 16bit、それ以外を 8bit にし、アルファなしのグレーは `*image.Gray` / `*image.Gray16`、アルファなしのカラーは `*image.RGBA` / `*image.RGBA64`、アルファ付きは `*image.NRGBA` / `*image.NRGBA64` を返します（libvips はアルファを乗算しません）。

### 保存形式

`SaveBuffer` の拡張子と `Save` のファイル名の拡張子でセーバーが決まります（`.jpg`、`.png`、`.webp`、`.gif`、`.tif`、`.heif`、`.jxl` など）。使える形式は libvips のビルドによります。

- AVIF: libvips 8.18.7 の Windows 向けビルド済みバイナリでは、`.avif` はファイル保存（`Save`）のみで `SaveBuffer` には使えません。メモリに保存するときは `im.SaveBuffer(".heif", vips.Options{"compression": "av1"})` としてください。
- HEIC（HEVC）は同じバイナリにエンコーダーがないため保存できません（読み込みはできます）。

### 並列数とキャッシュ

```go
vips.SetConcurrency(4)          // 1 枚の画像を計算するスレッド数（0 でデフォルト）
vips.SetCacheMax(0)             // 操作キャッシュを無効にする（デフォルト 100 操作）
vips.SetCacheMaxMem(50 << 20)   // キャッシュが保持するメモリの上限（デフォルト 100 MiB）
vips.SetCacheMaxFiles(20)       // キャッシュが開いたままにするファイル数の上限（デフォルト 100）
```

#### スレッド数の推奨設定

libvips は画素を計算するたび（保存、`ToImage`、`CopyMemory` など）に `SetConcurrency` の数のスレッドを使います。デフォルトは論理 CPU 数（環境変数 `VIPS_CONCURRENCY` で変えられます）なので、多数の画像を複数のゴルーチンで処理すると「ゴルーチン数 × スレッド数」のスレッドが CPU を奪い合って遅くなります。

| 用途 | 推奨 |
|---|---|
| 多数の画像を複数のゴルーチンで並列に処理する（サーバー、一括変換） | 起動時に `vips.SetConcurrency(1)` とし、並列化はゴルーチンで行う |
| 1 枚ずつ処理する（数百万画素までの画像、サムネイル） | `vips.SetConcurrency(2)`〜`4` |
| 1 枚ずつ処理する（数千万画素の画像をメモリ上で処理） | 物理コア数程度（下の環境では 8） |

`BenchmarkConcurrency` の結果です（i7-13700F：P コア 8 + E コア 8、論理 24 コア、デフォルト 24 スレッド。libvips 8.18.7、Windows。1 回あたりの時間の中央値、ms）。小さい画像は 768x512、`Large` は 6144x4096 です。

| 関数 | 並列 1 | 2 | 4 | 8 | デフォルト | 逐次 1 | 2 | 4 | 8 | デフォルト |
|---|---|---|---|---|---|---|---|---|---|---|
| `LoadBuffer`（デコード） | **0.49** | 0.55 | 0.62 | 0.79 | 0.94 | 2.39 | 2.03 | **2.01** | 2.12 | 2.35 |
| `ThumbnailBuffer`（→256） | **1.18** | 1.32 | 1.33 | 1.38 | 1.38 | **3.00** | 3.04 | 3.09 | 3.08 | 3.11 |
| `ThumbnailBuffer` `Large`（→2048） | **12.0** | 12.8 | 14.0 | 27.5 | 45.6 | 72.0 | **58.3** | 59.5 | 63.2 | 69.8 |
| `Resize`（0.4 倍） | **0.41** | 0.45 | 0.59 | 1.26 | 1.85 | 1.05 | 0.83 | **0.79** | 0.99 | 1.26 |
| `Resize` `Large`（0.25 倍） | **7.9** | 8.4 | 9.0 | 10.4 | 18.6 | 64.5 | 33.6 | 18.0 | **10.9** | 13.0 |
| `SaveBuffer`（JPEG） | **0.24** | 0.28 | 0.40 | 0.77 | 0.79 | **1.07** | 1.08 | 1.15 | 1.38 | 1.75 |
| `ToImage` | **0.33** | 0.35 | 0.38 | 0.48 | 0.67 | **2.38** | 2.45 | 2.54 | 2.63 | 2.77 |

- 並列（24 ゴルーチン）では、すべての関数で 1 スレッドが最も速く、デフォルトの 1.2〜4.6 倍のスループットになります。`Thumbnail` / `ThumbnailBuffer` は計算を最大 2 スレッドに制限していますが、libvips はスレッド数に比例した大きさのバッファを使うため、`SetConcurrency(1)` でさらに速くなります（`Large` で 45.6 → 12.0 ms）。
- 逐次でも、デフォルトが最も速い関数はありませんでした。大きな画像をメモリ上で処理するときだけスレッドが多いほうが速くなりますが、論理コア数（24）より物理コア数程度（8）のほうが速くなります。
- 最適な値は CPU によって変わります。本番と同じマシンで `go test -run '^$' -bench Concurrency .` を実行して確かめてください。

#### キャッシュ

libvips は同じ引数の操作の結果をキャッシュします。キャッシュされた操作は入出力の画像を参照し続けるため、`Close` した画像のメモリがすぐには解放されないことがあります。

### 制限

- 画像をその場で書き換える操作（`draw_*` など）は使えません。
- 画像のメタデータは読めますが、書き換えられません。
- libvips のエラーメッセージはプロセス全体で共有されるバッファに入るため、同時に失敗した操作のメッセージが失われることがあります（エラー自体は返ります）。

## テスト

リサイズのテスト（`resize_test.go`）は、実際の用途に近い画像で、サイズだけでなく結果の画素も確かめます。画像はすべて Kodak テスト画像 kodim23（`testdata/photo.jpg`、768x512）から作っています。`testdata` の画像は `go run testdata/gen.go` で作り直せます。CMYK と巨大な画像はファイルが大きいため、テストの中で作ります。

| テスト | 画像 | 確かめること |
|---|---|---|
| `TestThumbnailSize` | 横長の写真、EXIF で縦向きになる写真 | 幅のみ・高さのみ・枠に収める・拡大・`size`・`crop` の出力サイズ、1〜1600 ピクセルの各サイズにちょうどなること、不正なサイズのエラー |
| `TestResizeQuality` | 写真 | 縮小（1/2、1/4）の結果が画素の平均とほぼ一致すること（PSNR 39 dB 以上。1 ピクセルずれると下回る）、2 倍に拡大して戻すと元の画像になること |
| `TestResizeAlpha` | 透明な背景に白い円の PNG（8bit / 16bit） | 縁が黒ずまないこと、円の面積が保たれること、`Resize` で半分にすると `ThumbnailImage` と同じ画素になること |
| `TestResizeOrientation` | EXIF の回転情報（6）付き JPEG | `Thumbnail` 系が正しい向きにすること、保存後も同じ向きで表示されること |
| `TestResizeColor` | CMYK の JPEG、Display P3 の JPEG | CMYK が sRGB になること、P3 のプロファイルが保たれること、`output_profile` で sRGB になること |
| `TestResize16Bit` | 16bit の PNG | `Resize` が 16bit を保ち、`Thumbnail` 系が 8bit にすること |
| `TestResizeAnimated` | 4 フレームの GIF / WebP | 全フレームがリサイズされ、保存しても 4 フレームのアニメーションであること |
| `TestResizeLarge` | 6144x4096 の JPEG | 大きな画像のサムネイルと縮小 |
| `TestCopyMemory` | 写真 | `Thumbnail` の結果と、`sequential` で読み込んだ画像のメモリ上のコピーを複数の形式で保存できること、`Thumbnail` の結果にスレッド数の指定が残らないこと |

## ベンチマーク

テストと同じ環境で実行します。

```sh
go test -run '^$' -bench . -benchmem .
go test -run '^$' -bench . -count 10 . > new.txt   # 変更前の old.txt と benchstat old.txt new.txt で比較
```

768x512 の Kodak テスト画像（kodim23）の JPEG を使い、`ns/op` に加えて入力画像の 1 秒あたりの画素数（`Mpx/s`）を出力します。毎回操作を実行するため、操作キャッシュは無効にします。

| ベンチマーク | 内容 |
|---|---|
| `BenchmarkLoadBuffer` | JPEG の読み込みと全画素のデコード |
| `BenchmarkThumbnailBuffer` / `BenchmarkThumbnailBufferParallel` | JPEG から幅 256 のサムネイルを作り JPEG に保存（並列版は複数ゴルーチンから） |
| `BenchmarkThumbnailBufferLarge` | 6144x4096 の JPEG から幅 256 / 2048 のサムネイルを作り JPEG に保存（`-parallel` は複数ゴルーチンから） |
| `BenchmarkResize` | メモリ上の画像の 0.4 倍への縮小（カーネルごと。`alpha` / `alpha-large` は透明な背景に白い円の 512x512 / 4096x4096 の画像） |
| `BenchmarkSaveBuffer` | メモリ上の画像の JPEG / PNG / WebP / GIF への保存（出力サイズ `out-bytes` も出力） |
| `BenchmarkFromImage` / `BenchmarkToImage` | `image.Image`（`*image.YCbCr`）との変換 |
| `BenchmarkConcurrency` | 主な関数を `SetConcurrency` のスレッド数ごと（1 / 2 / 4 / 8 / デフォルト）に、1 つずつ（`serial`）と複数ゴルーチンから（`parallel`）実行（スレッド数の推奨設定の根拠） |
