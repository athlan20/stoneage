package gateway

import (
	"bufio"
	"errors"
)

// errLineTooLong 表示一行超过了长度上限。
var errLineTooLong = errors.New("line too long")

// readLine 读一行（不含换行符），超过 max 字节就返回 errLineTooLong，
// 并且不会为超长的行分配无限内存。
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > max+1 || (len(buf) > max && err == bufio.ErrBufferFull) {
			return nil, errLineTooLong
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, err
		}
		return buf[:len(buf)-1], nil
	}
}
