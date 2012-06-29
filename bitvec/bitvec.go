package bitvector

import "crypto/rand"

//	This file implements an object that represents a fixed-length bitmap. Bits are numbered starting with 1.
//	A bitmap is used to record which pages of a database file have been journalled during a transaction, or which pages have the "dont-write" property.
//	Usually only a few pages are meet either condition so the bitmap is sparse and has low cardinality.
//	But sometimes (for example when during a DROP of a large table) most or all of the pages in a database can get journalled. In those cases, 
//	the bitmap becomes dense with high cardinality. The algorithm needs to handle both cases well.
//	The size of the bitmap is fixed when the object is created.
//	All bits are clear when the bitmap is created. Individual bits may be set or cleared one at a time.
//	Test operations are about 100 times more common that set operations. Clear operations are exceedingly rare. There are usually between 5 and 500 set
//	operations per Bitvec object, though the number of sets can sometimes grow into tens of thousands or larger. The size of the Bitvec object is the number
//	of pages in the database file at the start of a transaction, and is thus usually less than a few thousand, but can be as large as 2 billion for a really big database.

type BitVector interface{
	Test(i int) bool
	Set(i int) bool
	Clear(i int)
	Len() int
}

// Create a new bitmap object able to handle bits between 0 and iSize, inclusive. Return a pointer to the new object.
func New(size int) BitVector {
	if i > 1 << 10 {
		return NewBitmap(size)
	}
	return NewHashmap(size)
}