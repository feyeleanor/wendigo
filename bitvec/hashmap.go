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
//
//	Test operations are about 100 times more common that set operations.
//	Clear operations are exceedingly rare. There are usually between 5 and 500 set operations per Bitvec object, though the number of sets can
//	sometimes grow into tens of thousands or larger. The size of the Bitvec object is the number of pages in the database file at the
//	start of a transaction, and is thus usually less than a few thousand, but can be as large as 2 billion for a really big database.


type Hashmap struct {
	hash	map[int] byte
	size	int
	set		int
}

func NewHashmap(size int) *Hashmap {
	return &Hashmap{ hash: make(map[int] byte), size: size }
}

func (h *Hashmap) Len() int {
	return h.size
}

func (h *Hashmap) Test(i int) bool {
	if i > 0 && i <= h.size {
		i--
		return h.hash[i / 8] & (1 << (i & 7)) != 0
	}
}

func (h *Hashmap) Set(i int) bool {
	if i > 0 && i <= h.size {
		i--
		segment = i / 8
		if x, ok := h.hash[segment]; ok {
			h.hash[segment] = x |= (1 << (i & 7))
		} else {
			h.hash[segment] = 1 << (i & 7)
		}
		h.set++
		return true
	}
}

func (h *Hashmap) Clear(i int) {
	if i > 0 && i <= h.size {
		i--
		segment := i / 8
		if x, ok := h.hash[segment]; ok {
			h.hash[segment] = x &= ~(1 << (i & 7))
		} else {
			h.hash[segment] = ~(1 << (i & 7))
		}
		h.set--
	}
}

func (h *Hashmap) Destroy() {}