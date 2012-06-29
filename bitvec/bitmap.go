package bitvector

import "crypto/rand"

//	This file implements an object that represents a fixed-length bitmap.
//	Bits are numbered starting with 1.
//	A bitmap is used to record which pages of a database file have been journalled during a transaction, or which pages have the "dont-write"
//	property. Usually only a few pages meet either condition so the bitmap is usually sparse and has low cardinality.
//	But sometimes (for example when during a DROP of a large table) most or all of the pages in a database can get journalled. In those cases, 
//	the bitmap becomes dense with high cardinality.  The algorithm needs to handle both cases well.
//	The size of the bitmap is fixed when the object is created.

//	All bits are clear when the bitmap is created. Individual bits may be set or cleared one at a time.

//	Test operations are about 100 times more common that set operations.
//	Clear operations are exceedingly rare. There are usually between 5 and 500 set operations per Bitvec object, though the number of sets can
//	sometimes grow into tens of thousands or larger. The size of the Bitvec object is the number of pages in the database file at the
//	start of a transaction, and is thus usually less than a few thousand, but can be as large as 2 billion for a really big database.

//	A bitmap is an instance of the following structure.
//	This bitmap records the existance of zero or more bits with values between 1 and iSize, inclusive.
//	The least significant bit is bit 1.

type Bitmap []byte

func NewBitmap(size int) (p Bitmap) {
	return make(bitmap, size)
}

//	Check to see if the i-th bit is set.  Return true or false.
func (b Bitmap) Test(i int) bool {
	segment := i / 8
	if i > 0 && segment <= len(b) {
		i--
		return b[segment] & (1 << (i & 7)) != 0
	}
	return false
}

//	Set the i-th bit.  Return true on success and false if anything goes wrong.
//	This routine might cause sub-bitmaps to be allocated. Failing to get the memory needed to hold the sub-bitmap is the only that can go wrong with an insert, assuming p and i are valid.
//	The calling function must ensure that p is a valid Bitvec object and that the value for "i" is within range of the Bitvec object.
//	Otherwise the behavior is undefined.
func (b Bitmap) Set(i int) bool {
	segment := i / 8
	if i > 0 && segment <= len(b) {
		i--
		b[segment] |= 1 << (i & 7)
		return true
	}
	return false
}

//	Clear the i-th bit.
//	pBuf must be a pointer to at least BITVEC_SZ bytes of temporary storage that BitvecClear can use to rebuilt its hash table.
func (b Bitmap) Clear(i int) {
	segment := i / 8
	if i > 0 && segment <= len(b) {
		i--
		b[segment] &= ~(1 << (i&(8-1)))
	}
}

//	Destroy a bitmap object. Reclaim all memory used.
func (b Bitmap) Destroy(i int) {
	//	TODO:	Ensure destruction of bitmaps is handled at a higher level
}

//	Return the value of the iSize parameter specified when Bitvec *p was created.
func (b Bitmap) Len() int {
	return len(b)
}
