package cloudfile

import "testing"

func TestIsGoogleDriveVolume(t *testing.T) {
	cases := []struct {
		name   string
		fsName string
		label  string
		want   bool
	}{
		{"Google ドライブの仮想ドライブ", "FAT32", "Google Drive", true},
		{"日本語のラベル", "FAT32", "Google ドライブ", true},
		{"大文字小文字を問わない", "fat32", "GOOGLE DRIVE", true},
		{"ふつうの USB メモリ", "FAT32", "USBDRIVE", false},
		{"ラベルを Google Drive にした NTFS", "NTFS", "Google Drive", false},
		{"システムドライブ", "NTFS", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isGoogleDriveVolume(c.fsName, c.label); got != c.want {
				t.Fatalf("判定が違う: got %v, want %v", got, c.want)
			}
		})
	}
}

// ふつうのフォルダ（NTFS 上）を Google ドライブと取り違えないこと。
func TestIsGoogleDriveStreamingOnLocalFolder(t *testing.T) {
	if IsGoogleDriveStreaming(t.TempDir()) {
		t.Fatal("ローカルのフォルダを Google ドライブと判定した")
	}
}
