//go:build !windows && !darwin

package cloudfile

import "io/fs"

// IsPlaceholder は常に false を返す。
//
// Linux では、クラウド同期クライアントの多くが FUSE のマウントとして実装されており、
// 「中身がまだ無い」ことをファイル属性で示す共通の仕組みが無い。判定できない以上、
// ここで嘘をつくより、フォルダ名からの確認（LooksLikeSyncFolder）に委ねる。
func IsPlaceholder(fs.FileInfo) bool { return false }
