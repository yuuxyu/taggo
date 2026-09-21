package meta

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Ogg コンテナのページ操作。
//
// Ogg は「ページ」の連なりで、タグ（Vorbis Comment）は先頭側のヘッダー
// パケットに入っている。タグを書き換えるとそのパケットの長さが変わり、
// ページの切り直しとページ番号の振り直し、CRC の計算し直しが要るため、
// ここでページの読み書きをまとめて引き受ける。
//
// 仕様は RFC 3533（Ogg）と Vorbis I / Opus のマッピング。

const (
	oggCapture     = "OggS"
	oggHeaderBytes = 27
	// oggMaxSegments は 1 ページに入る最大セグメント数。
	oggMaxSegments = 255
	// oggFlagContinued はページ先頭が前ページから続くパケットであることを表す。
	oggFlagContinued = 0x01
	// oggFlagBOS は論理ストリームの最初のページであることを表す。
	oggFlagBOS = 0x02
)

// oggPage は Ogg のページ 1 枚。
type oggPage struct {
	flags   byte
	granule int64
	serial  uint32
	seq     uint32
	lacing  []byte
	body    []byte
}

// endsPacket は、このページの最後のパケットがページ内で完結しているかを返す。
func (p *oggPage) endsPacket() bool {
	return len(p.lacing) == 0 || p.lacing[len(p.lacing)-1] != 255
}

// encode はページをバイト列へ直す。CRC はここで計算する。
func (p *oggPage) encode() []byte {
	out := make([]byte, oggHeaderBytes+len(p.lacing)+len(p.body))
	copy(out, oggCapture)
	out[4] = 0 // バージョンは 0 固定
	out[5] = p.flags
	binary.LittleEndian.PutUint64(out[6:], uint64(p.granule))
	binary.LittleEndian.PutUint32(out[14:], p.serial)
	binary.LittleEndian.PutUint32(out[18:], p.seq)
	// CRC 欄（22..25）は 0 のまま計算し、あとで埋める。
	out[26] = byte(len(p.lacing))
	copy(out[oggHeaderBytes:], p.lacing)
	copy(out[oggHeaderBytes+len(p.lacing):], p.body)
	binary.LittleEndian.PutUint32(out[22:], oggCRC(out))
	return out
}

// readOggPage は次のページを 1 枚読む。ストリームの終わりでは io.EOF を返す。
func readOggPage(r *bufio.Reader) (*oggPage, error) {
	head := make([]byte, oggHeaderBytes)
	if _, err := io.ReadFull(r, head); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("Ogg ページの読み取りに失敗しました: %w", err)
	}
	if string(head[:4]) != oggCapture {
		return nil, errors.New("Ogg ページの先頭が壊れています")
	}

	page := &oggPage{
		flags:   head[5],
		granule: int64(binary.LittleEndian.Uint64(head[6:])),
		serial:  binary.LittleEndian.Uint32(head[14:]),
		seq:     binary.LittleEndian.Uint32(head[18:]),
		lacing:  make([]byte, head[26]),
	}
	if _, err := io.ReadFull(r, page.lacing); err != nil {
		return nil, fmt.Errorf("Ogg のセグメント表の読み取りに失敗しました: %w", err)
	}

	size := 0
	for _, l := range page.lacing {
		size += int(l)
	}
	page.body = make([]byte, size)
	if _, err := io.ReadFull(r, page.body); err != nil {
		return nil, fmt.Errorf("Ogg ページ本体の読み取りに失敗しました: %w", err)
	}
	return page, nil
}

// packets はページに含まれるパケットの断片を切り出す。
// ページ内で完結していない末尾の断片も、そのまま最後の要素として返る。
func (p *oggPage) packets() [][]byte {
	var out [][]byte
	off, start := 0, 0
	for _, l := range p.lacing {
		off += int(l)
		if l == 255 {
			continue // パケットは次のセグメントへ続く
		}
		out = append(out, p.body[start:off])
		start = off
	}
	if start < off {
		out = append(out, p.body[start:off])
	}
	return out
}

