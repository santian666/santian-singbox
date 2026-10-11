package main

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"sync"
)

const (
	qrModeByte = 1 << 2
	qrECLM     = 0
)

type qrRSBlock struct {
	totalCount int
	dataCount  int
}

type qrBitBuffer struct {
	buffer []byte
	length int
}

type qrPNGBufferPool struct {
	pool sync.Pool
}

func (p *qrPNGBufferPool) Get() *png.EncoderBuffer {
	if value := p.pool.Get(); value != nil {
		return value.(*png.EncoderBuffer)
	}
	return &png.EncoderBuffer{}
}

func (p *qrPNGBufferPool) Put(buffer *png.EncoderBuffer) {
	p.pool.Put(buffer)
}

func (b *qrBitBuffer) put(num int, length int) {
	for i := 0; i < length; i++ {
		b.putBit(((num >> (length - i - 1)) & 1) == 1)
	}
}

func (b *qrBitBuffer) putBit(bit bool) {
	bufIndex := b.length / 8
	if len(b.buffer) <= bufIndex {
		b.buffer = append(b.buffer, 0)
	}
	if bit {
		b.buffer[bufIndex] |= 0x80 >> uint(b.length%8)
	}
	b.length++
}

type qrCode struct {
	typeNumber  int
	moduleCount int
	modules     [][]int8
	data        []byte
	dataCache   []byte
}

