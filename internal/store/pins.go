package store

import (
	"strings"
)

// SetPins はピン留めしたノートを、ピン留めした順のパスで差し替える。
// まだ読み込んでいないノートのパスが混ざっていてもよい。一覧に出すときに
// 読み込み済みのものだけを拾う。
func (s *Store) SetPins(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins = append([]string(nil), paths...)
}

// pinOrder は、ピン留めしたノートの小文字のパスから、ピン留めした順番を引く対応を返す。
// パスの大文字小文字は、Windows に合わせて区別しない。
func (s *Store) pinOrder() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order := make(map[string]int, len(s.pins))
	for i, path := range s.pins {
		key := strings.ToLower(path)
		if _, dup := order[key]; !dup {
			order[key] = i
		}
	}
	return order
}
