package meta

import (
	"encoding/binary"
	"io"
	"os"
)

// MP3 の再生時間は、コンテナにその値を持つ欄が無いため自分で求める必要がある。
// 全フレームを走査すれば正確に出せるが、走査は起動時に全ファイルへ掛かるので、
// 最初のフレームヘッダーからビットレートを読み、音声部のサイズで割る概算にとどめる。
// 固定ビットレートなら正確で、可変ビットレートでも Xing/Info ヘッダーがあれば正確になる。

// mpegBitrates は MPEG1 Layer III のビットレート表（kbps）。
var mpegBitratesV1L3 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

// mpegBitratesV2L3 は MPEG2 / MPEG2.5 Layer III のビットレート表（kbps）。
var mpegBitratesV2L3 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}

// mpegSampleRates は MPEG バージョンごとのサンプリング周波数表（Hz）。
var mpegSampleRates = [4][4]int{
	{11025, 12000, 8000, 0},  // MPEG2.5
	{0, 0, 0, 0},             // 予約
	{22050, 24000, 16000, 0}, // MPEG2
	{44100, 48000, 32000, 0}, // MPEG1
}

// mp3Duration は MP3 の再生時間を秒で概算する。求められない場合は 0 を返す。
// audioStart には ID3v2 タグを読み飛ばした後のオフセットを渡す。
func mp3Duration(path string, audioStart int64) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0
	}
	audioBytes := info.Size() - audioStart
	if audioBytes <= 0 {
		return 0
	}

	if _, err := f.Seek(audioStart, io.SeekStart); err != nil {
		return 0
	}
	// 先頭フレームを探すのに十分な量だけ読む。Xing ヘッダーもこの範囲に収まる。
	buf := make([]byte, 8192)
	n, err := io.ReadFull(f, buf)
	if err != nil && n == 0 {
		return 0
	}
	buf = buf[:n]

	offset, header, ok := findFrameHeader(buf)
	if !ok {
		return 0
	}

	// Xing / Info ヘッダーがあれば、総フレーム数から正確に求められる。
	if frames, samplesPerFrame, sampleRate, ok := xingFrameCount(buf[offset:], header); ok && sampleRate > 0 {
		return float64(frames*samplesPerFrame) / float64(sampleRate)
	}

	bitrate := header.bitrateKbps * 1000
	if bitrate <= 0 {
		return 0
	}
	return float64(audioBytes) * 8 / float64(bitrate)
}

// frameHeader は MP3 フレームヘッダーから読み取った値。
type frameHeader struct {
	versionID   int // 0=MPEG2.5, 2=MPEG2, 3=MPEG1
	bitrateKbps int
	sampleRate  int
	channelMode int
}

// findFrameHeader は同期ワード（0xFFE）を探して最初の妥当なフレームヘッダーを返す。
func findFrameHeader(buf []byte) (int, frameHeader, bool) {
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] != 0xFF || buf[i+1]&0xE0 != 0xE0 {
			continue
		}
		h, ok := parseFrameHeader(buf[i : i+4])
		if ok {
			return i, h, true
		}
	}
	return 0, frameHeader{}, false
}

func parseFrameHeader(b []byte) (frameHeader, bool) {
	versionID := int(b[1]>>3) & 0x03
	layer := int(b[1]>>1) & 0x03
	bitrateIndex := int(b[2] >> 4)
	sampleIndex := int(b[2]>>2) & 0x03
	channelMode := int(b[3] >> 6)

	// taggo が扱うのは Layer III（layer == 1）のみ。
	if versionID == 1 || layer != 1 {
		return frameHeader{}, false
	}
	if bitrateIndex == 0 || bitrateIndex == 15 || sampleIndex == 3 {
		return frameHeader{}, false
	}

	bitrate := mpegBitratesV1L3[bitrateIndex]
	if versionID != 3 {
		bitrate = mpegBitratesV2L3[bitrateIndex]
	}
	return frameHeader{
		versionID:   versionID,
		bitrateKbps: bitrate,
		sampleRate:  mpegSampleRates[versionID][sampleIndex],
		channelMode: channelMode,
	}, true
}

// xingFrameCount は、先頭フレーム内の Xing / Info ヘッダーから総フレーム数を読む。
// 可変ビットレートの MP3 では、これが唯一の正確な再生時間の情報源になる。
func xingFrameCount(frame []byte, h frameHeader) (frames, samplesPerFrame, sampleRate int, ok bool) {
	// Xing ヘッダーの位置は、MPEG バージョンとチャンネルモードで決まる。
	offset := 4
	switch {
	case h.versionID == 3 && h.channelMode != 3: // MPEG1 ステレオ
		offset += 32
	case h.versionID == 3: // MPEG1 モノラル
		offset += 17
	case h.channelMode != 3: // MPEG2 / 2.5 ステレオ
		offset += 17
	default: // MPEG2 / 2.5 モノラル
		offset += 9
	}

	if offset+12 > len(frame) {
		return 0, 0, 0, false
	}
	tag := string(frame[offset : offset+4])
	if tag != "Xing" && tag != "Info" {
		return 0, 0, 0, false
	}

	flags := binary.BigEndian.Uint32(frame[offset+4 : offset+8])
	if flags&0x01 == 0 {
		return 0, 0, 0, false // フレーム数フィールドが無い
	}
	frames = int(binary.BigEndian.Uint32(frame[offset+8 : offset+12]))

	samplesPerFrame = 1152 // MPEG1 Layer III
	if h.versionID != 3 {
		samplesPerFrame = 576 // MPEG2 / 2.5 Layer III
	}
	return frames, samplesPerFrame, h.sampleRate, frames > 0
}
