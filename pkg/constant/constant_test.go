package constant

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
)

func ByteToHex(bytes []byte) string {
	return hex.EncodeToString(bytes)
}

func TestByteToHex(t *testing.T) {

	bytes := []byte{102, 6, 61, 18, 1, 218, 235, 234}
	hexString := ByteToHex(bytes)
	fmt.Println(hexString)

	b0 := binary.LittleEndian.Uint64(bytes)
	fmt.Printf("hexString is %x\n", b0)

	b1 := []byte{103, 108, 111, 98, 97, 108, 95, 118, 111, 108, 117, 109, 101,
                  95, 97, 99, 99, 117, 109, 117, 108, 97, 116, 111, 114}
	s := base64.StdEncoding.EncodeToString(b1)

	fmt.Printf("hexString is %s\n", s)
}
