/**
 * 画像のプレビュー。拡大・縮小とフィット表示を切り替えられる。
 * メタデータはカード側と同じ Entry から取り出すので、追加の読み込みは要らない。
 */

import { useCallback, useEffect, useState } from "react";
import { fileURL, type Entry } from "../api/taggo";
import "./ImagePreview.css";

interface Props {
  entry: Entry;
}

/** 倍率の刻み。1 が原寸。 */
const ZOOM_STEPS = [0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4];

export function ImagePreview({ entry }: Props) {
  // fit のときは表示領域に収める。数値のときはその倍率で原寸表示する。
  const [zoom, setZoom] = useState<"fit" | number>("fit");
  const [failed, setFailed] = useState(false);

  // 別の画像へ切り替えたら表示状態を初期化する。
  useEffect(() => {
    setZoom("fit");
    setFailed(false);
  }, [entry.path]);

  const step = useCallback(
    (direction: 1 | -1) => {
      setZoom((current) => {
        const base = current === "fit" ? 1 : current;
        const index = ZOOM_STEPS.findIndex((z) => z >= base);
        const next = index + direction;
        if (next < 0) return ZOOM_STEPS[0];
        if (next >= ZOOM_STEPS.length) return ZOOM_STEPS[ZOOM_STEPS.length - 1];
        return ZOOM_STEPS[next];
      });
    },
    [],
  );

  const meta = entry.image;

  return (
    <div className="imgpreview">
      <div className="imgpreview__stage">
        {failed ? (
          <div className="imgpreview__error">画像を表示できませんでした</div>
        ) : (
          <img
            className={`imgpreview__img${zoom === "fit" ? " is-fit" : ""}`}
            src={fileURL(entry.path)}
            alt={entry.title}
            draggable={false}
            style={zoom === "fit" ? undefined : { width: `${(meta?.width ?? 0) * zoom}px` }}
            onError={() => setFailed(true)}
          />
        )}
      </div>

      <div className="imgpreview__controls">
        <button className="btn" type="button" onClick={() => step(-1)} title="縮小">
          −
        </button>
        <span className="imgpreview__zoom">
          {zoom === "fit" ? "フィット" : `${Math.round(zoom * 100)}%`}
        </span>
        <button className="btn" type="button" onClick={() => step(1)} title="拡大">
          ＋
        </button>
        <button className="btn" type="button" onClick={() => setZoom("fit")}>
          全体を表示
        </button>
        <button className="btn" type="button" onClick={() => setZoom(1)}>
          原寸
        </button>
      </div>

      <dl className="imgpreview__meta">
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
    </div>
  );
}
