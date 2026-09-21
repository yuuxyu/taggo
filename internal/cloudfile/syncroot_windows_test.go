package cloudfile

import "testing"

// 同期フォルダの中にあることの確認は、同期サービスが入った環境でしかできない。
// ここでは、ふつうのフォルダを同期フォルダと取り違えないことだけを確かめる。
func TestSyncRootProviderOutsideSyncRoot(t *testing.T) {
	if got := SyncRootProvider(t.TempDir()); got != "" {
		t.Fatalf("同期フォルダの外なのにサービス名が返った: %q", got)
	}
}

func TestSyncRootProviderMissingPath(t *testing.T) {
	if got := SyncRootProvider(`Z:\taggo\存在しないフォルダ`); got != "" {
		t.Fatalf("存在しないパスでサービス名が返った: %q", got)
	}
}