func qrPNG(text string) ([]byte, error) {
	var out bytes.Buffer
	if err := writeQRPNG(&out, text); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeQRPNG(writer io.Writer, text string) error {
	qr, err := makeQRCode([]byte(text))
	if err != nil {
		return err
	}
	const cell = 4
	const quiet = 4
	size := (qr.moduleCount + quiet*2) * cell
	img := image.NewGray(image.Rect(0, 0, size, size))
	for index := range img.Pix {
		img.Pix[index] = 0xff
	}
	for row := 0; row < qr.moduleCount; row++ {
		for col := 0; col < qr.moduleCount; col++ {
			if qr.modules[row][col] != 1 {
				continue
			}
			x0 := (col + quiet) * cell
			y0 := (row + quiet) * cell
			for y := y0; y < y0+cell; y++ {
				offset := y * img.Stride
				for x := x0; x < x0+cell; x++ {
					img.Pix[offset+x] = 0
				}
			}
		}
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed, BufferPool: qrPNGEncodePool}
	return encoder.Encode(writer, img)
}

func makeQRCode(data []byte) (*qrCode, error) {
	for version := 1; version <= 40; version++ {
		if qrDataFits(version, len(data)) {
			qr := &qrCode{typeNumber: version, moduleCount: version*4 + 17, data: data}
			mask := qr.bestMaskPattern()
			if err := qr.make(false, mask); err != nil {
				return nil, err
			}
			return qr, nil
		}
	}
	return nil, errors.New("协议链接太长，无法生成二维码")
}

func qrDataFits(typeNumber int, dataBytes int) bool {
	totalDataCount := 0
	for _, block := range qrRSBlocks(typeNumber) {
		totalDataCount += block.dataCount
	}
	return 4+qrLengthInBits(typeNumber)+dataBytes*8 <= totalDataCount*8
}

func (q *qrCode) make(test bool, maskPattern int) error {
	q.moduleCount = q.typeNumber*4 + 17
	q.modules = make([][]int8, q.moduleCount)
	for row := range q.modules {
		q.modules[row] = make([]int8, q.moduleCount)
		for col := range q.modules[row] {
			q.modules[row][col] = -1
		}
	}
	q.setupPositionProbePattern(0, 0)
	q.setupPositionProbePattern(q.moduleCount-7, 0)
	q.setupPositionProbePattern(0, q.moduleCount-7)
	q.setupPositionAdjustPattern()
	q.setupTimingPattern()
	q.setupTypeInfo(test, maskPattern)
	if q.typeNumber >= 7 {
		q.setupTypeNumber(test)
	}
	if q.dataCache == nil {
		cache, err := createQRData(q.typeNumber, q.data)
		if err != nil {
			return err
		}
		q.dataCache = cache
	}
	q.mapData(q.dataCache, maskPattern)
	return nil
}

func (q *qrCode) setupPositionProbePattern(row, col int) {
	for r := -1; r <= 7; r++ {
		if row+r <= -1 || q.moduleCount <= row+r {
			continue
		}
		for c := -1; c <= 7; c++ {
			if col+c <= -1 || q.moduleCount <= col+c {
				continue
			}
			dark := (0 <= r && r <= 6 && (c == 0 || c == 6)) ||
				(0 <= c && c <= 6 && (r == 0 || r == 6)) ||
				(2 <= r && r <= 4 && 2 <= c && c <= 4)
			q.set(row+r, col+c, dark)
		}
	}
}

func (q *qrCode) setupTimingPattern() {
	for r := 8; r < q.moduleCount-8; r++ {
		if q.modules[r][6] == -1 {
			q.set(r, 6, r%2 == 0)
		}
	}
	for c := 8; c < q.moduleCount-8; c++ {
		if q.modules[6][c] == -1 {
			q.set(6, c, c%2 == 0)
		}
	}
}

func (q *qrCode) setupPositionAdjustPattern() {
	pos := qrPatternPosition[q.typeNumber-1]
	for _, row := range pos {
		for _, col := range pos {
			if q.modules[row][col] != -1 {
				continue
			}
			for r := -2; r <= 2; r++ {
				for c := -2; c <= 2; c++ {
					q.set(row+r, col+c, r == -2 || r == 2 || c == -2 || c == 2 || (r == 0 && c == 0))
				}
			}
		}
	}
}

func (q *qrCode) setupTypeNumber(test bool) {
	bits := qrBCHTypeNumber(q.typeNumber)
	for i := 0; i < 18; i++ {
		dark := !test && ((bits>>uint(i))&1) == 1
		q.set(i/3, i%3+q.moduleCount-8-3, dark)
		q.set(i%3+q.moduleCount-8-3, i/3, dark)
	}
}

func (q *qrCode) setupTypeInfo(test bool, maskPattern int) {
	data := (qrECLM << 3) | maskPattern
	bits := qrBCHTypeInfo(data)
	for i := 0; i < 15; i++ {
		dark := !test && ((bits>>uint(i))&1) == 1
		if i < 6 {
			q.set(i, 8, dark)
		} else if i < 8 {
			q.set(i+1, 8, dark)
		} else {
			q.set(q.moduleCount-15+i, 8, dark)
		}
		if i < 8 {
			q.set(8, q.moduleCount-i-1, dark)
		} else if i < 9 {
			q.set(8, 15-i, dark)
		} else {
			q.set(8, 15-i-1, dark)
		}
	}
	q.set(q.moduleCount-8, 8, !test)
}

func (q *qrCode) mapData(data []byte, maskPattern int) {
	inc := -1
	row := q.moduleCount - 1
	bitIndex := 7
	byteIndex := 0
	mask := qrMaskFunc(maskPattern)
	for col := q.moduleCount - 1; col > 0; col -= 2 {
		if col == 6 {
			col--
		}
		for {
			for c := 0; c < 2; c++ {
				if q.modules[row][col-c] != -1 {
					continue
				}
				dark := false
				if byteIndex < len(data) {
					dark = ((data[byteIndex] >> uint(bitIndex)) & 1) == 1
				}
				if mask(row, col-c) {
					dark = !dark
				}
				q.set(row, col-c, dark)
				bitIndex--
				if bitIndex == -1 {
					byteIndex++
					bitIndex = 7
				}
			}
			row += inc
			if row < 0 || q.moduleCount <= row {
				row -= inc
				inc = -inc
				break
			}
		}
	}
}

func (q *qrCode) bestMaskPattern() int {
	bestPattern := 0
	bestLost := 0
	for i := 0; i < 8; i++ {
		_ = q.make(true, i)
		lost := q.lostPoint()
		if i == 0 || lost < bestLost {
			bestLost = lost
			bestPattern = i
		}
	}
	q.dataCache = nil
	return bestPattern
}

func (q *qrCode) set(row, col int, dark bool) {
	if dark {
		q.modules[row][col] = 1
	} else {
		q.modules[row][col] = 0
	}
}

func createQRData(typeNumber int, data []byte) ([]byte, error) {
	blocks := qrRSBlocks(typeNumber)
	totalDataCount := 0
	for _, block := range blocks {
		totalDataCount += block.dataCount
	}
	var buffer qrBitBuffer
	buffer.put(qrModeByte, 4)
	buffer.put(len(data), qrLengthInBits(typeNumber))
	for _, item := range data {
		buffer.put(int(item), 8)
	}
	if buffer.length > totalDataCount*8 {
		return nil, errors.New("二维码容量不足")
	}
	if buffer.length+4 <= totalDataCount*8 {
		buffer.put(0, 4)
	}
	for buffer.length%8 != 0 {
		buffer.putBit(false)
	}
	for len(buffer.buffer) < totalDataCount {
		buffer.put(0xec, 8)
		if len(buffer.buffer) >= totalDataCount {
			break
		}
		buffer.put(0x11, 8)
	}
	return createQRBytes(buffer.buffer, blocks), nil
}

func createQRBytes(buffer []byte, blocks []qrRSBlock) []byte {
	offset := 0
	maxDC := 0
	maxEC := 0
	dcData := make([][]byte, len(blocks))
	ecData := make([][]byte, len(blocks))
	for i, block := range blocks {
		dcCount := block.dataCount
		ecCount := block.totalCount - dcCount
		maxDC = max(maxDC, dcCount)
		maxEC = max(maxEC, ecCount)
		dcData[i] = append([]byte{}, buffer[offset:offset+dcCount]...)
		offset += dcCount
		rsPoly := qrErrorCorrectPolynomial(ecCount)
		rawPoly := append([]byte{}, dcData[i]...)
		rawPoly = append(rawPoly, make([]byte, ecCount)...)
		mod := qrPolyMod(rawPoly, rsPoly)
		ecData[i] = make([]byte, ecCount)
		for j := range ecData[i] {
			modIndex := j + len(mod) - ecCount
			if modIndex >= 0 {
				ecData[i][j] = mod[modIndex]
			}
		}
	}
	totalCodeCount := 0
	for _, block := range blocks {
		totalCodeCount += block.totalCount
	}
	result := make([]byte, 0, totalCodeCount)
	for i := 0; i < maxDC; i++ {
		for r := range blocks {
			if i < len(dcData[r]) {
				result = append(result, dcData[r][i])
			}
		}
	}
	for i := 0; i < maxEC; i++ {
		for r := range blocks {
			if i < len(ecData[r]) {
				result = append(result, ecData[r][i])
			}
		}
	}
	return result
}

func qrRSBlocks(typeNumber int) []qrRSBlock {
	table := qrRSBlockTableM[typeNumber-1]
	blocks := make([]qrRSBlock, 0)
	for i := 0; i < len(table); i += 3 {
		count, total, data := table[i], table[i+1], table[i+2]
		for j := 0; j < count; j++ {
			blocks = append(blocks, qrRSBlock{totalCount: total, dataCount: data})
		}
	}
	return blocks
}

func qrLengthInBits(typeNumber int) int {
	if typeNumber < 10 {
		return 8
	}
	return 16
}

func qrErrorCorrectPolynomial(length int) []byte {
	if cached, ok := qrErrorPolynomialCache.Load(length); ok {
		return cached.([]byte)
	}
	poly := []byte{1}
	for i := 0; i < length; i++ {
		poly = qrPolyMultiply(poly, []byte{1, byte(qrGExp(i))})
	}
	actual, _ := qrErrorPolynomialCache.LoadOrStore(length, poly)
	return actual.([]byte)
}

func qrPolyMultiply(a, b []byte) []byte {
	result := make([]byte, len(a)+len(b)-1)
	for i := range a {
		for j := range b {
			if a[i] == 0 || b[j] == 0 {
				continue
			}
			result[i+j] ^= byte(qrGExp(qrGLog(int(a[i])) + qrGLog(int(b[j]))))
		}
	}
	return result
}

func qrPolyMod(data, divisor []byte) []byte {
	result := append([]byte{}, data...)
	for len(result) >= len(divisor) {
		if result[0] == 0 {
			result = result[1:]
			continue
		}
		ratio := qrGLog(int(result[0])) - qrGLog(int(divisor[0]))
		for i := range divisor {
			if divisor[i] != 0 {
				result[i] ^= byte(qrGExp(qrGLog(int(divisor[i])) + ratio))
			}
		}
		for len(result) > 0 && result[0] == 0 {
			result = result[1:]
		}
	}
	return result
}

func qrGLog(n int) int {
	if n < 1 {
		return 0
	}
	return qrLogTable[n]
}

func qrGExp(n int) int {
	for n < 0 {
		n += 255
	}
	for n >= 256 {
		n -= 255
	}
	return qrExpTable[n]
}

func qrBCHTypeInfo(data int) int {
	g15 := (1 << 10) | (1 << 8) | (1 << 5) | (1 << 4) | (1 << 2) | (1 << 1) | 1
	mask := (1 << 14) | (1 << 12) | (1 << 10) | (1 << 4) | (1 << 1)
	d := data << 10
	for qrBCHDigit(d)-qrBCHDigit(g15) >= 0 {
		d ^= g15 << uint(qrBCHDigit(d)-qrBCHDigit(g15))
	}
	return ((data << 10) | d) ^ mask
}

func qrBCHTypeNumber(data int) int {
	g18 := (1 << 12) | (1 << 11) | (1 << 10) | (1 << 9) | (1 << 8) | (1 << 5) | (1 << 2) | 1
	d := data << 12
	for qrBCHDigit(d)-qrBCHDigit(g18) >= 0 {
		d ^= g18 << uint(qrBCHDigit(d)-qrBCHDigit(g18))
	}
	return (data << 12) | d
}

func qrBCHDigit(data int) int {
	digit := 0
	for data != 0 {
		digit++
		data >>= 1
	}
	return digit
}

func qrMaskFunc(pattern int) func(int, int) bool {
	switch pattern {
	case 0:
		return func(i, j int) bool { return (i+j)%2 == 0 }
	case 1:
		return func(i, j int) bool { return i%2 == 0 }
	case 2:
		return func(i, j int) bool { return j%3 == 0 }
	case 3:
		return func(i, j int) bool { return (i+j)%3 == 0 }
	case 4:
		return func(i, j int) bool { return (i/2+j/3)%2 == 0 }
	case 5:
		return func(i, j int) bool { return (i*j)%2+(i*j)%3 == 0 }
	case 6:
		return func(i, j int) bool { return ((i*j)%2+(i*j)%3)%2 == 0 }
	default:
		return func(i, j int) bool { return ((i*j)%3+(i+j)%2)%2 == 0 }
	}
}

func (q *qrCode) lostPoint() int {
	lost := 0
	for row := 0; row < q.moduleCount; row++ {
		for col := 0; col < q.moduleCount; col++ {
			sameCount := 0
			dark := q.modules[row][col] == 1
			for r := -1; r <= 1; r++ {
				if row+r < 0 || q.moduleCount <= row+r {
					continue
				}
				for c := -1; c <= 1; c++ {
					if col+c < 0 || q.moduleCount <= col+c || (r == 0 && c == 0) {
						continue
					}
					if (q.modules[row+r][col+c] == 1) == dark {
						sameCount++
					}
				}
			}
			if sameCount > 5 {
				lost += 3 + sameCount - 5
			}
		}
	}
	for row := 0; row < q.moduleCount-1; row++ {
		for col := 0; col < q.moduleCount-1; col++ {
			count := 0
			if q.modules[row][col] == 1 {
				count++
			}
			if q.modules[row+1][col] == 1 {
				count++
			}
			if q.modules[row][col+1] == 1 {
				count++
			}
			if q.modules[row+1][col+1] == 1 {
				count++
			}
			if count == 0 || count == 4 {
				lost += 3
			}
		}
	}
	for row := 0; row < q.moduleCount; row++ {
		for col := 0; col < q.moduleCount-6; col++ {
			if q.modules[row][col] == 1 && q.modules[row][col+1] == 0 && q.modules[row][col+2] == 1 &&
				q.modules[row][col+3] == 1 && q.modules[row][col+4] == 1 && q.modules[row][col+5] == 0 &&
				q.modules[row][col+6] == 1 {
				lost += 40
			}
		}
	}
	for col := 0; col < q.moduleCount; col++ {
		for row := 0; row < q.moduleCount-6; row++ {
			if q.modules[row][col] == 1 && q.modules[row+1][col] == 0 && q.modules[row+2][col] == 1 &&
				q.modules[row+3][col] == 1 && q.modules[row+4][col] == 1 && q.modules[row+5][col] == 0 &&
				q.modules[row+6][col] == 1 {
				lost += 40
			}
		}
	}
	darkCount := 0
	for row := 0; row < q.moduleCount; row++ {
		for col := 0; col < q.moduleCount; col++ {
			if q.modules[row][col] == 1 {
				darkCount++
			}
		}
	}
	ratio := abs(100*darkCount/q.moduleCount/q.moduleCount - 50)
	lost += ratio / 5 * 10
	return lost
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var qrExpTable = func() [256]int {
	var exp [256]int
	for i := 0; i < 8; i++ {
		exp[i] = 1 << uint(i)
	}
	for i := 8; i < 256; i++ {
		exp[i] = exp[i-4] ^ exp[i-5] ^ exp[i-6] ^ exp[i-8]
	}
	return exp
}()

var qrLogTable = func() [256]int {
	var log [256]int
	for i := 0; i < 255; i++ {
		log[qrExpTable[i]] = i
	}
	return log
}()

var qrErrorPolynomialCache sync.Map

var qrPNGEncodePool = &qrPNGBufferPool{}

var qrPatternPosition = [][]int{
	{}, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
	{6, 30, 54}, {6, 32, 58}, {6, 34, 62}, {6, 26, 46, 66}, {6, 26, 48, 70}, {6, 26, 50, 74}, {6, 30, 54, 78}, {6, 30, 56, 82}, {6, 30, 58, 86}, {6, 34, 62, 90},
	{6, 28, 50, 72, 94}, {6, 26, 50, 74, 98}, {6, 30, 54, 78, 102}, {6, 28, 54, 80, 106}, {6, 32, 58, 84, 110}, {6, 30, 58, 86, 114}, {6, 34, 62, 90, 118}, {6, 26, 50, 74, 98, 122}, {6, 30, 54, 78, 102, 126}, {6, 26, 52, 78, 104, 130},
	{6, 30, 56, 82, 108, 134}, {6, 34, 60, 86, 112, 138}, {6, 30, 58, 86, 114, 142}, {6, 34, 62, 90, 118, 146}, {6, 30, 54, 78, 102, 126, 150}, {6, 24, 50, 76, 102, 128, 154}, {6, 28, 54, 80, 106, 132, 158}, {6, 32, 58, 84, 110, 136, 162}, {6, 26, 54, 82, 110, 138, 166}, {6, 30, 58, 86, 114, 142, 170},
}

var qrRSBlockTableM = [][]int{
	{1, 26, 16}, {1, 44, 28}, {1, 70, 44}, {2, 50, 32}, {2, 67, 43},
	{4, 43, 27}, {4, 49, 31}, {2, 60, 38, 2, 61, 39}, {3, 58, 36, 2, 59, 37}, {4, 69, 43, 1, 70, 44},
	{1, 80, 50, 4, 81, 51}, {6, 58, 36, 2, 59, 37}, {8, 59, 37, 1, 60, 38}, {4, 64, 40, 5, 65, 41}, {5, 65, 41, 5, 66, 42},
	{7, 73, 45, 3, 74, 46}, {10, 74, 46, 1, 75, 47}, {9, 69, 43, 4, 70, 44}, {3, 70, 44, 11, 71, 45}, {3, 67, 41, 13, 68, 42},
	{17, 68, 42}, {17, 74, 46}, {4, 75, 47, 14, 76, 48}, {6, 73, 45, 14, 74, 46}, {8, 75, 47, 13, 76, 48},
	{19, 74, 46, 4, 75, 47}, {22, 73, 45, 3, 74, 46}, {3, 73, 45, 23, 74, 46}, {21, 73, 45, 7, 74, 46}, {19, 75, 47, 10, 76, 48},
	{2, 74, 46, 29, 75, 47}, {10, 74, 46, 23, 75, 47}, {14, 74, 46, 21, 75, 47}, {14, 74, 46, 23, 75, 47}, {12, 75, 47, 26, 76, 48},
	{6, 75, 47, 34, 76, 48}, {29, 74, 46, 14, 75, 47}, {13, 74, 46, 32, 75, 47}, {40, 75, 47, 7, 76, 48}, {18, 75, 47, 31, 76, 48},
}
