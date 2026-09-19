import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

/**
 * taggo の内部エンドポイント（/taggo/file, /taggo/thumb）は Go 側のアセットハンドラーが返す。
 *
 * ところが `wails dev` は GET をすべて Vite の開発サーバーへプロキシし、
 * 開発サーバーが 404 か 405 を返したときにだけ Go のハンドラーへ委譲する。
 * Vite は未知のパスに対して SPA フォールバックで index.html を 200 で返すため、
 * 何もしないと画像やプレビューの配信要求が HTML で上書きされ、
 * ブラウザ側では「画像を表示できません」になってしまう。
 *
 * そこで、このプレフィックスだけは Vite が扱わずに 404 を返し、
 * Go 側へ処理が渡るようにする。本番ビルドは開発サーバーを経由しないため影響しない。
 */
function taggoBackendFallthrough(): Plugin {
  // Go 側の app.PathFile / app.PathThumb と対応する。
  const backendPrefix = "/taggo/";

  return {
    name: "taggo-backend-fallthrough",
    configureServer(server) {
      // ここで use すると Vite の内部ミドルウェア（SPA フォールバックを含む）より
      // 前に挿入されるため、確実に先回りできる。
      server.middlewares.use((req, res, next) => {
        if (req.url?.startsWith(backendPrefix)) {
          res.statusCode = 404;
          res.end();
          return;
        }
        next();
      });
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), taggoBackendFallthrough()],
});
