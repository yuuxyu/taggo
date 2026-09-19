/**
 * 画像のプレビュー。漫画ビューアのように、既定では画像全体が見えるよう
 * ウィンドウにフィットさせて表示する（縦長画像は高さ基準、横長画像は幅基準に
 * 自動で切り替わる object-fit: contain の挙動）。ズームやページ送りの操作パネルは、
 * DetailPanel が管理する「マウスを動かした間だけ出す」HUD として重ねる。
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { fileURL, type Entry } from "../api/taggo";
import "./ImagePreview.css";

interface Props {
  entry: Entry;
  /** true の間だけ操作 HUD を表示する。DetailPanel がマウス移動から判定する。 */
  uiVisible: boolean;
  onNavigate: (direction: 1 | -1) => void;
  /** 画像一覧での現在位置（0 始まり）。-1 ならページ送り UI を出さない。 */
  imageIndex: number;
  imageTotal: number;
}

/** 倍率の刻み。1 が原寸。 */
const ZOOM_STEPS = [0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4];
/** ホイール 1 ジェスチャーにつき 1 ページだけ送るためのクールダウン。 */
const WHEEL_COOLDOWN_MS = 350;

type ZoomMode = "fit-height" | "fit-contain" | number;

export function ImagePreview({ entry, uiVisible, onNavigate, imageIndex, imageTotal }: Props) {
  // 既定は「全体を表示」。縦長画像は高さが、横長画像は幅が自動でウィンドウに
  // 合うため、画像の向きによらず全体が常に見える（object-fit: contain の性質）。
  const [zoom, setZoom] = useState<ZoomMode>("fit-contain");
  const [failed, setFailed] = useState(false);
  const stageRef = useRef<HTMLDivElement>(null);

  // 別の画像へ切り替えたら表示状態を初期化する。
  useEffect(() => {
    setZoom("fit-contain");
    setFailed(false);
  }, [entry.path]);

  const step = useCallback((direction: 1 | -1) => {
    setZoom((current) => {
      const base = typeof current === "number" ? current : 1;
      const index = ZOOM_STEPS.findIndex((z) => z >= base);
      const next = index + direction;
      if (next < 0) return ZOOM_STEPS[0];
      if (next >= ZOOM_STEPS.length) return ZOOM_STEPS[ZOOM_STEPS.length - 1];
      return ZOOM_STEPS[next];
    });
  }, []);

  // マウスホイールで前後の画像へ送る。React の onWheel は既定で passive
  // 登録されて preventDefault が効かないため、ネイティブのリスナーを使う。
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;

    let cooling = false;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      if (cooling || e.deltaY === 0) return;
      cooling = true;
      window.setTimeout(() => {
        cooling = false;
      }, WHEEL_COOLDOWN_MS);
      onNavigate(e.deltaY > 0 ? 1 : -1);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [onNavigate]);

  const meta = entry.image;
  const zoomClass =
    zoom === "fit-height" ? " is-fit-height" : zoom === "fit-contain" ? " is-fit-contain" : "";
  const hasMeta = Boolean(meta?.width || meta?.taken || meta?.make || meta?.model || meta?.lens);
  const hasPrev = imageIndex > 0;
  const hasNext = imageIndex >= 0 && imageIndex < imageTotal - 1;

  return (
    <div className="mangaview__stage" ref={stageRef}>
      {failed ? (
        <div className="mangaview__error">画像を表示できませんでした</div>
      ) : (
        <img
          className={`mangaview__img${zoomClass}`}
          src={fileURL(entry.path)}
          alt={entry.title}
          draggable={false}
          style={typeof zoom === "number" ? { width: `${(meta?.width ?? 0) * zoom}px` } : undefined}
          onError={() => setFailed(true)}
        />
      )}

      <div className={`mangaview__hud${uiVisible ? " is-visible" : ""}`}>
        <div className="mangaview__hud-row">
          <div className="mangaview__zoom">
            <button className="btn btn--ghost" type="button" onClick={() => step(-1)} title="縮小">
              −
            </button>
            <span className="mangaview__zoomlabel">
              {zoom === "fit-height"
                ? "高さに合わせる"
                : zoom === "fit-contain"
                  ? "全体表示"
                  : `${Math.round(zoom * 100)}%`}
            </span>
            <button className="btn btn--ghost" type="button" onClick={() => step(1)} title="拡大">
              ＋
            </button>
            <button className="btn btn--ghost" type="button" onClick={() => setZoom("fit-height")}>
              高さに合わせる
            </button>
            <button className="btn btn--ghost" type="button" onClick={() => setZoom("fit-contain")}>
              全体を表示
            </button>
            <button className="btn btn--ghost" type="button" onClick={() => setZoom(1)}>
              原寸
            </button>
          </div>

          {imageIndex >= 0 && (
            <div className="mangaview__pager">
              <button
                className="btn btn--ghost"
                type="button"
                disabled={!hasPrev}
                onClick={() => onNavigate(-1)}
              >
                ‹ 前へ
              </button>
              <span className="mangaview__pagecount">
                {imageIndex + 1} / {imageTotal}
              </span>
              <button
                className="btn btn--ghost"
                type="button"
                disabled={!hasNext}
                onClick={() => onNavigate(1)}
              >
                次へ ›
              </button>
            </div>
          )}
        </div>

        {hasMeta && (
          <dl className="mangaview__meta">
            {meta?.width ? (
              <>
                <dt>解像度</dt>
                <dd>
                  {meta.width} × {meta.height} px
                </dd>
              </>
            ) : null}
            {meta?.taken && (
              <>
                <dt>撮影日時</dt>
                <dd>{meta.taken}</dd>
              </>
            )}
            {(meta?.make || meta?.model) && (
              <>
                <dt>機器</dt>
                <dd>{[meta.make, meta.model].filter(Boolean).join(" ")}</dd>
              </>
            )}
            {meta?.lens && (
              <>
                <dt>レンズ</dt>
                <dd>{meta.lens}</dd>
              </>
            )}
          </dl>
        )}
      </div>
    </div>
  );
}
