package constant

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/mr-tron/base58"
)

func ByteToHex(bytes []byte) string {
	return hex.EncodeToString(bytes)
}

func TestByteToHex(t *testing.T) {

	bytes := []byte{56, 252, 116, 8, 158, 223, 205, 95}
	hexString := ByteToHex(bytes)
	fmt.Println(hexString)

	b0 := binary.LittleEndian.Uint64(bytes)
	fmt.Printf("hexString is %x\n", b0)

	b1 := []byte{103, 108, 111, 98, 97, 108, 95, 118, 111, 108, 117, 109, 101,
		95, 97, 99, 99, 117, 109, 117, 108, 97, 116, 111, 114}
	s := base64.StdEncoding.EncodeToString(b1)

	b2 := []byte{189, 122, 159, 211, 84, 120, 180, 121, 31, 37, 255, 135, 212, 236, 66, 224, 147, 104, 184, 124, 175, 129, 60, 233, 222, 162, 146, 196, 61, 198, 23, 30, 199, 183, 40, 70, 79, 67, 179, 84, 175, 251, 165, 123, 216, 188, 189, 89, 166, 35, 109, 59, 20, 177, 139, 71, 228, 10, 200, 231, 56, 159, 224, 253}
	base58String := base58.Encode(b2)

	fmt.Println(s)
	fmt.Println(base58String)

	b3 := []byte{238, 66, 104, 97, 103, 199, 153, 246, 233, 217, 73, 206, 78, 147, 44, 70, 8, 246, 66, 251, 142, 246, 241, 9, 144, 40, 178, 143, 255, 209, 15, 65, 246, 114, 16, 150, 199, 18, 114, 165, 41, 186, 140, 2, 200, 33, 131, 0, 174, 240, 240, 228, 50, 114, 161, 245, 220, 75, 140, 249, 142, 124, 28, 47}
	base58String = base58.Encode(b3)
	fmt.Println(base58String)
}
