/**
 * キー操作（Ctrl + N / Ctrl + P）でカードへフォーカスを移したときに、フォーカスの印を確実に出す。
 *
 * ブラウザがフォーカスの印を出すかどうか（:focus-visible）は、直前の操作から推し量って決まる。
 * Chromium は Ctrl などを押しながらのキーをキーボード操作に数えないので、マウスでノートを開いて
 * Ctrl + N だけで移ると :focus-visible にならず、どのカードにフォーカスがあるか見えない
 * （本文に文字を打ったあとなら出るので、編集したかどうかで見え方が変わってしまう）。
 * そこで、キー操作でフォーカスを移すときは要素に印（data-key-focus）を付け、見た目はこの印でも出す。
 * 印はフォーカスが外れたら外す。
 */

/** フォーカスの印の見た目。Tab などで移ったとき（:focus-visible）と、focusByKey で移ったときに出す。 */
export const FOCUS_RING =
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent " +
  "data-key-focus:outline-2 data-key-focus:outline-offset-2 data-key-focus:outline-accent";

/** キー操作で el へフォーカスを移し、フォーカスの印を出す。 */
export function focusByKey(el: HTMLElement, options?: FocusOptions) {
  el.dataset.keyFocus = "";
  el.addEventListener("blur", () => delete el.dataset.keyFocus, { once: true });
  el.focus(options);
}
