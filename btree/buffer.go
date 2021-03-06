package btree

type Buffer		[]byte

func (b Buffer) ReadUint16() uint16 {
	return b[0] << 8 | b[1]
}

func (b Buffer) WriteUint16(v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

func (b Buffer) IncrementUint16(value uint16) {
	b.WriteUint16(b.ReadUint16() + value)
}

func (b Buffer) ReadUint32() uint32 {
	return b[0] << 24 | b[1] << 16 | b[2] << 8 | b[3]
}

func (b Buffer) WriteUint32(v uint32) {
	p[0] = byte(v >> 24)
	p[1] = byte(v >> 16)
	p[2] = byte(v >> 8)
	p[3] = byte(v)
}

func (b Buffer) IncrementUint32(value uint16) {
	b.WriteUint32(b.readUint32() + value)
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


//	Routines to read and write variable-length integers.
//	The header of a record consists of a sequence variable-length integers. These integers are almost always small and are encoded as a single byte.
//	The variable-length integer encoding is as follows:
//		KEY:		A = 0xxxxxxx		7 bits of data and one flag bit
//					B = 1xxxxxxx		7 bits of data and one flag bit
//					C = xxxxxxxx		8 bits of data
//
//		 7 bits - A
//		14 bits - BA
//		21 bits - BBA
//		28 bits - BBBA
//		35 bits - BBBBA
//		42 bits - BBBBBA
//		49 bits - BBBBBBA
//		56 bits - BBBBBBBA
//		64 bits - BBBBBBBBC

const (
	SLOT_2_0	= 0x001fc07f		//	0x7f << 14 | 0x7f
	SLOT_4_2_0	= 0xf01fc07f		//	0x7f << 28 | 0x7f << 14 | 0x7f
)

//	Read a 64-bit variable-length integer from memory starting at p[0].
func (p Buffer) ReadVarint64() (v uint64, remainder []byte) {
	var count	int
	if v = uint64(p[0]); v & 0x80 == 0 {
		count = 1
	} else if b := uint64(p[1]); b & 0x80 == 0 {
		count = 2
		v = (v & 0x7f) << 7 | b
	} else if v = v << 14 | p[2]; v & 0x80 == 0 {
		count = 3
		v = (v & SLOT_2_0) | (b & 0x7f) << 7
	} else {
		//	CSE1 from below
		v &= SLOT_2_0
		if b = b << 14 |= p[3]; b & 0x80 == 0 {
			//	b: p1 << 14 | p3 (unmasked)
			count = 4
			//	moved CSE1 up
			//	v &= (0x7f << 14) | 0x7f
			v = v << 7 | (b & SLOT_2_0)
		} else {
			//	v: p0 << 14 | p2 (masked)
			//	b: p1 << 14 | p3 (unmasked)
			//	1:save off p0 << 21 | p1 << 14 | p2 << 7 | p3 (masked)
			//	moved CSE1 up
			//	v &= (0x7f << 14) | 0x7f
			b &= SLOT_2_0
			s := v																//	s: p0 << 14 | p2 (masked)
			if v = v << 14 |= p[4]; v & 0x80 == 0 {								//	v: p0 << 28 | p2 << 14 | p4 (unmasked)
				count = 5
				//	we can skip these cause they were (effectively) done above in calc'ing s
				//	v &= 0x7f << 28 | 0x7f << 14 | 0x7f
				//	b &= 0x7f << 14 | 0x7f
				v = uint64(s >> 18) << 32 | (v | b << 7)
			} else {
				//	2:save off p0 << 21 | p1 << 14 | p2 << 7 | p3 (masked)
				s = s << 7 | b													//	s: p0 << 21 | p1 << 14 | p2 << 7 | p3 (masked)
				if b = b << 14 |= p[5]; b & 0x80 == 0 {							//	b: p1 << 28 | p3 << 14 | p5 (unmasked)
					count = 6
					//	we can skip this cause it was (effectively) done above in calc'ing s
					//	b &= 0x7f << 28 | 0x7f << 14 | 0x7f
					v &= SLOT_2_0
					v = (v << 7 | b) | uint64(s >> 18) << 32
				} else if v = v << 14 |= p[6]; v & 0x80 == 0 {						//	v: p2 << 28 | p4 << 14 | p6 (unmasked)
					count = 7
					v = (v & SLOT_4_2_0) | (b & SLOT_2_0) << 7 | uint64(s >> 11) << 32
				} else {
					//	CSE2 from below
					v &= SLOT_2_0
					if b = b << 14 | p[7]; b & 0x80 == 0 {					//	b: p3 << 28 | p5 << 14 | p7 (unmasked)
						count = 8
						//	moved CSE2 up
						//	v &= 0x7f << 14 | 0x7f
						v = (v << 7 | (b & SLOT_4_2_0)) | uint64(s >> 4) << 32
					} else {
						count = 9
						v = v << 15 | p[8]									//	v: p4 << 29 | p6 << 15 | p8 (unmasked)
						//	moved CSE2 up
						//	v &= 0x7f << 29 | 0x7f << 15 | 0xff
						s = s << 4 | (p[4] & 0x7f) >> 3
						v = (v | (b & SLOT_2_0) << 8) | uint64(s) << 32
					}
				}
			}
		}
	}
	return v, p[count:]
}

//	Read a 32-bit variable-length integer from memory starting at p[0]. Return the number of bytes read.
//	If the varint stored in p[0] is larger than can fit in a 32-bit unsigned integer, then return 0xffffffff.
func (b Buffer) ReadVarint32() (v uint32, remainder Buffer) {
	if b[0] < 0x80 {													//	The 1-byte case. Overwhelmingly the most common.
		return unit32(b[0]), b[1:]
	} else if b[1] & 0x80 == 0 {										//	The 2-byte case. Values between 128 and 16383.
		v = uint32(b[0] & 0x7f) << 7 | uint32(b[1])
		remainder = b[2:]
	} else if v = uint32(b[0] << 14) | b[2]; v & 0x80 == 0 {			//	The 3-byte case. Values between 16384 and 2097151.
		v = (v & uint32(0x7f << 14) | 0x7f) | int32(b[1] & 0x7f) << 7
		remainder = b[3:]
	} else {
		//	A 32-bit varint is used to store size information in btrees. Objects are rarely larger than 2MiB limit of a 3-byte varint. A 3-byte varint is sufficient, for example, to record the size of a 1048569-byte BLOB or string.
		//	We only unroll the first 1-, 2-, and 3- byte cases. The very rare larger cases can be handled by the slower 64-bit varint routine.
		if v64, buf := b.ReadVarint64(); v64 & SQLITE_MAX_U32 != v64 {
			v = 0xffffffff
		} else {
			v = uint32(v64)
		}
		remainder = buf
	}
	return
}

//	Write a 64-bit variable-length integer to memory starting at p[0]. The length of data write will be between 1 and 9 bytes. The number of bytes written is returned.
//	A variable-length integer consists of the lower 7 bits of each byte for all bytes that have the 8th bit set and one byte with the 8th bit clear. Except, if we get to the 9th byte, it stores the full 8 bits and is the last byte.
func (b Buffer) WriteVarint64(v uint64) (remainder Buffer) {
	if v & (uint64(0xff000000) << 32) {
		b[8] = byte(v)
		v >>= 8
		for i := 7; i >= 0; i-- {
			b[i] = byte((v & 0x7f) | 0x80)
			v >>= 7
		}
		remainder = b[9:]
	} else {
		buf := make(Buffer, 0, 10)
		for {
			buf = append(buf, byte((v & 0x7f) | 0x80))
			v >>= 7
			if v == 0 {
				break
			}
		}
		buf[0] &= 0x7f
		l := len(buf) - 1
		for i, j := 0, l; j >= 0; j-- {
			b[i] = buf[j]
			i++
		}
		remainder = b[l:]
	}
	return
}

func (b Buffer) WriteVarint32(v uint32) Buffer {
	if v < 0x80 {
		b[0] = byte(v)
		return b[1:]
	} else if v & ~0x3fff == 0 {
		p[0] = (byte)((v>>7) | 0x80)
		p[1] = (byte)(v & 0x7f)
		return b[2:]
	}
	return b.WriteVarint64(b, v)
}

//	Return the number of bytes that will be needed to store the given 64-bit integer.
func VarintLen(v uint64) (count int) {
	for count := 1; v != 0 && count < 9; count++ {
		v >>= 7
	}
	return
}

//	This function compares the two table rows or index records specified by {nKey1, pKey1} and pPKey2. It returns a negative, zero or positive integer if key1 is less than, equal to or greater than key2. The {nKey1, pKey1} key must be a blob created by th OP_MakeRecord opcode of the VDBE. The pPKey2 key must be a parsed key such as obtained from sqlite3VdbeParseRecord.
//	Key1 and Key2 do not have to contain the same number of fields. The key with fewer fields is usually compares less than the longer key. However if the UNPACKED_INCRKEY flags in pPKey2 is set and the common prefixes are equal, then key1 is less than key2. Or if the UNPACKED_MATCH_PREFIX flag is set and the prefixes are equal, then the keys are considered to be equal and the parts beyond the common prefix are ignored.
func (b Buffer) RecordCompare(r *UnpackedRecord) (rc int) {
	nKey1 := len(b)
	aKey1 := ([]byte)(b)
	pKeyInfo := r.pKeyInfo
	mem1 := &Mem{ enc: pKeyInfo.enc, db: pKeyInfo.db }

	szHdr1, buf := aKey1.ReadVarint32()
	iudx1 := len(nKey1) - len(buf)
	d1 := szHdr1
	nField := pKeyInfo.nField;
	for i := 0; idx1 < szHdr1 && i < r.nField; i++ {
		//	Read the serial types for the next element in each key.
		serial_type1, buf := Buffer(aKey[idx1:]).ReadVarint32()
		idx += len(aKey[idx1:]) - len(buf)
		if d1 >= nKey1 && VdbeSerialTypeLen(serial_type1) > 0 {
			break
		}

		//	Extract the values to be compared.
		d1 += mem1.VdbeSerialGet(aKey1[d1:], serial_type1)

		//	Do the comparison
		if i < nField {
			rc = sqlite3MemCompare(&mem1, r.aMem[i], pKeyInfo.Collations[i])
		} else {
			rc = sqlite3MemCompare(&mem1, r.aMem[i], 0)
		}

		if rc != 0 {
			assert( mem1.zMalloc == "" )			//	See comment below

			//	Invert the result if we are using DESC sort order.
			if pKeyInfo.aSortOrder && i < nField && pKeyInfo.aSortOrder[i] {
				rc = -rc
			}
    
			//	If the PREFIX_SEARCH flag is set and all fields except the final rowid field were equal, then clear the PREFIX_SEARCH flag and set pPKey2.rowid to the value of the rowid field in (pKey1, nKey1). This is used by the OP_IsUnique opcode.
			if r.flags & UNPACKED_PREFIX_SEARCH && i == r.nField - 1 {
				assert( idx1 == szHdr1 && rc )
				r.flags &= ~UNPACKED_PREFIX_SEARCH
				r.rowid = mem1.Integer()
			}
			return rc
		}
	}

	//	No memory allocation is ever used on mem1. Prove this using the following assert(). If the assert() fails, it indicates a memory leak and a need to call mem1.Release().
	assert( mem1.zMalloc == "" )

	//	rc == 0 here means that one of the keys ran out of fields and all the fields up to that point were equal. If the UNPACKED_INCRKEY flag is set, then break the tie by treating key2 as larger. If the UPACKED_PREFIX_MATCH flag is set, then keys with common prefixes are considered to be equal. Otherwise, the longer key is the larger. As it happens, the pPKey2 will always be the longer if there is a difference.
	assert( rc == 0 )
	switch {
	case r.flags & UNPACKED_INCRKEY:
		rc = -1
	case r.flags & UNPACKED_PREFIX_MATCH:
		//	Leave rc == 0
	case idx1 < szHdr1:
		rc = 1
	}
	return
}