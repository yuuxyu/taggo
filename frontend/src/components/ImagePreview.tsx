/**
 * 画像のプレビュー。漫画ビューアのように、既定では画像全体が見えるよう
 * ウィンドウにフィットさせて表示する（縦長画像は高さ基準、横長画像は幅基準に
 * 自動で切り替わる object-fit: contain の挙動）。ズームやページ送りの操作パネルは、
 * DetailPanel が管理する「マウスを動かした間だけ出す」HUD として重ねる。
 *
 * フィットは「全体を表示」の 1 種類だけにしている。高さ基準のフィットは、
 * 縦長画像では全体表示と同じ結果になり、横長画像では幅がはみ出して横スクロールが
 * 要るだけなので、「はみ出さずに見せる」という目的には要らない。大きく見たいときは
 * 倍率指定（＋ / 原寸 / Ctrl+ホイール）で足りる。
 *
 * 操作の割り当ては次のとおり。
 *  - ホイール          … 前後のファイルへページ送り（タグ編集中は送らない）
 *  - Ctrl + ホイール   … カーソル位置を軸にした拡大・縮小
 *  - ドラッグ          … はみ出しているときの画像の移動
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  ArrowsPointingOutIcon,
  MinusIcon,
  PlusIcon,
  Square2StackIcon,
} from "@heroicons/react/20/solid";
import { fileURL, type Entry } from "../api/taggo";
import { Button } from "./Button";
import { Pager } from "./Pager";

interface Props {
  entry: Entry;
  /** true の間だけ操作 HUD を表示する。DetailPanel がマウス移動から判定する。 */
  uiVisible: boolean;
  /** 前後のファイルへの移動。種類を問わず一覧の並び順で動く。 */
  onNavigate: (direction: 1 | -1) => void;
  /**
   * false の間はホイールでのページ送りをしない。タグ編集欄が出ている間に
   * ホイールを回して、編集中のファイルから意図せず離れてしまうのを防ぐ。
   */
  wheelNavigation: boolean;
  /** 一覧での現在位置（0 始まり）。-1 ならページ送り UI を出さない。 */
  index: number;
  total: number;
}

/** 倍率の刻み。1 が原寸。 */
const ZOOM_STEPS = [0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4];
/** ホイール 1 ジェスチャーにつき 1 ページだけ送るためのクールダウン。 */
export const WHEEL_COOLDOWN_MS = 350;
/** 実際の倍率が刻みとほぼ同じとき、同じ値へ「動かない」のを避けるための許容差。 */
const ZOOM_EPSILON = 0.005;

type ZoomMode = "fit-contain" | number;

/** 拡大の軸にするため、カーソルが画像のどこを指していたかを覚えておく。 */
interface ZoomAnchor {
  clientX: number;
  clientY: number;
  /** 画像内の相対位置（0..1）。 */
  fx: number;
  fy: number;
}

/**
 * 画像が実際に描かれている矩形を返す。
 *
 * 「全体を表示」では img 要素はステージいっぱいで、その内側に object-fit: contain で
 * レターボックス表示されるため、要素の矩形と実際の絵の矩形が一致しない。
 * 倍率指定のときは要素＝絵なので、同じ計算でそのまま要素の矩形が返る。
 */
function paintedRect(img: HTMLImageElement): DOMRect {
  const r = img.getBoundingClientRect();
  const { naturalWidth: nw, naturalHeight: nh } = img;
  if (!nw || !nh) return r;
  const scale = Math.min(r.width / nw, r.height / nh);
  const w = nw * scale;
  const h = nh * scale;
  return new DOMRect(r.left + (r.width - w) / 2, r.top + (r.height - h) / 2, w, h);
}

/**
 * いま画面に出ている倍率を返す。「全体を表示」では画像の向きとウィンドウの
 * 大きさで倍率が決まるため、実際に描かれている幅と原寸の比から求める。
 */
function currentScale(img: HTMLImageElement | null): number {
  if (!img?.naturalWidth) return 1;
  return paintedRect(img).width / img.naturalWidth;
}

/**
 * base のひとつ上（direction: 1）またはひとつ下（-1）の刻みを返す。
 *
 * 「全体を表示」からの拡大・縮小でも段差が大きくならないよう、比較の基準には
 * 実際の倍率を使う。たとえば全体表示が 90% なら、拡大は 150% ではなく 100% へ動く。
 * その向きにもう刻みが無いときは倍率を変えない（縮小のつもりで拡大してしまう、
 * といった逆向きの動きを避ける）。
 */
function steppedZoom(base: number, direction: 1 | -1): number | null {
  if (direction === 1) {
    return ZOOM_STEPS.find((z) => z > base + ZOOM_EPSILON) ?? null;
  }
  const smaller = ZOOM_STEPS.filter((z) => z < base - ZOOM_EPSILON);
  return smaller.length > 0 ? smaller[smaller.length - 1] : null;
}

