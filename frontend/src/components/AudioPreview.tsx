/**
 * 音声のプレビュー。
 *
 * wavesurfer.js で実波形を描き、波形上のクリックでそのタイムスタンプへ飛べるようにする。
 * 再生制御（再生・一時停止・シーク・音量・ミュート・ループ）もここでまとめて扱う。
 */

import { useCallback, useEffect, useRef, useState } from "react";
import WaveSurfer from "wavesurfer.js";
import { fileURL, type Entry } from "../api/taggo";
import "./AudioPreview.css";

interface Props {
  entry: Entry;
}

/** 秒数を m:ss 形式にする。 */
function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds)) return "0:00";
  const total = Math.floor(seconds);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/** OS のテーマに合わせた波形の配色を返す。 */
function waveColors() {
  const styles = getComputedStyle(document.documentElement);
  return {
    wave: styles.getPropertyValue("--border-strong").trim() || "#cfc9c1",
    progress: styles.getPropertyValue("--accent").trim() || "#2f7d6e",
  };
}

export function AudioPreview({ entry }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const waveRef = useRef<WaveSurfer | null>(null);

  const [ready, setReady] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(entry.audio?.durationSec ?? 0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [loop, setLoop] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // ループ設定は finish ハンドラーから参照するので、最新値を ref で持つ。
  const loopRef = useRef(loop);
  loopRef.current = loop;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const colors = waveColors();
    const ws = WaveSurfer.create({
      container,
      height: 96,
      waveColor: colors.wave,
      progressColor: colors.progress,
      cursorColor: colors.progress,
      barWidth: 2,
      barGap: 1,
      barRadius: 2,
      normalize: true,
    });

    waveRef.current = ws;
    setReady(false);
    setError(null);
    setPlaying(false);
    setCurrent(0);

    ws.on("ready", () => {
      setReady(true);
      setDuration(ws.getDuration());
    });
    ws.on("timeupdate", (time: number) => setCurrent(time));
    ws.on("play", () => setPlaying(true));
    ws.on("pause", () => setPlaying(false));
    ws.on("finish", () => {
      if (loopRef.current) {
        void ws.play();
        return;
      }
      setPlaying(false);
    });
    ws.on("error", (err: Error) => {
      setError(String(err));
    });

    void ws.load(fileURL(entry.path));

    return () => {
      ws.destroy();
      waveRef.current = null;
    };
  }, [entry.path]);

  // 音量とミュートは再生中でも即座に反映する。
  useEffect(() => {
    waveRef.current?.setVolume(muted ? 0 : volume);
  }, [volume, muted]);

  const togglePlay = useCallback(() => {
    void waveRef.current?.playPause();
  }, []);

  const seekBy = useCallback((delta: number) => {
    const ws = waveRef.current;
    if (!ws) return;
    const total = ws.getDuration();
    if (total <= 0) return;
    ws.seekTo(Math.min(1, Math.max(0, (ws.getCurrentTime() + delta) / total)));
  }, []);

  const meta = entry.audio;

  return (
    <div className="audiopreview">
      <header className="audiopreview__header">
        <div className="audiopreview__cover" aria-hidden="true">
          {meta?.hasCoverArt ? "♫" : "♪"}
        </div>
        <div className="audiopreview__info">
          <h3>{meta?.title || entry.title}</h3>
          <p>
            {[meta?.artist, meta?.album, meta?.year].filter(Boolean).join(" · ") || "情報なし"}
          </p>
          {meta?.genre && <p className="audiopreview__genre">{meta.genre}</p>}
        </div>
      </header>

      {error ? (
        <div className="audiopreview__error">音声を読み込めませんでした: {error}</div>
      ) : (
        <div className="audiopreview__wave" ref={containerRef} />
      )}

      {!ready && !error && <p className="audiopreview__loading">波形を解析中…</p>}

      <div className="audiopreview__controls">
        <button className="btn" type="button" onClick={() => seekBy(-10)} disabled={!ready}>
          −10秒
        </button>
        <button className="btn btn--primary" type="button" onClick={togglePlay} disabled={!ready}>
          {playing ? "一時停止" : "再生"}
        </button>
        <button className="btn" type="button" onClick={() => seekBy(10)} disabled={!ready}>
          +10秒
        </button>

        <span className="audiopreview__time">
          {formatTime(current)} / {formatTime(duration)}
        </span>

        <button
          className={`btn${loop ? " btn--primary" : ""}`}
          type="button"
          onClick={() => setLoop((v) => !v)}
          title="ループ再生"
        >
          ループ
        </button>

        <button className="btn" type="button" onClick={() => setMuted((v) => !v)}>
          {muted ? "ミュート解除" : "ミュート"}
        </button>

        <label className="audiopreview__volume">
          音量
          <input
            type="range"
            min={0}
            max={1}
            step={0.01}
            value={muted ? 0 : volume}
            onChange={(e) => {
              setVolume(Number(e.target.value));
              setMuted(false);
            }}
          />
        </label>
      </div>

      <p className="audiopreview__hint">波形をクリックすると、その位置から再生します。</p>
    </div>
  );
}