// oggStream は 1 つの論理ストリームの先頭部分を、パケット単位で読み出す。
type oggStream struct {
	src *bufio.Reader
	// serial は最初のページから決まる論理ストリーム番号。
	serial uint32
	// pages はここまでに読んだページ。ヘッダーを書き換えるときに使う。
	pages []*oggPage
	// partial は組み立て途中のパケット。
	partial []byte
	// queue は読み出し待ちの完成済みパケット。
	queue [][]byte
	// multiplexed は、途中で別の論理ストリームが現れたことを表す。
	multiplexed bool
}

func newOggStream(r io.Reader) *oggStream {
	return &oggStream{src: bufio.NewReaderSize(r, 64<<10)}
}

// nextPacket は次のパケットを返す。必要なだけページを読み進める。
func (s *oggStream) nextPacket() ([]byte, error) {
	for len(s.queue) == 0 {
		page, err := readOggPage(s.src)
		if err != nil {
			return nil, err
		}
		if len(s.pages) == 0 {
			s.serial = page.serial
		} else if page.serial != s.serial {
			// 多重化・連結されたストリーム。安全に書き換えられないので印を付ける。
			s.multiplexed = true
		}
		s.pages = append(s.pages, page)

		frags := page.packets()
		for i, frag := range frags {
			s.partial = append(s.partial, frag...)
			if i == len(frags)-1 && !page.endsPacket() {
				break // 次のページへ続く
			}
			s.queue = append(s.queue, s.partial)
			s.partial = nil
		}
	}

	pkt := s.queue[0]
	s.queue = s.queue[1:]
	return pkt, nil
}

// headerPagesDone は、ここまでに読んだページでヘッダーが終わっているかを返す。
// 終わっていれば、その次のページから音声データが始まる。
func (s *oggStream) headerPagesDone() bool {
	return len(s.queue) == 0 && len(s.partial) == 0 &&
		len(s.pages) > 0 && s.pages[len(s.pages)-1].endsPacket()
}

// packOggPages はパケット列をページへ詰める。
// granule はページの granule position で、ヘッダーのページでは 0 を使う。
func packOggPages(packets [][]byte, serial uint32, firstSeq uint32, bos bool, granule int64) []*oggPage {
	var pages []*oggPage
	page := &oggPage{serial: serial, seq: firstSeq, granule: granule}
	if bos {
		page.flags |= oggFlagBOS
	}

	// flush は今のページを閉じ、次のページを用意する。
	// パケットの途中で閉じたときだけ、次のページに「続き」の印を付ける。
	// ちょうどパケットの切れ目で 255 セグメントに達することもあるため、
	// 満杯かどうかではなく最後のセグメントの値で判断する。
	flush := func() {
		continued := !page.endsPacket()
		pages = append(pages, page)
		next := &oggPage{serial: serial, seq: page.seq + 1, granule: granule}
		if continued {
			next.flags |= oggFlagContinued
		}
		page = next
	}

	for _, pkt := range packets {
		// パケットは 255 バイトごとのセグメントに割る。
		// 最後のセグメントは 255 未満（0 を含む）で、そこがパケットの終わりを表す。
		off := 0
		for {
			seg := len(pkt) - off
			if seg > 255 {
				seg = 255
			}
			if len(page.lacing) == oggMaxSegments {
				flush()
			}
			page.lacing = append(page.lacing, byte(seg))
			page.body = append(page.body, pkt[off:off+seg]...)
			off += seg
			if seg < 255 {
				break
			}
		}
	}
	pages = append(pages, page)
	return pages
}

// oggCRCTable は Ogg の CRC-32（多項式 0x04c11db7・ビット反転なし）の表。
var oggCRCTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		crc := uint32(i) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

func oggCRC(b []byte) uint32 {
	var crc uint32
	for _, v := range b {
		crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^v]
	}
	return crc
}