export function ImagePreview({
  entry,
  uiVisible,
  onNavigate,
  wheelNavigation,
  index,
  total,
}: Props) {
  // 既定は「全体を表示」。縦長画像は高さが、横長画像は幅が自動でウィンドウに
  // 合うため、画像の向きによらず全体が常に見える（object-fit: contain の性質）。
  const [zoom, setZoom] = useState<ZoomMode>("fit-contain");
  const [failed, setFailed] = useState(false);
  // はみ出していて、ドラッグで動かせる状態かどうか。カーソルの形に使う。
  const [pannable, setPannable] = useState(false);
  const [panning, setPanning] = useState(false);
  // 倍率指定の基準にする原寸の幅。メタデータから分からない画像もあるので、
  // 読み込めた時点で実際の幅を控えておく。
  const [naturalWidth, setNaturalWidth] = useState(0);
  const stageRef = useRef<HTMLDivElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const anchorRef = useRef<ZoomAnchor | null>(null);

  // 別の画像へ切り替えたら表示状態を初期化する。
  useEffect(() => {
    setZoom("fit-contain");
    setFailed(false);
    setNaturalWidth(0);
  }, [entry.path]);

  // ホイールのリスナーから呼ぶので、step 自体は付け替えが起きないよう
  // 依存を持たせない。いまの倍率は ref 経由で読む。
  const zoomRef = useRef<ZoomMode>(zoom);
  zoomRef.current = zoom;

  const step = useCallback((direction: 1 | -1) => {
    const current = zoomRef.current;
    const base = typeof current === "number" ? current : currentScale(imgRef.current);
    const next = steppedZoom(base, direction);
    if (next !== null) setZoom(next);
  }, []);

  /** いまはみ出しているかを見て、ドラッグ可否を更新する。 */
  const syncPannable = useCallback(() => {
    const el = stageRef.current;
    if (!el) return;
    setPannable(el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight);
  }, []);

  // 倍率が変わった直後に、カーソルが指していた点を同じ位置へ戻す。
  // React の再描画を待つ必要があるので、レイアウト確定後に補正する。
  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    anchorRef.current = null;
    const el = stageRef.current;
    const img = imgRef.current;
    if (el && img && anchor) {
      const r = paintedRect(img);
      el.scrollLeft += r.left + anchor.fx * r.width - anchor.clientX;
      el.scrollTop += r.top + anchor.fy * r.height - anchor.clientY;
    }
    syncPannable();
  }, [zoom, syncPannable]);

  // ウィンドウの大きさが変わると、はみ出しの有無も変わる。
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const observer = new ResizeObserver(syncPannable);
    observer.observe(el);
    return () => observer.disconnect();
  }, [syncPannable]);

  // ホイールは前後の画像へのページ送り、Ctrl + ホイールは拡大・縮小に割り当てる。
  // React の onWheel は既定で passive 登録されて preventDefault が効かないため、
  // ネイティブのリスナーを使う。
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;

    let cooling = false;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      if (e.deltaY === 0) return;

      if (e.ctrlKey) {
        // 拡大の軸はカーソルが指している点。倍率が変わったあとに同じ点が
        // 同じ位置へ来るよう、補正に使う情報を残す。
        const img = imgRef.current;
        if (img) {
          const r = paintedRect(img);
          anchorRef.current = {
            clientX: e.clientX,
            clientY: e.clientY,
            fx: r.width > 0 ? Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)) : 0.5,
            fy: r.height > 0 ? Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)) : 0.5,
          };
        }
        step(e.deltaY > 0 ? -1 : 1);
        return;
      }

      if (!wheelNavigation || cooling) return;
      cooling = true;
      window.setTimeout(() => {
        cooling = false;
      }, WHEEL_COOLDOWN_MS);
      onNavigate(e.deltaY > 0 ? 1 : -1);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [onNavigate, step, wheelNavigation]);

  // はみ出しているときは、ドラッグで画像を動かせるようにする。
  const dragRef = useRef<{ x: number; y: number; left: number; top: number } | null>(null);

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    const el = stageRef.current;
    if (!el || e.button !== 0 || !pannable) return;
    dragRef.current = { x: e.clientX, y: e.clientY, left: el.scrollLeft, top: el.scrollTop };
    el.setPointerCapture(e.pointerId);
    setPanning(true);
  };

  const onPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    const el = stageRef.current;
    if (!drag || !el) return;
    el.scrollLeft = drag.left - (e.clientX - drag.x);
    el.scrollTop = drag.top - (e.clientY - drag.y);
  };

  const endPan = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current) return;
    dragRef.current = null;
    setPanning(false);
    stageRef.current?.releasePointerCapture(e.pointerId);
  };

  const meta = entry.image;
  const hasMeta = Boolean(meta?.width || meta?.taken || meta?.make || meta?.model || meta?.lens);
  // 原寸が分からないうちは幅を指定せず、画像そのものの大きさに任せる。
  const baseWidth = meta?.width || naturalWidth;

  // 表示方式の切り替えボタン。いま選ばれているものが分かるよう、
  // 選択中は押し込んだ見た目（hud）にする。
  const modeVariant = (active: boolean) => (active ? "hud" : "hudGhost");

  return (
    // HUD はスクロールしないこの外側の箱に対して配置する。スクロールする側に
    // 置くと、拡大してずらしたときに HUD まで一緒に流れてしまう。
    <div className="relative size-full bg-black">
      <div
        ref={stageRef}
        /* 画像は縦横とも中央に置く。「全体を表示」は object-fit が中央寄せするので、
           倍率指定でウィンドウより小さくなったときに上へ張り付くと、拡大・縮小の
           連続感が途切れてしまう。
           中央寄せに safe を付けているのは、はみ出したときに中央寄せのままだと
           先頭側（上・左）がスクロールで届かなくなるため。safe なら、はみ出した
           ときだけ先頭揃えに切り替わる。 */
        className={`flex size-full touch-none items-center-safe justify-center-safe overflow-auto select-none ${
          panning ? "cursor-grabbing" : pannable ? "cursor-grab" : ""
        }`}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endPan}
        onPointerCancel={endPan}
      >
        {failed ? (
          <div className="p-10 text-white/70">画像を表示できませんでした</div>
        ) : (
          <img
            ref={imgRef}
            /* Preflight の img { max-width: 100% } は倍率指定の邪魔になるので外す。
               縮まないようにしているのは、flex アイテムの既定（flex-shrink: 1）だと
               ウィンドウより大きい倍率を指定しても縮められてしまうため。 */
            className={`max-w-none shrink-0 ${
              zoom === "fit-contain" ? "size-full object-contain" : ""
            }`}
            src={fileURL(entry.path)}
            alt={entry.title}
            draggable={false}
            style={
              typeof zoom === "number" && baseWidth > 0
                ? { width: `${baseWidth * zoom}px` }
                : undefined
            }
            onLoad={(e) => {
              setNaturalWidth(e.currentTarget.naturalWidth);
              syncPannable();
            }}
            onError={() => setFailed(true)}
          />
        )}
      </div>

      {/* ---- 下部の操作 HUD ----
          マウスが動いた間だけ表示する。常に画像の上に乗るので、
          背景の画像が何色でも読めるよう暗いグラデーションを敷く。
          外枠はクリックを受け取らない。受け取ってしまうと、拡大時に画面の
          いちばん下へ出る横スクロールバーを掴めなくなるため。 */}
      <div
        className={`pointer-events-none absolute inset-x-0 bottom-0 z-10 flex flex-col gap-1.5 bg-linear-to-t from-black/70 to-transparent px-5 pt-5 pb-4 text-left text-white transition-opacity duration-300 ${
          uiVisible ? "opacity-100" : "opacity-0"
        }`}
      >
        {/* 撮影情報は操作の手前に置く。ボタンの下に置くと、Markdown や音声の
            プレビューに対してページ送りの位置がその行のぶんだけ上へずれてしまう。 */}
        {hasMeta && (
          <dl
            className={`m-0 flex flex-wrap gap-x-4.5 gap-y-1 p-0 text-xs text-white/70 ${
              uiVisible ? "pointer-events-auto" : ""
            }`}
          >
            {meta?.width ? (
              <>
                <dt className="mr-1 after:content-['：']">解像度</dt>
                <dd className="m-0 text-white">
                  {meta.width} × {meta.height} px
                </dd>
              </>
            ) : null}
            {meta?.taken && (
              <>
                <dt className="mr-1 after:content-['：']">撮影日時</dt>
                <dd className="m-0 text-white">{meta.taken}</dd>
              </>
            )}
            {(meta?.make || meta?.model) && (
              <>
                <dt className="mr-1 after:content-['：']">機器</dt>
                <dd className="m-0 text-white">
                  {[meta.make, meta.model].filter(Boolean).join(" ")}
                </dd>
              </>
            )}
            {meta?.lens && (
              <>
                <dt className="mr-1 after:content-['：']">レンズ</dt>
                <dd className="m-0 text-white">{meta.lens}</dd>
              </>
            )}
          </dl>
        )}

        <div
          className={`flex flex-wrap items-center justify-between gap-2.5 ${
            uiVisible ? "pointer-events-auto" : ""
          }`}
        >
          <div className="flex flex-wrap items-center gap-1.5">
            <Button variant="hudGhost" onClick={() => step(-1)} title="縮小" aria-label="縮小">
              <MinusIcon className="size-4" />
            </Button>
            <span className="min-w-16 text-center text-xs tabular-nums text-white/70">
              {zoom === "fit-contain" ? "全体表示" : `${Math.round(zoom * 100)}%`}
            </span>
            <Button variant="hudGhost" onClick={() => step(1)} title="拡大" aria-label="拡大">
              <PlusIcon className="size-4" />
            </Button>
            <Button
              variant={modeVariant(zoom === "fit-contain")}
              aria-pressed={zoom === "fit-contain"}
              onClick={() => setZoom("fit-contain")}
            >
              <ArrowsPointingOutIcon className="size-4" aria-hidden="true" />
              全体を表示
            </Button>
            <Button
              variant={modeVariant(zoom === 1)}
              aria-pressed={zoom === 1}
              onClick={() => setZoom(1)}
            >
              <Square2StackIcon className="size-4" aria-hidden="true" />
              原寸
            </Button>
          </div>

          <Pager index={index} total={total} onNavigate={onNavigate} onImage />
        </div>
      </div>
    </div>
  );
}
