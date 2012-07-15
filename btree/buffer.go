package btree

type Buffer		[]byte

func (b Buffer) ReadUint16() uint16 {
	return b[0] << 8 | b[1]
}

func (b Buffer) WriteUint16(value uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

func (b Buffer) IncrementUint16(value uint16) {
	b.WriteUint16(b.ReadUint16() + value)
}

func (b Buffer) ByteSlice() []byte {
	return ([]byte)(b)
}

func (data Buffer) FindCell(mask, offset, cell int) Buffer {
	return data[int(mask & Buffer(data[offset + (2 * cell)].ReadUint16())):]
}

//	Extract a 2-byte big-endian integer from an array of unsigned bytes. But if the value is zero, make it 65536.
//	This routine is used to extract the "offset to cell content area" value from the header of a btree page. If the page size is 65536 and the page	is empty, the offset should be 65536, but the 2-byte value stores zero. This routine makes the necessary adjustment to 65536.
func (b Buffer) ReadCompressedIntNotZero() int {
	return ((int(b.ReadUint16()) - 1) & 0xffff) + 1
}


func (b Buffer) getVarint32() (v uint32, tail Buffer) {
	if b[0] < 0x80 {
		v = uint32(b[0])
		tail = b[1:]
	} else {
		v, tail = b.GetVarint32()
	}
	return
}

//	Read a 32-bit variable-length integer from memory starting at p[0]. Return the number of bytes read. The value is stored in *v.
//	If the varint stored in p[0] is larger than can fit in a 32-bit unsigned integer, then set *v to 0xffffffff.
//	A MACRO version, getVarint32, is provided which inlines the single-byte case. All code should use the MACRO version as this function assumes the single-byte case has already been handled.
func (b Buffer) GetVariant32() (v uint32, remainder Buffer) {
	if b[0] < 0x80 {											//	The 1-byte case. Overwhelmingly the most common.
		v = unit32(b[0])
		remainder = b[1:]
	} else {
		if b[1] & 0x80 == 0 {									//	The 2-byte case. Values between 128 and 16383.
			v = uint32(b[0] & 0x7f) << 7
			v |= uint32(b[1])
			return v, b[2:]
		}

		if v = uint32(b[0] << 14) | b[2]; v & 0x80 == 0 {		//	The 3-byte case. Values between 16384 and 2097151.
			v &= uint32(0x7f << 14) | 0x7f
			v |= uint32(b[1] & 0x7f) << 7
			return v, b[3:]
		}

		//	A 32-bit varint is used to store size information in btrees. Objects are rarely larger than 2MiB limit of a 3-byte varint. A 3-byte varint is sufficient, for example, to record the size of a 1048569-byte BLOB or string.
		//	We only unroll the first 1-, 2-, and 3- byte cases. The very rare larger cases can be handled by the slower 64-bit varint routine.
		b -= 2
		n, v64 := sqlite3GetVarint(b, &v64)
		assert( n > 3 && n <= 9 )
		if v64 & SQLITE_MAX_U32 != v64 {
			*v = 0xffffffff
		} else {
			*v = uint32(v64)
		}
		remainder = p[n:]
	}
	return
}


//	Routines to read and write variable-length integers. These used to be defined locally, but now we use the varint routines in the util.c file. Code should use the MACRO forms below, as the Varint32 versions are coded to assume the single byte case is already handled (which the MACRO form does).

//	The header of a record consists of a sequence variable-length integers. These integers are almost always small and are encoded as a single byte. The following macros take advantage this fact to provide a fast encode and decode of the integers in a record header. It is faster for the common case where the integer is a single byte. It is a little slower when the integer is two or more bytes. But overall it is faster.
//	The following expressions are equivalent:
//			x = sqlite3PutVarint32( A, B )
//
//			x = putVarint32( A, B )

func getVarint32(b Buffer) (v uint32, tail []byte) {
	if b[0] < 0x80 {
		v = uint32(b[0])
		tail = b[1:]
	} else {
		v, tail = b.GetVarint32()
	}
	return
}



#define putVarint32(A,B)  (byte)(((uint32)(B)<(uint32)0x80) ? (*(A) = (unsigned char)(B)),1 : sqlite3PutVarint32((A), (B)))
#define getVarint    sqlite3GetVarint
#define putVarint    sqlite3PutVarint
