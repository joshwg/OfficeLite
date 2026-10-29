// BAHTTEXT: spelling an amount in Thai baht and satang words.

package text

import (
	"math"
	"strings"

	f "officelite/excel/formula"
)

// Thai number words.
var thaiDigits = [...]string{"ศูนย์", "หนึ่ง", "สอง", "สาม", "สี่", "ห้า", "หก", "เจ็ด", "แปด", "เก้า"}

const (
	thaiTen         = "สิบ"
	thaiTwenty      = "ยี่"  // prefix of สิบ for 20-29
	thaiOneAfterTen = "เอ็ด" // units digit 1 after tens: สิบเอ็ด
	thaiMillion     = "ล้าน"
	thaiBaht        = "บาท"
	thaiSatang      = "สตางค์"
	thaiExact       = "ถ้วน" // "exactly": no satang
	thaiMinus       = "ลบ"
)

var thaiPlaces = [...]string{"แสน", "หมื่น", "พัน", "ร้อย"} // 1e5 .. 1e2

// thaiBlock writes 1 <= n <= 999999 in Thai words.
func thaiBlock(b *strings.Builder, n int) {
	div := 100000
	for _, place := range thaiPlaces {
		if d := n / div; d > 0 {
			b.WriteString(thaiDigits[d])
			b.WriteString(place)
		}
		n %= div
		div /= 10
	}
	tens, ones := n/10, n%10
	switch {
	case tens == 2:
		b.WriteString(thaiTwenty)
		fallthrough
	case tens == 1:
		b.WriteString(thaiTen)
	case tens > 2:
		b.WriteString(thaiDigits[tens])
		b.WriteString(thaiTen)
	}
	switch {
	case tens > 0 && ones == 1:
		b.WriteString(thaiOneAfterTen)
	case ones > 0:
		b.WriteString(thaiDigits[ones])
	}
}

// bahtText implements BAHTTEXT(number), spelling an amount in Thai baht and satang, e.g. 1234 is
// หนึ่งพันสองร้อยสามสิบสี่บาทถ้วน. The amount is rounded to whole satang;
// amounts of a million and more repeat ล้าน per six-digit block.
func bahtText(_ *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	neg := x < 0
	cents := math.Floor(f.RoundDigits(math.Abs(x), 2, f.RoundHalf)*100 + 0.5)
	baht, satang := math.Floor(cents/100), int(math.Mod(cents, 100))
	var blocks []int // six-digit blocks, least significant first
	for baht > 0 {
		blocks = append(blocks, int(math.Mod(baht, 1e6)))
		baht = math.Floor(baht / 1e6)
	}
	var b strings.Builder
	if neg && (len(blocks) > 0 || satang > 0) {
		b.WriteString(thaiMinus)
	}
	switch {
	case len(blocks) > 0:
		for i := len(blocks) - 1; i >= 0; i-- {
			if blocks[i] > 0 {
				thaiBlock(&b, blocks[i])
			}
			if i > 0 {
				b.WriteString(thaiMillion)
			}
		}
		b.WriteString(thaiBaht)
	case satang == 0:
		b.WriteString(thaiDigits[0] + thaiBaht)
	}
	if satang == 0 {
		b.WriteString(thaiExact)
	} else {
		thaiBlock(&b, satang)
		b.WriteString(thaiSatang)
	}
	return f.String(b.String())
}
