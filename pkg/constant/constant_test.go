package constant

import (
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

	b1 := []byte{51, 230, 133, 164, 1, 127, 131, 173}
	b := binary.LittleEndian.Uint64(b1)

	fmt.Printf("hexString is %x\n", b)
}
